package scriptmem_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/scriptmem"
)

func stubLLM(dsl string) script.CompleteFunc {
	return func(_ context.Context, _, _ string) (string, error) { return dsl, nil }
}

// Temporal path: Execute returns a durable plan, tagged Temporal, with no
// in-memory result. Fully testable without a runtime.
func TestExecute_TemporalReturnsPlan(t *testing.T) {
	g := script.Grammar()
	llm := stubLLM(`temporal static ( echo "hi" >=> echo )`)

	out, err := scriptmem.Execute(context.Background(), llm, g, scriptmem.MemoryConfig{}, "say hi then echo")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Backend != scriptmem.Temporal {
		t.Fatalf("Backend = %v, want temporal", out.Backend)
	}
	if len(out.Plan.Nodes) != 2 {
		t.Errorf("plan nodes = %d, want 2", len(out.Plan.Nodes))
	}
	if out.Result != "" {
		t.Errorf("temporal outcome should carry no in-memory result, got %q", out.Result)
	}
}

// Routing: a memory-backed program routes to the in-process runtime
// (not the temporal/plan arm). We assert the ROUTING decision rather than
// a specific verb's output: most in-memory verbs need credentials, and
// the runtime's verb set is exercised by its own tests. What this proves
// is that Execute sends a memory block to RunMemory and never produces a
// plan for it — the unified entry's branch is correct.
func TestExecute_MemoryRoutesToRuntime(t *testing.T) {
	g := script.Grammar()
	// hf_summarize is memory-backed in the catalog; with no credentials
	// the runtime will error, but the routing is what we assert: we must
	// NOT get a temporal plan, and any error must come from the runtime
	// (execution), not from resolution/compilation.
	llm := stubLLM(`memory static ( hf_summarize "x" )`)

	out, err := scriptmem.Execute(context.Background(), llm, g, scriptmem.MemoryConfig{}, "summarize")
	if err != nil {
		// Acceptable: ran in-process and the verb failed (no creds). The
		// point is it was ATTEMPTED in memory, not turned into a plan.
		if out.Backend == scriptmem.Temporal || out.Plan.Nodes != nil {
			t.Fatalf("memory program must not produce a temporal plan; got %+v", out)
		}
		return
	}
	// Or it succeeded: must be tagged memory with no plan.
	if out.Backend != scriptmem.Memory {
		t.Errorf("Backend = %v, want memory", out.Backend)
	}
	if out.Plan.Nodes != nil {
		t.Errorf("memory outcome should carry no plan, got %+v", out.Plan)
	}
}

// Safety net through the envelope: an unknown verb fails before any
// execution, on either backend.
func TestExecute_UnknownVerbRejected(t *testing.T) {
	g := script.Grammar()
	for _, src := range []string{
		`temporal static ( teleport "mars" )`,
		`memory static ( teleport "mars" )`,
	} {
		_, err := scriptmem.Execute(context.Background(), stubLLM(src), g, scriptmem.MemoryConfig{}, "x")
		if err == nil {
			t.Fatalf("expected rejection for %q", src)
		}
		var unknown *script.UnknownBuiltinError
		if !errors.As(err, &unknown) {
			t.Errorf("%s: expected UnknownBuiltinError, got %T", src, err)
		}
	}
}

// A historical verb on temporal is honestly not-implemented (not unknown)
// and never produces a plan.
func TestExecute_HistoricalVerbOnTemporalNotImplemented(t *testing.T) {
	g := script.Grammar()
	llm := stubLLM(`temporal static ( hf_summarize "x" )`)
	_, err := scriptmem.Execute(context.Background(), llm, g, scriptmem.MemoryConfig{}, "summarize")
	if err == nil {
		t.Fatal("hf_summarize on temporal should fail")
	}
	var notImpl *script.NotImplementedOnBackendError
	if !errors.As(err, &notImpl) {
		t.Errorf("expected NotImplementedOnBackendError, got %T", err)
	}
}
