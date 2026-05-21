package script_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
	"github.com/vinodhalaharvi/agentscript/pkg/script/resolved"
)

// === Test registry helpers =================================================

// echoReg returns a registry with a single "echo" builtin taking one
// required string arg.
func echoReg(t *testing.T) *registry.Registry {
	t.Helper()
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{
		Name:    "echo",
		AgentID: "agentscript/echo",
		ArgSchema: registry.ArgSchema{
			Params: []registry.ParamSpec{{Name: "message", Type: registry.StringT}},
		},
	})
	return r
}

// resolveSrc parses then resolves a source string against reg.
func resolveSrc(t *testing.T, reg *registry.Registry, src string) (resolved.AST, error) {
	t.Helper()
	a, err := script.Parse(context.Background(), script.Source(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return script.Resolve(context.Background(), reg, a)
}

// === Happy path ============================================================

func TestResolve_SingleCall(t *testing.T) {
	reg := echoReg(t)
	got, err := resolveSrc(t, reg, `temporal static ( echo "hello" )`)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("Blocks = %d, want 1", len(got.Blocks))
	}
	pipe, ok := got.Blocks[0].Body.(resolved.Pipeline)
	if !ok {
		t.Fatalf("Body is %T, want resolved.Pipeline", got.Blocks[0].Body)
	}
	if len(pipe.Stages) != 1 {
		t.Fatalf("Stages = %d, want 1", len(pipe.Stages))
	}
	call, ok := pipe.Stages[0].(resolved.Call)
	if !ok {
		t.Fatalf("Stage is %T, want resolved.Call", pipe.Stages[0])
	}
	if call.Name != "echo" {
		t.Errorf("Name = %q, want echo", call.Name)
	}
	if call.Spec.AgentID != "agentscript/echo" {
		t.Errorf("Spec.AgentID = %q, want agentscript/echo", call.Spec.AgentID)
	}
	if len(call.Args) != 1 {
		t.Fatalf("Args = %d, want 1", len(call.Args))
	}
}

func TestResolve_Pipeline(t *testing.T) {
	reg := echoReg(t)
	got, err := resolveSrc(t, reg, `temporal static ( echo "a" >=> echo "b" >=> echo "c" )`)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	pipe := got.Blocks[0].Body.(resolved.Pipeline)
	if len(pipe.Stages) != 3 {
		t.Fatalf("Stages = %d, want 3", len(pipe.Stages))
	}
	for i, s := range pipe.Stages {
		c, ok := s.(resolved.Call)
		if !ok {
			t.Fatalf("stage %d is %T, want resolved.Call", i, s)
		}
		if c.Spec.AgentID != "agentscript/echo" {
			t.Errorf("stage %d Spec.AgentID = %q", i, c.Spec.AgentID)
		}
	}
}

func TestResolve_PreservesBackendMode(t *testing.T) {
	reg := echoReg(t)
	got, err := resolveSrc(t, reg, `memory dynamic ( echo "x" )`)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	b := got.Blocks[0]
	if b.Backend != ast.BackendMemory {
		t.Errorf("Backend = %v, want memory", b.Backend)
	}
	if b.Mode != ast.ModeDynamic {
		t.Errorf("Mode = %v, want dynamic", b.Mode)
	}
}

func TestResolve_DoesNotMutateInput(t *testing.T) {
	reg := echoReg(t)
	a, err := script.Parse(context.Background(), script.Source(`temporal static ( echo "x" )`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Snapshot the call name before resolving.
	pipe := a.Blocks[0].Body.(ast.Pipeline)
	origCall := pipe.Stages[0].(ast.Call)
	origName := origCall.Name
	origArgCount := len(origCall.Args)

	if _, err := script.Resolve(context.Background(), reg, a); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// The original AST value should be unchanged.
	pipe2 := a.Blocks[0].Body.(ast.Pipeline)
	call2 := pipe2.Stages[0].(ast.Call)
	if call2.Name != origName || len(call2.Args) != origArgCount {
		t.Error("Resolve mutated the input AST")
	}
}

// === ResolveWith arrow form ================================================

func TestResolveWith_Arrow(t *testing.T) {
	reg := echoReg(t)
	arrow := script.ResolveWith(reg)
	a, _ := script.Parse(context.Background(), script.Source(`temporal static ( echo "hi" )`))
	got, err := arrow(context.Background(), a)
	if err != nil {
		t.Fatalf("arrow: %v", err)
	}
	if len(got.Blocks) != 1 {
		t.Errorf("Blocks = %d, want 1", len(got.Blocks))
	}
}

// === Unknown builtin =======================================================

func TestResolve_UnknownBuiltin(t *testing.T) {
	reg := echoReg(t)
	_, err := resolveSrc(t, reg, `temporal static ( nonexistent "x" )`)
	if err == nil {
		t.Fatal("expected error for unknown builtin")
	}
	var ube *script.UnknownBuiltinError
	if !errors.As(err, &ube) {
		t.Fatalf("err = %v, want UnknownBuiltinError", err)
	}
	if ube.Name != "nonexistent" {
		t.Errorf("UnknownBuiltinError.Name = %q, want nonexistent", ube.Name)
	}
	// Known list should include echo.
	foundEcho := false
	for _, n := range ube.Known {
		if n == "echo" {
			foundEcho = true
		}
	}
	if !foundEcho {
		t.Errorf("Known = %v, should include echo", ube.Known)
	}
}

func TestResolve_NilRegistry(t *testing.T) {
	a, _ := script.Parse(context.Background(), script.Source(`temporal static ( echo "x" )`))
	_, err := script.Resolve(context.Background(), nil, a)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

// === Arity errors ==========================================================

func TestResolve_TooFewArgs(t *testing.T) {
	reg := echoReg(t) // echo requires 1 arg
	_, err := resolveSrc(t, reg, `temporal static ( echo )`)
	if err == nil {
		t.Fatal("expected arity error for echo with 0 args")
	}
	var ae *script.ArityError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want ArityError", err)
	}
	if ae.Got != 0 || ae.MinWant != 1 {
		t.Errorf("ArityError = got %d minWant %d, expected got 0 minWant 1", ae.Got, ae.MinWant)
	}
}

func TestResolve_TooManyArgs(t *testing.T) {
	reg := echoReg(t) // echo takes exactly 1 arg, not variadic
	_, err := resolveSrc(t, reg, `temporal static ( echo "a" "b" "c" )`)
	if err == nil {
		t.Fatal("expected arity error for echo with 3 args")
	}
	var ae *script.ArityError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want ArityError", err)
	}
}

func TestResolve_OptionalArg(t *testing.T) {
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{
		Name:    "greet",
		AgentID: "agentscript/greet",
		ArgSchema: registry.ArgSchema{
			Params: []registry.ParamSpec{
				{Name: "name", Type: registry.StringT},
				{Name: "greeting", Type: registry.StringT, Optional: true},
			},
		},
	})
	// 1 arg (omitting the optional) should resolve.
	if _, err := resolveSrc(t, r, `temporal static ( greet "alice" )`); err != nil {
		t.Errorf("1 arg should resolve (optional omitted): %v", err)
	}
	// 2 args should resolve.
	if _, err := resolveSrc(t, r, `temporal static ( greet "alice" "hi" )`); err != nil {
		t.Errorf("2 args should resolve: %v", err)
	}
	// 0 args should fail (name is required).
	if _, err := resolveSrc(t, r, `temporal static ( greet )`); err == nil {
		t.Error("0 args should fail (name required)")
	}
	// 3 args should fail (only 2 declared, not variadic).
	if _, err := resolveSrc(t, r, `temporal static ( greet "a" "b" "c" )`); err == nil {
		t.Error("3 args should fail (max 2)")
	}
}

func TestResolve_Variadic(t *testing.T) {
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{
		Name:    "concat",
		AgentID: "agentscript/concat",
		ArgSchema: registry.ArgSchema{
			Params:       []registry.ParamSpec{{Name: "first", Type: registry.StringT}},
			Variadic:     true,
			VariadicType: registry.StringT,
		},
	})
	// 1 arg (just the required leading one).
	if _, err := resolveSrc(t, r, `temporal static ( concat "a" )`); err != nil {
		t.Errorf("1 arg should resolve: %v", err)
	}
	// many args.
	if _, err := resolveSrc(t, r, `temporal static ( concat "a" "b" "c" "d" )`); err != nil {
		t.Errorf("many args should resolve for variadic: %v", err)
	}
	// 0 args should fail (first is required).
	if _, err := resolveSrc(t, r, `temporal static ( concat )`); err == nil {
		t.Error("0 args should fail (first required)")
	}
}

