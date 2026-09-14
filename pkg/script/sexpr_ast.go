// Package script — sexpr_ast.go converts the generic s-expression Form
// tree into the canonical AST. It is the second half of the s-expression
// front end; the reader itself lives in the sexpr subpackage.
//
// The AST is unchanged. Nothing downstream of Parse — Resolve, Lower,
// Finalize, Validate, Submit — knows or cares which surface syntax
// produced it. That is the whole point of doing the work here.
//
// Surface grammar:
//
//	program = form+
//	form    = block | expr
//	block   = "(" ("block" | "script") option* expr ")"
//	option  = ":backend" ("memory" | "temporal" | "sibyl")
//	        | ":mode"    ("static" | "dynamic")
//	expr    = pipe | par | call | symbol
//	pipe    = "(" "pipe" expr+ ")"
//	par     = "(" "par" expr expr+ ")"
//	call    = "(" symbol literal* ")"
//	symbol  = bare identifier — a call with no arguments
//
// A top-level form that is not a block is an implicit block with the
// default backend (memory) and mode (static). Backend selection is a
// property of the source, never a runtime flag: the same text always
// runs the same way.
package script

import (
	"context"
	"fmt"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/script/sexpr"
)

// Reserved form heads. Everything else is a verb name resolved against
// the registry at Resolve time.
const (
	formBlock  = "block"
	formScript = "script"
	formPipe   = "pipe"
	formPar    = "par"
)

// surfaceVerbs maps s-expression surface names onto registry verb names.
//
// "if" is a poor head in a lisp-shaped syntax: every reader arrives
// expecting a special form with lazy branches, and AgentScript's "if" is
// a pipeline stage taking a condition string. Renaming the surface to
// "when" (with "guard" as an accepted synonym) removes the collision
// without touching the registry, the resolver, or the runtime.
var surfaceVerbs = map[string]string{
	"when":  "if",
	"guard": "if",
}

// verbSurface is the inverse of surfaceVerbs, used by the printer so a
// round trip through Parse and PrintSExpr is stable.
var verbSurface = map[string]string{
	"if": "when",
}

// === Entry point ===========================================================

// ParseSExpr converts s-expression source into the canonical AST. It is
// an alternative front end to the legacy operator dialect and produces
// exactly the same AST values.
//
// Errors are wrapped in ParseError so callers can errors.As them the
// same way they do for the legacy parser.
func ParseSExpr(_ context.Context, src Source) (ast.AST, error) {
	forms, err := sexpr.Read(string(src))
	if err != nil {
		return ast.AST{}, ParseError{Err: err, Source: string(src)}
	}
	if len(forms) == 0 {
		return ast.AST{}, ParseError{
			Err:    fmt.Errorf("script is empty"),
			Source: string(src),
		}
	}
	out := ast.AST{Blocks: make([]ast.Block, 0, len(forms))}
	for i, f := range forms {
		b, err := blockFromForm(f)
		if err != nil {
			return ast.AST{}, ParseError{
				Err:    fmt.Errorf("block %d: %w", i, err),
				Source: string(src),
			}
		}
		out.Blocks = append(out.Blocks, b)
	}
	return out, nil
}

// === Blocks ================================================================

// blockFromForm converts one top-level form. An explicit (block ...) or
// (script ...) form carries options; any other form is an implicit block
// with defaults.
func blockFromForm(f sexpr.Form) (ast.Block, error) {
	backend := ast.BackendMemory
	mode := ast.ModeStatic
	body := f

	if head, ok := f.Head(); ok && (head == formBlock || head == formScript) {
		rest, b, m, err := readBlockOptions(f.Items[1:], backend, mode)
		if err != nil {
			return ast.Block{}, err
		}
		backend, mode = b, m
		switch len(rest) {
		case 1:
			body = rest[0]
		case 0:
			return ast.Block{}, fmt.Errorf("%s: %s has no body", f.Pos, head)
		default:
			return ast.Block{}, fmt.Errorf(
				"%s: %s takes exactly one body form, got %d "+
					"(wrap them in a pipe or par)", f.Pos, head, len(rest))
		}
	}

	node, err := nodeFromForm(body)
	if err != nil {
		return ast.Block{}, err
	}
	// Block.Body is always an ast.Pipeline — an invariant Lower and the
	// memory bridge both rely on.
	return ast.Block{Backend: backend, Mode: mode, Body: asPipeline(node)}, nil
}

// readBlockOptions consumes leading :keyword value pairs, returning the
// remaining forms plus the resolved backend and mode.
func readBlockOptions(
	items []sexpr.Form,
	backend ast.Backend,
	mode ast.Mode,
) ([]sexpr.Form, ast.Backend, ast.Mode, error) {
	for len(items) > 0 && items[0].Kind == sexpr.KindKeyword {
		key := items[0]
		if len(items) < 2 {
			return nil, backend, mode, fmt.Errorf(
				"%s: option :%s has no value", key.Pos, key.Text)
		}
		val := items[1]
		if val.Kind != sexpr.KindSymbol {
			return nil, backend, mode, fmt.Errorf(
				"%s: option :%s expects a symbol, got %s", val.Pos, key.Text, val.Kind)
		}
		switch key.Text {
		case "backend":
			b, err := backendFromName(val.Text)
			if err != nil {
				return nil, backend, mode, fmt.Errorf("%s: %w", val.Pos, err)
			}
			backend = b
		case "mode":
			m, err := modeFromName(val.Text)
			if err != nil {
				return nil, backend, mode, fmt.Errorf("%s: %w", val.Pos, err)
			}
			mode = m
		default:
			return nil, backend, mode, fmt.Errorf(
				"%s: unknown option :%s (want :backend or :mode)", key.Pos, key.Text)
		}
		items = items[2:]
	}
	return items, backend, mode, nil
}

