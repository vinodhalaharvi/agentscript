// Package script — parse.go implements the Parse phase of the
// translator: text in, immutable AST out.
//
// Internally this uses two grammars side-by-side:
//
//  1. The participle-friendly grammar (parsedScript, parsedBlock, ...)
//     uses struct tags to describe the syntax. Participle requires
//     pointers and slice receivers; the parsed types reflect that.
//
//  2. The clean AST (ast.AST, ast.Block, ...) in the ast subpackage is
//     what the rest of the translator consumes. Sealed sums, no
//     participle leakage.
//
// The transition between the two happens in toAST after a successful
// parse. We keep them separate so participle's quirks (pointer fields,
// pre-defined tokens, etc.) don't pollute the AST that everything
// downstream sees.
package script

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// === Parse arrow ===========================================================

// Parse converts source text into the canonical AST. It is the first
// phase of the translator pipeline: Source >>> Parse >>> Resolve >>> ...
//
// Parse errors include file/line/column information from participle.
// They are returned as ParseError values; callers can errors.As to
// recover the underlying participle error for richer reporting.
func Parse(ctx context.Context, src Source) (ast.AST, error) {
	parsed, err := scriptParser.ParseString("", string(src))
	if err != nil {
		return ast.AST{}, ParseError{Err: err, Source: string(src)}
	}
	return toAST(parsed)
}

// ParseError wraps a participle error with the original source for
// improved error reporting downstream. Implements error and
// errors.Unwrap.
type ParseError struct {
	Err    error
	Source string
}

// Error implements error.
func (e ParseError) Error() string {
	return fmt.Sprintf("script.Parse: %v", e.Err)
}

// Unwrap returns the underlying participle error.
func (e ParseError) Unwrap() error { return e.Err }

// === Participle grammar ====================================================
//
// The grammar supports sequential pipelines and parallel fan-out, with
// the block's own parentheses able to wrap a parallel — matching the
// original internal/agentscript grammar exactly:
//
//	script     = block+
//	block      = backend mode "(" expr ")"
//	backend    = "memory" | "temporal"
//	mode       = "static" | "dynamic"
//	expr       = pipeline ( "<*>" pipeline )*      // 1 ⇒ sequential, 2+ ⇒ parallel
//	pipeline   = stage ( ">=>" stage )*
//	stage      = call | "(" expr ")"               // group for nesting/precedence
//	call       = IDENT ( STRING )*
//
// So both  memory static ( a <*> b )  (parallel directly in the block
// body, sharing the block's parens) and  ( ( a <*> b ) >=> c )  (nested
// groups) parse, exactly as the original runtime grammar accepted.

// parsedScript is the participle-level root.
type parsedScript struct {
	Blocks []*parsedBlock `@@+`
}

// parsedBlock matches: <backend> <mode> ( <expr> )
type parsedBlock struct {
	Backend string      `@("memory" | "temporal")`
	Mode    string      `@("static" | "dynamic")`
	OpenP   struct{}    `"("`
	Body    *parsedExpr `@@`
	CloseP  struct{}    `")"`
}

// parsedExpr matches: <pipeline> ( "<*>" <pipeline> )*
// One pipeline ⇒ sequential; two or more ⇒ a parallel fan-out. This is
// the production used by both the block body and any parenthesized group,
// so the block's own parens can wrap a parallel.
type parsedExpr struct {
	First    *parsedPipeline   `@@`
	Branches []*parsedPipeline `( "<*>" @@ )*`
}

// parsedPipeline matches: <stage> ( ">=>" <stage> )*
type parsedPipeline struct {
	First *parsedStage   `@@`
	Rest  []*parsedStage `( ">=>" @@ )*`
}

// parsedStage is one element of a pipeline: either a call or a
// parenthesized group (which is itself an expr, so it may be parallel).
type parsedStage struct {
	Group *parsedGroup `  @@`
	Call  *parsedCall  `| @@`
}

// parsedGroup matches: "(" <expr> ")"
type parsedGroup struct {
	Open  struct{}    `"("`
	Expr  *parsedExpr `@@`
	Close struct{}    `")"`
}

// parsedCall matches: <ident> <string>*
//
// Identifier is anything matching the Ident token; arguments are
// double-quoted strings. The parser accepts zero or more string args; the
// resolver enforces arity later.
type parsedCall struct {
	Name string   `@Ident`
	Args []string `@String*`
}

// scriptLexer defines tokens. Order matters: longer/more-specific
// tokens come first so they beat shorter prefixes (e.g. ">=>" and "<*>"
// win against single-char punctuation).
var scriptLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "Whitespace", Pattern: `[ \t\r\n]+`},
	{Name: "Arrow", Pattern: `>=>`},
	{Name: "Fanout", Pattern: `<\*>`},
	{Name: "Punct", Pattern: `[()]`},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
})