// === No-arg builtin ========================================================

func TestResolve_NoArgBuiltin(t *testing.T) {
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{
		Name:      "now",
		AgentID:   "agentscript/now",
		ArgSchema: registry.ArgSchema{}, // no params
	})
	if _, err := resolveSrc(t, r, `temporal static ( now )`); err != nil {
		t.Errorf("no-arg builtin should resolve with 0 args: %v", err)
	}
	if _, err := resolveSrc(t, r, `temporal static ( now "extra" )`); err == nil {
		t.Error("no-arg builtin should reject an arg")
	}
}

// === Error message smoke tests =============================================

func TestErrorMessages(t *testing.T) {
	ube := &script.UnknownBuiltinError{Name: "foo", Known: []string{"echo", "now"}}
	if ube.Error() == "" {
		t.Error("UnknownBuiltinError.Error() empty")
	}

	ubeEmpty := &script.UnknownBuiltinError{Name: "foo"}
	if ubeEmpty.Error() == "" {
		t.Error("UnknownBuiltinError (no known) Error() empty")
	}

	ae := &script.ArityError{Builtin: "echo", Got: 0, MinWant: 1, MaxWant: 1}
	if ae.Error() == "" {
		t.Error("ArityError.Error() empty")
	}

	ate := &script.ArgTypeError{Builtin: "echo", Index: 0, Want: registry.NumT, Got: "string"}
	if ate.Error() == "" {
		t.Error("ArgTypeError.Error() empty")
	}
}
