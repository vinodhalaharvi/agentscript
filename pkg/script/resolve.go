// Package script — resolve.go implements the Resolve phase of the
// translator: AST in, ResolvedAST out.
//
// Resolve walks the AST and, for each Call:
//
//  1. Looks up Call.Name in the Registry. Unknown name → error.
//  2. Attaches the matching BuiltinSpec, producing a resolved.Call.
//  3. Validates the call's arguments against the spec's ArgSchema
//     (arity and per-arg type).
//
// Resolve is a pure function over (AST, Registry). The output is a new
// resolved.AST; the input AST is never mutated.
package script

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
	"github.com/vinodhalaharvi/agentscript/pkg/script/resolved"
)

// Resolve binds an AST to a Registry, producing a ResolvedAST. Because
// the Registry is a parameter rather than ambient state, Resolve is
// curried into a one-argument arrow shape (Arrow[ast.AST, resolved.AST])
// via ResolveWith below.
//
// Use ResolveWith(reg) to obtain the arrow; Resolve is the underlying
// two-argument implementation.
func Resolve(ctx context.Context, reg *registry.Registry, in ast.AST) (resolved.AST, error) {
	if reg == nil {
		return resolved.AST{}, fmt.Errorf("script.Resolve: registry is nil")
	}
	out := resolved.AST{Blocks: make([]resolved.Block, 0, len(in.Blocks))}
	for i, b := range in.Blocks {
		rb, err := resolveBlock(reg, b)
		if err != nil {
			return resolved.AST{}, fmt.Errorf("block %d: %w", i, err)
		}
		out.Blocks = append(out.Blocks, rb)
	}
	return out, nil
}

// ResolveWith curries Resolve over a Registry, producing the arrow used
// in the translator pipeline: Arrow[ast.AST, resolved.AST].
func ResolveWith(reg *registry.Registry) func(context.Context, ast.AST) (resolved.AST, error) {
	return func(ctx context.Context, in ast.AST) (resolved.AST, error) {
		return Resolve(ctx, reg, in)
	}
}

func resolveBlock(reg *registry.Registry, b ast.Block) (resolved.Block, error) {
	body, err := resolveNode(reg, b.Backend, b.Body)
	if err != nil {
		return resolved.Block{}, err
	}
	return resolved.Block{Backend: b.Backend, Mode: b.Mode, Body: body}, nil
}

func resolveNode(reg *registry.Registry, backend ast.Backend, n ast.Node) (resolved.Node, error) {
	switch node := n.(type) {
	case ast.Pipeline:
		stages := make([]resolved.Node, 0, len(node.Stages))
		for i, s := range node.Stages {
			rs, err := resolveNode(reg, backend, s)
			if err != nil {
				return nil, fmt.Errorf("stage %d: %w", i, err)
			}
			stages = append(stages, rs)
		}
		return resolved.Pipeline{Stages: stages}, nil

	case ast.Parallel:
		branches := make([]resolved.Node, 0, len(node.Branches))
		for i, br := range node.Branches {
			rb, err := resolveNode(reg, backend, br)
			if err != nil {
				return nil, fmt.Errorf("branch %d: %w", i, err)
			}
			branches = append(branches, rb)
		}
		return resolved.Parallel{Branches: branches}, nil

	case ast.Call:
		return resolveCall(reg, backend, node)

	default:
		return nil, fmt.Errorf("unknown AST node type %T", n)
	}
}

func resolveCall(reg *registry.Registry, backend ast.Backend, c ast.Call) (resolved.Node, error) {
	// Vocabulary check: is this a real verb at all? A genuinely unknown
	// name (typo, hallucination) fails here — the safety net.
	spec, ok := reg.Lookup(c.Name)
	if !ok {
		return nil, &UnknownBuiltinError{Name: c.Name, Known: reg.Names()}
	}
	if err := validateArgs(c, spec); err != nil {
		return nil, err
	}
	// Availability check: the verb exists, but does it run on THIS
	// backend yet? A known verb targeted at an unsupported backend yields
	// a distinct NotImplementedOnBackendError — never confused with an
	// unknown verb. This is what keeps the registry complete (every real
	// verb resolves) while being honest about where each can execute.
	if err := checkBackend(c.Name, spec, backend); err != nil {
		return nil, err
	}
	return resolved.Call{Name: c.Name, Args: c.Args, Spec: spec}, nil
}