// scriptParser is the singleton participle parser for the grammar.
var scriptParser = participle.MustBuild[parsedScript](
	participle.Lexer(scriptLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.Unquote("String"),
)

// === Parsed → AST conversion ===============================================

// toAST converts the participle-level parsed tree into the canonical
// AST. The shape is mostly mechanical; the only real work is
// (a) translating keyword strings to enum values and (b) wrapping
// every Call in a Pipeline so downstream code always sees a uniform
// Block.Body shape.
//
// Wrapping rule: a Block's body is always a Pipeline node. A
// single-call block is a Pipeline with one stage. This keeps Lower
// from needing a special case for "block whose body is just a Call".
func toAST(parsed *parsedScript) (ast.AST, error) {
	out := ast.AST{Blocks: make([]ast.Block, 0, len(parsed.Blocks))}
	for i, pb := range parsed.Blocks {
		b, err := blockToAST(pb)
		if err != nil {
			return ast.AST{}, fmt.Errorf("block %d: %w", i, err)
		}
		out.Blocks = append(out.Blocks, b)
	}
	return out, nil
}

func blockToAST(pb *parsedBlock) (ast.Block, error) {
	backend, err := parseBackend(pb.Backend)
	if err != nil {
		return ast.Block{}, err
	}
	mode, err := parseMode(pb.Mode)
	if err != nil {
		return ast.Block{}, err
	}
	body, err := exprToAST(pb.Body)
	if err != nil {
		return ast.Block{}, err
	}
	return ast.Block{Backend: backend, Mode: mode, Body: body}, nil
}

func parseBackend(s string) (ast.Backend, error) {
	switch s {
	case "memory":
		return ast.BackendMemory, nil
	case "temporal":
		return ast.BackendTemporal, nil
	default:
		// The grammar restricts this to memory|temporal, so this is
		// defense in depth rather than a reachable path.
		return ast.BackendUnknown, fmt.Errorf("unknown backend %q", s)
	}
}

func parseMode(s string) (ast.Mode, error) {
	switch s {
	case "static":
		return ast.ModeStatic, nil
	case "dynamic":
		return ast.ModeDynamic, nil
	default:
		return ast.ModeUnknown, fmt.Errorf("unknown mode %q", s)
	}
}

// pipelineToAST always produces a Pipeline node, even for a single
// call. That uniformity simplifies the resolver and lowering passes.
// exprToAST converts a parsedExpr (a "<*>"-separated list of pipelines).
// One pipeline ⇒ that pipeline node; two or more ⇒ an ast.Parallel of the
// pipelines, wrapped in a one-stage Pipeline so a Block.Body is always an
// ast.Pipeline (the invariant Lower and the memory bridge rely on). This
// is what lets the block's own parens wrap a parallel:
// memory static ( a <*> b ).
func exprToAST(pe *parsedExpr) (ast.Node, error) {
	if pe == nil || pe.First == nil {
		return nil, fmt.Errorf("empty expression")
	}
	if len(pe.Branches) == 0 {
		return pipelineToAST(pe.First) // sequential — already an ast.Pipeline
	}
	branches := make([]ast.Node, 0, 1+len(pe.Branches))
	first, err := pipelineToAST(pe.First)
	if err != nil {
		return nil, fmt.Errorf("branch 0: %w", err)
	}
	branches = append(branches, first)
	for i, pp := range pe.Branches {
		b, err := pipelineToAST(pp)
		if err != nil {
			return nil, fmt.Errorf("branch %d: %w", i+1, err)
		}
		branches = append(branches, b)
	}
	return ast.Pipeline{Stages: []ast.Node{ast.Parallel{Branches: branches}}}, nil
}

// pipelineToAST converts a sequential pipeline of stages. Always returns
// an ast.Pipeline (even single-stage) to keep the body shape uniform.
func pipelineToAST(pp *parsedPipeline) (ast.Node, error) {
	if pp == nil || pp.First == nil {
		return nil, fmt.Errorf("empty pipeline")
	}
	stages := make([]ast.Node, 0, 1+len(pp.Rest))
	first, err := stageToAST(pp.First)
	if err != nil {
		return nil, fmt.Errorf("stage 0: %w", err)
	}
	stages = append(stages, first)
	for i, ps := range pp.Rest {
		s, err := stageToAST(ps)
		if err != nil {
			return nil, fmt.Errorf("stage %d: %w", i+1, err)
		}
		stages = append(stages, s)
	}
	return ast.Pipeline{Stages: stages}, nil
}

// stageToAST converts one pipeline stage — a call or a parenthesized
// group (which is itself an expr, possibly parallel).
func stageToAST(ps *parsedStage) (ast.Node, error) {
	switch {
	case ps == nil:
		return nil, fmt.Errorf("nil stage")
	case ps.Group != nil:
		return groupToAST(ps.Group)
	case ps.Call != nil:
		return callToAST(ps.Call)
	default:
		return nil, fmt.Errorf("empty stage")
	}
}

// groupToAST converts a parenthesized group "(" expr ")". Inside a group
// a Parallel is a legitimate stage, so a group wrapping a parallel expr
// yields the ast.Parallel directly (unwrapping the one-stage Pipeline
// that exprToAST adds for the body invariant). A sequential group yields
// its Pipeline node (grouping/precedence).
func groupToAST(pg *parsedGroup) (ast.Node, error) {
	if pg == nil || pg.Expr == nil {
		return nil, fmt.Errorf("empty group")
	}
	node, err := exprToAST(pg.Expr)
	if err != nil {
		return nil, err
	}
	if p, ok := node.(ast.Pipeline); ok && len(p.Stages) == 1 {
		if _, isPar := p.Stages[0].(ast.Parallel); isPar {
			return p.Stages[0], nil
		}
	}
	return node, nil
}

func callToAST(pc *parsedCall) (ast.Node, error) {
	if pc == nil || pc.Name == "" {
		return nil, fmt.Errorf("missing call name")
	}
	args := make([]ast.Arg, 0, len(pc.Args))
	for _, raw := range pc.Args {
		// participle.Unquote strips surrounding quotes; raw is the
		// inner content. Normalize \" within for safety, even though
		// the current lexer doesn't permit them.
		args = append(args, ast.StringArg{Value: strings.ReplaceAll(raw, `\"`, `"`)})
	}
	return ast.Call{Name: pc.Name, Args: args}, nil
}
