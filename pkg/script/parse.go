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
// The grammar is intentionally minimal for the MVP:
//
//	script   = block+
//	block    = backend mode "(" expression ")"
//	backend  = "memory" | "temporal"
//	mode     = "static" | "dynamic"
//	expression = pipeline
//	pipeline = call ( ">=>" call )*
//	call     = IDENT ( STRING )*
//
// Parallel ("<*>") is intentionally NOT in the MVP grammar — the AST
// can represent it but the parser won't produce it yet.

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

// parsedPipeline matches: <call> ( ">=>" <call> )*
//
// Note: this only supports pipelines of calls in the MVP. Once Parallel
// and grouping syntax are added, this becomes a more general expression
// production.
type parsedPipeline struct {
	First *parsedCall   `@@`
	Rest  []*parsedCall `( ">=>" @@ )*`
}

// parsedCall matches: <ident> <string>*
//
// Identifier is anything matching the Ident token; arguments are
// double-quoted strings. The MVP accepts zero or more string args; the
// resolver enforces arity later.
type parsedCall struct {
	Name string   `@Ident`
	Args []string `@String*`
}

// scriptLexer defines tokens. Order matters: longer/more-specific
// tokens come first so they beat shorter prefixes (e.g. ">=>" wins
// against ">" if we ever add it).
var scriptLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "Whitespace", Pattern: `[ \t\r\n]+`},
	{Name: "Arrow", Pattern: `>=>`},
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
	first, err := callToAST(pp.First)
	if err != nil {
		return nil, fmt.Errorf("stage 0: %w", err)
	}
	stages = append(stages, first)
	for i, pc := range pp.Rest {
		c, err := callToAST(pc)
		if err != nil {
			return nil, fmt.Errorf("stage %d: %w", i+1, err)
		}
		stages = append(stages, c)
	}
	return ast.Pipeline{Stages: stages}, nil
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