// checkBackend verifies the builtin supports the block's backend. The
// ast.Backend keyword is mapped to the registry's Backend enum.
func checkBackend(name string, spec registry.BuiltinSpec, backend ast.Backend) error {
	var rb registry.Backend
	switch backend {
	case ast.BackendMemory:
		rb = registry.BackendMemory
	case ast.BackendTemporal:
		rb = registry.BackendTemporal
	default:
		// Unknown/zero backend: don't block resolution on it here; the
		// parser guarantees a valid backend, and Finalize/dispatch will
		// handle anything unexpected.
		return nil
	}
	if !spec.SupportsBackend(rb) {
		return &NotImplementedOnBackendError{Builtin: name, Backend: backend.String()}
	}
	return nil
}

// validateArgs checks a call's arguments against the builtin's schema:
// arity (min required, max unless variadic) and per-argument type.
func validateArgs(c ast.Call, spec registry.BuiltinSpec) error {
	schema := spec.ArgSchema
	got := len(c.Args)
	minReq := schema.MinRequired()
	maxAllowed := len(schema.Params)

	if got < minReq {
		return &ArityError{Builtin: c.Name, Got: got, MinWant: minReq, MaxWant: maxAllowed, Variadic: schema.Variadic}
	}
	if !schema.Variadic && got > maxAllowed {
		return &ArityError{Builtin: c.Name, Got: got, MinWant: minReq, MaxWant: maxAllowed, Variadic: false}
	}

	// Per-arg type check.
	for i, arg := range c.Args {
		want := paramTypeAt(schema, i)
		if !argMatchesType(arg, want) {
			return &ArgTypeError{
				Builtin: c.Name,
				Index:   i,
				Want:    want,
				Got:     argTypeName(arg),
			}
		}
	}
	return nil
}

// paramTypeAt returns the declared type for the argument at index i,
// honoring variadic trailing params.
func paramTypeAt(schema registry.ArgSchema, i int) registry.ArgType {
	if i < len(schema.Params) {
		return schema.Params[i].Type
	}
	// Past the fixed params: must be variadic (arity check already passed).
	return schema.VariadicType
}

func argMatchesType(arg ast.Arg, want registry.ArgType) bool {
	switch arg.(type) {
	case ast.StringArg:
		return want == registry.StringT
	case ast.NumArg:
		return want == registry.NumT
	default:
		return false
	}
}

func argTypeName(arg ast.Arg) string {
	switch arg.(type) {
	case ast.StringArg:
		return "string"
	case ast.NumArg:
		return "number"
	default:
		return "unknown"
	}
}

// === Errors ================================================================

// UnknownBuiltinError is returned when a Call references a name not in
// the registry.
type UnknownBuiltinError struct {
	Name  string
	Known []string
}

func (e *UnknownBuiltinError) Error() string {
	if len(e.Known) == 0 {
		return fmt.Sprintf("unknown builtin %q (no builtins registered)", e.Name)
	}
	return fmt.Sprintf("unknown builtin %q (known: %s)", e.Name, strings.Join(e.Known, ", "))
}

// NotImplementedOnBackendError is returned when a Call references a verb
// that IS in the registry (a real, known verb) but does not yet run on
// the block's target backend. It is deliberately distinct from
// UnknownBuiltinError: the verb is recognized vocabulary, it just isn't
// available on this backend yet (e.g. a memory-only verb under a temporal
// block, before it has a registered Sibyl activity).
type NotImplementedOnBackendError struct {
	Builtin string
	Backend string
}

func (e *NotImplementedOnBackendError) Error() string {
	return fmt.Sprintf("builtin %q is not implemented on the %s backend yet", e.Builtin, e.Backend)
}

// ArityError is returned when a call has the wrong number of arguments.
type ArityError struct {
	Builtin  string
	Got      int
	MinWant  int
	MaxWant  int
	Variadic bool
}

func (e *ArityError) Error() string {
	if e.Variadic {
		return fmt.Sprintf("builtin %q: got %d args, want at least %d", e.Builtin, e.Got, e.MinWant)
	}
	if e.MinWant == e.MaxWant {
		return fmt.Sprintf("builtin %q: got %d args, want %d", e.Builtin, e.Got, e.MinWant)
	}
	return fmt.Sprintf("builtin %q: got %d args, want %d-%d", e.Builtin, e.Got, e.MinWant, e.MaxWant)
}

// ArgTypeError is returned when an argument has the wrong type.
type ArgTypeError struct {
	Builtin string
	Index   int
	Want    registry.ArgType
	Got     string
}

func (e *ArgTypeError) Error() string {
	return fmt.Sprintf("builtin %q: arg %d is %s, want %s", e.Builtin, e.Index, e.Got, e.Want)
}
