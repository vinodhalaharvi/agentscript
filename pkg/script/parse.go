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
// The grammar supports sequential pipelines and parenthesized parallel
// fan-out, matching the original internal/agentscript grammar:
//
//	script     = block+
//	block      = backend mode "(" pipeline ")"
//	backend    = "memory" | "temporal"
//	mode       = "static" | "dynamic"
//	pipeline   = stage ( ">=>" stage )*
//	stage      = call | group
//	group      = "(" pipeline ( "<*>" pipeline )* ")"
//	call       = IDENT ( STRING )*
//
// A group with one inner pipeline is just grouping/precedence; a group
// with two or more "<*>"-separated pipelines is a parallel fan-out. This
// lets ( a <*> b ) >=> merge and arbitrary nesting parse, exactly as the
// original runtime grammar did.

// parsedScript is the participle-level root.
type parsedScript struct {
	Blocks []*parsedBlock `@@+`
}

// parsedBlock matches: <backend> <mode> ( <pipeline> )
type parsedBlock struct {
	Backend string          `@("memory" | "temporal")`
	Mode    string          `@("static" | "dynamic")`
	OpenP   struct{}        `"("`
	Body    *parsedPipeline `@@`
	CloseP  struct{}        `")"`
}

// parsedPipeline matches: <stage> ( ">=>" <stage> )*
type parsedPipeline struct {
	First *parsedStage   `@@`
	Rest  []*parsedStage `( ">=>" @@ )*`
}

// parsedStage is one element of a pipeline: either a call or a
// parenthesized group (which may be a parallel fan-out).
type parsedStage struct {
	Group *parsedGroup `  @@`
	Call  *parsedCall  `| @@`
}

// parsedGroup matches: "(" <pipeline> ( "<*>" <pipeline> )* ")"
// One branch ⇒ grouping; two or more ⇒ parallel fan-out.
type parsedGroup struct {
	Open     struct{}          `"("`
	First    *parsedPipeline   `@@`
	Branches []*parsedPipeline `( "<*>" @@ )*`
	Close    struct{}          `")"`
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
	body, err := pipelineToAST(pb.Body)
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
func pipelineToAST(pp *parsedPipeline) (ast.Node, error) {
	if pp == nil || pp.First == nil {
		return nil, fmt.Errorf("empty block body")
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

// stageToAST converts one pipeline stage — either a call or a
// parenthesized group — into an AST node.
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

// groupToAST converts a parenthesized group. One inner pipeline ⇒ just
// that pipeline (grouping/precedence). Two or more "<*>"-separated
// pipelines ⇒ a Parallel fan-out whose branches are those pipelines.
func groupToAST(pg *parsedGroup) (ast.Node, error) {
	if pg == nil || pg.First == nil {
		return nil, fmt.Errorf("empty group")
	}
	first, err := pipelineToAST(pg.First)
	if err != nil {
		return nil, fmt.Errorf("group branch 0: %w", err)
	}
	if len(pg.Branches) == 0 {
		// Pure grouping — unwrap to the inner pipeline node.
		return first, nil
	}
	branches := make([]ast.Node, 0, 1+len(pg.Branches))
	branches = append(branches, first)
	for i, pb := range pg.Branches {
		b, err := pipelineToAST(pb)
		if err != nil {
			return nil, fmt.Errorf("group branch %d: %w", i+1, err)
		}
		branches = append(branches, b)
	}
	return ast.Parallel{Branches: branches}, nil
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
