package script_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
)

// The contract this PR establishes:
//   - The registry is COMPLETE: every historical verb resolves (no
//     "unknown builtin" for a real verb).
//   - Availability is per-backend: historical verbs run on memory; a
//     known verb under temporal that isn't ported yields a distinct
//     NotImplementedOnBackendError — NEVER UnknownBuiltinError.
//   - Genuinely unknown names still fail as UnknownBuiltinError (safety
//     net preserved).

func TestCompleteRegistry_RecognizesAllHistoricalVerbs(t *testing.T) {
	r := script.CompleteRegistry()
	// A representative spread across the verb families.
	for _, v := range []string{
		"echo", "search", "summarize", "hf_summarize", "hf_translate",
		"mcp", "mcp_agent", "mcp_connect", "perplexity", "perplexity_pro",
		"weather", "stock", "crypto", "email", "calendar", "translate",
		"image_generate", "video_analyze", "rag_query", "kg_extract",
		"if", "foreach", "match", "fmap", "pfmap", "exec",
	} {
		if _, ok := r.Lookup(v); !ok {
			t.Errorf("CompleteRegistry should recognize historical verb %q", v)
		}
	}
}

func TestCompleteRegistry_MemoryVerbResolvesOnMemory(t *testing.T) {
	r := script.CompleteRegistry()
	// hf_summarize is memory-only; under a memory block it RESOLVES clean
	// (vocabulary + availability both pass). Note: memory *execution* is a
	// later step — here we assert resolution succeeds, which is this PR's
	// contract. Compile would invoke the temporal-only Finalize, so we
	// stop at Resolve.
	a, err := script.Parse(context.Background(), script.Source(`memory static ( hf_summarize "x" )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := script.Resolve(context.Background(), r, a); err != nil {
		t.Fatalf("memory-backed verb should resolve under memory: %v", err)
	}
}

func TestCompleteRegistry_MemoryVerbUnderTemporal_NotImplemented(t *testing.T) {
	r := script.CompleteRegistry()
	// hf_summarize under temporal: KNOWN verb, not available on temporal.
	// This is caught at RESOLVE (availability check), before Finalize.
	a, err := script.Parse(context.Background(), script.Source(`temporal static ( hf_summarize "x" )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = script.Resolve(context.Background(), r, a)
	if err == nil {
		t.Fatal("memory-only verb under temporal should fail at resolve")
	}
	var notImpl *script.NotImplementedOnBackendError
	if !errors.As(err, &notImpl) {
		t.Fatalf("expected NotImplementedOnBackendError, got %T: %v", err, err)
	}
	var unknown *script.UnknownBuiltinError
	if errors.As(err, &unknown) {
		t.Error("a known verb must NOT produce UnknownBuiltinError")
	}
	if notImpl.Builtin != "hf_summarize" || notImpl.Backend != "temporal" {
		t.Errorf("error fields = %q/%q, want hf_summarize/temporal", notImpl.Builtin, notImpl.Backend)
	}
}

func TestCompleteRegistry_EchoRunsOnTemporal(t *testing.T) {
	r := script.CompleteRegistry()
	// echo is the one verb on BOTH backends. It compiles end-to-end on
	// temporal (Finalize produces a Plan)...
	if _, err := script.Compile(context.Background(), r, script.Source(`temporal static ( echo "hi" )`)); err != nil {
		t.Errorf("echo should compile on temporal: %v", err)
	}
	// ...and resolves on memory (memory execution is a later step).
	a, _ := script.Parse(context.Background(), script.Source(`memory static ( echo "hi" )`))
	if _, err := script.Resolve(context.Background(), r, a); err != nil {
		t.Errorf("echo should resolve on memory: %v", err)
	}
}

func TestCompleteRegistry_SafetyNetUnknownStillUnknown(t *testing.T) {
	r := script.CompleteRegistry()
	// A genuinely unknown name (hallucination) must STILL be unknown,
	// on either backend — the safety net is preserved.
	for _, src := range []string{
		`temporal static ( teleport "mars" )`,
		`memory static ( teleport "mars" )`,
	} {
		_, err := script.Compile(context.Background(), r, script.Source(src))
		if err == nil {
			t.Fatalf("unknown verb should fail: %s", src)
		}
		var unknown *script.UnknownBuiltinError
		if !errors.As(err, &unknown) {
			t.Errorf("%s: expected UnknownBuiltinError, got %T", src, err)
		}
	}
}

func TestSupportsBackend_EmptyDefaultsToMemoryOnly(t *testing.T) {
	// A spec with no declared backends must be memory-only — the safe
	// default that prevents dispatching un-ported verbs to Temporal.
	s := registry.BuiltinSpec{Name: "x"}
	if !s.SupportsBackend(registry.BackendMemory) {
		t.Error("empty Backends should support memory")
	}
	if s.SupportsBackend(registry.BackendTemporal) {
		t.Error("empty Backends must NOT support temporal")
	}
}