// backendFromName maps a surface backend name to the enum. "sibyl" is
// accepted as a synonym for "temporal": the AST names the execution
// engine, scripts tend to name the project.
func backendFromName(name string) (ast.Backend, error) {
	switch name {
	case "memory":
		return ast.BackendMemory, nil
	case "temporal", "sibyl":
		return ast.BackendTemporal, nil
	default:
		return ast.BackendUnknown, fmt.Errorf(
			"unknown backend %q (want memory, temporal, or sibyl)", name)
	}
}

func modeFromName(name string) (ast.Mode, error) {
	switch name {
	case "static":
		return ast.ModeStatic, nil
	case "dynamic":
		return ast.ModeDynamic, nil
	default:
		return ast.ModeUnknown, fmt.Errorf(
			"unknown mode %q (want static or dynamic)", name)
	}
}

// === Expressions ===========================================================

func nodeFromForm(f sexpr.Form) (ast.Node, error) {
	switch f.Kind {
	case sexpr.KindSymbol:
		// A bare symbol is a call with no arguments — the s-expression
		// spelling of a pipeline stage like ">=> summarize".
		//
		// Args is an empty slice rather than nil so the two front ends
		// produce reflect.DeepEqual ASTs for equivalent scripts; the
		// legacy parser always allocates.
		return ast.Call{Name: verbName(f.Text), Args: []ast.Arg{}}, nil
	case sexpr.KindList:
		return nodeFromList(f)
	default:
		return nil, fmt.Errorf(
			"%s: expected a call or a form, got %s", f.Pos, f.Kind)
	}
}

func nodeFromList(f sexpr.Form) (ast.Node, error) {
	if len(f.Items) == 0 {
		return nil, fmt.Errorf("%s: empty form ()", f.Pos)
	}
	head := f.Items[0]
	if head.Kind != sexpr.KindSymbol {
		return nil, fmt.Errorf(
			"%s: form head must be a symbol, got %s", head.Pos, head.Kind)
	}
	switch head.Text {
	case formPipe:
		return pipeFromForm(f)
	case formPar:
		return parFromForm(f)
	case formBlock, formScript:
		return nil, fmt.Errorf(
			"%s: %s is only valid at top level", head.Pos, head.Text)
	default:
		return callFromForm(f)
	}
}

// pipeFromForm converts (pipe a b c) into a Pipeline. Threading is
// explicit in the surface syntax: each stage receives the previous
// stage's output, exactly as ">=>" did.
func pipeFromForm(f sexpr.Form) (ast.Node, error) {
	rest := f.Items[1:]
	if len(rest) == 0 {
		return nil, fmt.Errorf("%s: pipe needs at least one stage", f.Pos)
	}
	stages := make([]ast.Node, 0, len(rest))
	for i, item := range rest {
		n, err := nodeFromForm(item)
		if err != nil {
			return nil, fmt.Errorf("pipe stage %d: %w", i, err)
		}
		stages = append(stages, n)
	}
	return ast.Pipeline{Stages: stages}, nil
}

// parFromForm converts (par a b) into a Parallel. Each branch is wrapped
// as a Pipeline to match what the legacy parser produced, so the two
// front ends yield identical ASTs for equivalent scripts.
func parFromForm(f sexpr.Form) (ast.Node, error) {
	rest := f.Items[1:]
	if len(rest) < 2 {
		return nil, fmt.Errorf(
			"%s: par needs at least two branches, got %d", f.Pos, len(rest))
	}
	branches := make([]ast.Node, 0, len(rest))
	for i, item := range rest {
		n, err := nodeFromForm(item)
		if err != nil {
			return nil, fmt.Errorf("par branch %d: %w", i, err)
		}
		branches = append(branches, asPipeline(n))
	}
	return ast.Parallel{Branches: branches}, nil
}

// callFromForm converts (verb "arg" ...) into a Call. Arity and types
// are not checked here; Resolve owns that against the registry.
func callFromForm(f sexpr.Form) (ast.Node, error) {
	head := f.Items[0]
	args := make([]ast.Arg, 0, len(f.Items)-1)
	for _, item := range f.Items[1:] {
		switch item.Kind {
		case sexpr.KindString:
			args = append(args, ast.StringArg{Value: item.Text})
		case sexpr.KindNumber:
			args = append(args, ast.NumArg{Value: item.Num})
		default:
			return nil, fmt.Errorf(
				"%s: %s expects literal arguments, got %s",
				item.Pos, head.Text, item.Kind)
		}
	}
	return ast.Call{Name: verbName(head.Text), Args: args}, nil
}

// === Helpers ===============================================================

// verbName maps a surface name onto its registry verb name.
func verbName(surface string) string {
	if v, ok := surfaceVerbs[surface]; ok {
		return v
	}
	return surface
}

// asPipeline wraps a node in a single-stage Pipeline unless it already
// is one. Used for Block bodies and Parallel branches, both of which are
// required to be Pipelines.
func asPipeline(n ast.Node) ast.Node {
	if p, ok := n.(ast.Pipeline); ok {
		return p
	}
	return ast.Pipeline{Stages: []ast.Node{n}}
}
