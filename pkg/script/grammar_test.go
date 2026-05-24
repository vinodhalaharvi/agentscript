package script_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
)

func TestGrammar_ReturnsCompleteVocabulary(t *testing.T) {
	g := script.Grammar()
	if g.Registry == nil {
		t.Fatal("Grammar().Registry must not be nil")
	}
	if len(g.Verbs) < 100 {
		t.Errorf("Grammar should expose the full vocabulary; got %d verbs", len(g.Verbs))
	}
	if !sort.StringsAreSorted(g.Verbs) {
		t.Error("Grammar().Verbs should be sorted")
	}
	want := map[string]bool{"echo": false, "hf_summarize": false, "mcp_agent": false, "perplexity": false}
	for _, v := range g.Verbs {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for v, found := range want {
		if !found {
			t.Errorf("Grammar should advertise %q", v)
		}
	}
}

func TestGrammar_DescribesOperatorsAndBackends(t *testing.T) {
	g := script.Grammar()
	syms := map[string]bool{}
	for _, op := range g.Operators {
		syms[op.Symbol] = true
	}
	if !syms[">=>"] || !syms["<*>"] {
		t.Errorf("operators should include >=> and <*>, got %v", syms)
	}
	if len(g.Backends) != 2 {
		t.Errorf("expected memory+temporal backends, got %v", g.Backends)
	}
}

func TestGrammar_DiscoveryDrivenTranslateAndCompile(t *testing.T) {
	g := script.Grammar()
	llm := func(_ context.Context, _, _ string) (string, error) {
		return `temporal static ( echo "hi" )`, nil
	}
	src, err := script.TranslateGrammar(context.Background(), llm, g, "say hi")
	if err != nil {
		t.Fatalf("TranslateGrammar: %v", err)
	}
	plan, err := script.CompileGrammar(context.Background(), g, src)
	if err != nil {
		t.Fatalf("CompileGrammar: %v", err)
	}
	if len(plan.Nodes) != 1 {
		t.Errorf("nodes = %d, want 1", len(plan.Nodes))
	}
}

func TestGrammar_HistoricalVerbIsKnownNotUnknown(t *testing.T) {
	g := script.Grammar()
	llm := func(_ context.Context, _, _ string) (string, error) {
		return `temporal static ( hf_summarize "x" )`, nil
	}
	src, _ := script.TranslateGrammar(context.Background(), llm, g, "summarize")
	_, err := script.CompileGrammar(context.Background(), g, src)
	if err == nil {
		t.Fatal("hf_summarize on temporal should fail (not implemented there yet)")
	}
	var notImpl *script.NotImplementedOnBackendError
	if !errors.As(err, &notImpl) {
		t.Errorf("expected NotImplementedOnBackendError, got %T: %v", err, err)
	}
}
