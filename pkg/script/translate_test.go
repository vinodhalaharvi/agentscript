package script_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
)

// stubLLM returns a CompleteFunc that always yields the given output.
func stubLLM(out string) script.CompleteFunc {
	return func(_ context.Context, _, _ string) (string, error) { return out, nil }
}

func TestBuildPrompt_ListsBuiltins(t *testing.T) {
	p := script.BuildPrompt(script.DefaultRegistry())
	if !strings.Contains(p, "echo") {
		t.Error("prompt should list the echo builtin")
	}
	if !strings.Contains(p, ">=>") {
		t.Error("prompt should describe the >=> sequential operator")
	}
	// Must NOT teach the old grammar.
	if strings.Contains(p, "->") && !strings.Contains(p, ">=>") {
		t.Error("prompt should not use the legacy -> pipe")
	}
}

func TestBuildPrompt_NilRegistry(t *testing.T) {
	// Should not panic; lists nothing.
	p := script.BuildPrompt(nil)
	if !strings.Contains(p, "(none)") {
		t.Error("nil registry should yield (none) for available commands")
	}
}

func TestTranslate_StripsFences(t *testing.T) {
	llm := stubLLM("```agentscript\ntemporal static ( echo \"hi\" )\n```")
	src, err := script.Translate(context.Background(), llm, script.DefaultRegistry(), "say hi")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if strings.Contains(string(src), "```") {
		t.Errorf("fences not stripped: %q", src)
	}
	if !strings.HasPrefix(string(src), "temporal static") {
		t.Errorf("Source = %q, want it to start with the block", src)
	}
}

func TestTranslate_StripsSurroundingProse(t *testing.T) {
	llm := stubLLM("Sure! Here you go:\ntemporal static ( echo \"hi\" )\nHope that helps!")
	src, err := script.Translate(context.Background(), llm, script.DefaultRegistry(), "say hi")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if !strings.HasPrefix(string(src), "temporal static") {
		t.Errorf("leading prose not stripped: %q", src)
	}
	if strings.Contains(string(src), "Hope that helps") {
		t.Errorf("trailing prose not stripped: %q", src)
	}
}

func TestTranslate_NilLLM(t *testing.T) {
	_, err := script.Translate(context.Background(), nil, script.DefaultRegistry(), "x")
	if err == nil {
		t.Fatal("expected error for nil CompleteFunc")
	}
}

func TestTranslate_LLMError(t *testing.T) {
	llm := func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("network down")
	}
	_, err := script.Translate(context.Background(), llm, script.DefaultRegistry(), "x")
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

// === TranslateAndCompile: the full prose → validated Plan path ============

func TestTranslateAndCompile_HappyPath(t *testing.T) {
	llm := stubLLM(`temporal static ( echo "hello" >=> echo )`)
	plan, err := script.TranslateAndCompile(context.Background(), llm, script.DefaultRegistry(), "say hello then echo it")
	if err != nil {
		t.Fatalf("TranslateAndCompile: %v", err)
	}
	if len(plan.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(plan.Nodes))
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("produced plan should be valid: %v", err)
	}
}

// The safety net: if the LLM emits a command that isn't a builtin,
// TranslateAndCompile must fail at the compile step — nothing executes.
func TestTranslateAndCompile_RejectsUnknownCommand(t *testing.T) {
	llm := stubLLM(`temporal static ( teleport "mars" )`)
	_, err := script.TranslateAndCompile(context.Background(), llm, script.DefaultRegistry(), "teleport me")
	if err == nil {
		t.Fatal("SAFETY NET FAILED: unknown command compiled without error")
	}
	// It should be a resolve/compile error, surfaced clearly.
	if !strings.Contains(strings.ToLower(err.Error()), "teleport") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "builtin") {
		t.Errorf("error should point at the unknown command, got: %v", err)
	}
}

func TestTranslateAndCompile_RejectsMalformed(t *testing.T) {
	llm := stubLLM(`this is not agentscript at all`)
	_, err := script.TranslateAndCompile(context.Background(), llm, script.DefaultRegistry(), "garbage")
	if err == nil {
		t.Fatal("malformed DSL should fail to compile")
	}
}
