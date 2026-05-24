package script_test

import (
	"context"
	"testing"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"
	"github.com/vinodhalaharvi/weft/weft"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
)

// These tests assert the COMPOSITION property: the compile pipeline is a
// composed weft.Arrow, and it can be composed further into larger arrows
// just like any other. If the phases ever stop being composable arrows,
// these won't compile — which is the point.

func TestCompileArrow_IsAComposableArrow(t *testing.T) {
	reg := script.DefaultRegistry()

	// CompileArrow returns a weft.Arrow[Source, Plan]. Assigning it to the
	// arrow type is a compile-time proof of its shape.
	var compile weft.Arrow[script.Source, sibyl.Plan] = script.CompileArrow(reg)

	plan, err := compile(context.Background(), `temporal static ( echo "hi" >=> echo )`)
	if err != nil {
		t.Fatalf("compile arrow: %v", err)
	}
	if len(plan.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(plan.Nodes))
	}
}

func TestCompileArrow_ComposesIntoLargerArrow(t *testing.T) {
	reg := script.DefaultRegistry()

	// Compose CompileArrow with a downstream arrow that counts nodes.
	// This is the real test of composability: the compile pipeline is
	// just an arrow, so it slots into Pipe like anything else.
	countNodes := weft.Arrow[sibyl.Plan, int](func(_ context.Context, p sibyl.Plan) (int, error) {
		return len(p.Nodes), nil
	})

	sourceToCount := weft.Pipe2(script.CompileArrow(reg), countNodes)

	n, err := sourceToCount(context.Background(), `temporal static ( echo "a" >=> echo "b" >=> echo "c" )`)
	if err != nil {
		t.Fatalf("composed arrow: %v", err)
	}
	if n != 3 {
		t.Errorf("composed pipeline counted %d nodes, want 3", n)
	}
}

func TestCompileArrow_ErrorShortCircuits(t *testing.T) {
	reg := script.DefaultRegistry()
	compile := script.CompileArrow(reg)

	// Unknown builtin: the error must propagate out of the composition
	// (Compose short-circuits on the first failing arrow), not panic or
	// produce a partial plan.
	_, err := compile(context.Background(), `temporal static ( teleport "mars" )`)
	if err == nil {
		t.Fatal("expected composition to short-circuit with an error")
	}
}
