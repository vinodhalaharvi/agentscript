package scriptmem

import (
	"context"
	"testing"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript"
	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// Build a resolved memory program by running the real front end
// (script.Parse → script.Resolve) against the complete registry, then
// assert the adapter maps it to the correct old *Program shape. Exercises
// the bridge's structural translation without needing a live runtime.
func resolveMemory(t *testing.T, src string) (prog *agentscript.Program, backend ast.Backend) {
	t.Helper()
	a, err := script.Parse(context.Background(), script.Source(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := script.Resolve(context.Background(), script.CompleteRegistry(), a)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	prog, backend, err = toProgram(r)
	if err != nil {
		t.Fatalf("toProgram: %v", err)
	}
	return prog, backend
}

func TestAdapter_SingleCall(t *testing.T) {
	prog, backend := resolveMemory(t, `memory static ( hf_summarize "the thread" )`)
	if backend != ast.BackendMemory {
		t.Fatalf("backend = %v, want memory", backend)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("statements = %d, want 1", len(prog.Statements))
	}
	cmd := prog.Statements[0].Command
	if cmd == nil {
		t.Fatal("expected a Command")
	}
	if cmd.Action != "hf_summarize" {
		t.Errorf("Action = %q, want hf_summarize", cmd.Action)
	}
	if cmd.Arg != "the thread" {
		t.Errorf("Arg = %q, want 'the thread'", cmd.Arg)
	}
}

func TestAdapter_PipelineChains(t *testing.T) {
	prog, _ := resolveMemory(t, `memory static ( search "x" >=> hf_summarize >=> echo )`)
	s := prog.Statements[0]
	names := []string{}
	for s != nil {
		if s.Command != nil {
			names = append(names, s.Command.Action)
		}
		s = s.Pipe
	}
	if len(names) != 3 || names[0] != "search" || names[1] != "hf_summarize" || names[2] != "echo" {
		t.Errorf("pipeline chain = %v, want [search hf_summarize echo]", names)
	}
}

func TestAdapter_RejectsTemporalBackend(t *testing.T) {
	a, _ := script.Parse(context.Background(), script.Source(`temporal static ( echo "x" )`))
	r, _ := script.Resolve(context.Background(), script.CompleteRegistry(), a)
	_, err := RunMemory(context.Background(), MemoryConfig{}, r)
	if err == nil {
		t.Fatal("RunMemory should reject a temporal-backend program")
	}
}
