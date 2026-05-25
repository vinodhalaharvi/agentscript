package script_test

import (
	"context"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// The parser MUST support parallel <*>, matching the original
// internal/agentscript grammar. The block's own parens wrap the parallel
// (no redundant inner parens), exactly like the original runtime.
func TestParse_ParallelForms(t *testing.T) {
	cases := []string{
		`memory static ( a "x" <*> b "y" )`,                             // bare-body parallel (the Slack shape)
		`memory static ( a "x" <*> b "y" <*> c "z" )`,                   // 3-way
		`memory static ( ( a "x" <*> b "y" ) >=> merge )`,               // parallel group then pipe
		`memory static ( ( a >=> b <*> c >=> d ) >=> merge >=> e "q" )`, // complex
		`temporal static ( echo "x" <*> echo "y" )`,
	}
	for _, src := range cases {
		if _, err := script.Parse(context.Background(), script.Source(src)); err != nil {
			t.Errorf("parse failed for %q: %v", src, err)
		}
	}
}

// Bare-body parallel: the block body is a one-stage Pipeline whose stage
// is the Parallel (Block.Body is always a Pipeline — the invariant).
func TestParse_BareBodyParallel(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`memory static ( a "x" <*> b "y" )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pipe, ok := a.Blocks[0].Body.(ast.Pipeline)
	if !ok {
		t.Fatalf("body should be ast.Pipeline, got %T", a.Blocks[0].Body)
	}
	if len(pipe.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(pipe.Stages))
	}
	par, ok := pipe.Stages[0].(ast.Parallel)
	if !ok {
		t.Fatalf("stage should be ast.Parallel, got %T", pipe.Stages[0])
	}
	if len(par.Branches) != 2 {
		t.Errorf("branches = %d, want 2", len(par.Branches))
	}
}

// A plain sequential body must NOT contain a Parallel.
func TestParse_SequentialIsNotParallel(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`memory static ( a "x" >=> b "y" )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pipe := a.Blocks[0].Body.(ast.Pipeline)
	for _, s := range pipe.Stages {
		if _, isPar := s.(ast.Parallel); isPar {
			t.Error("sequential pipeline must not contain a Parallel stage")
		}
	}
}

// End to end: the full complex grammar parses AND resolves.
func TestParse_ComplexGrammarResolves(t *testing.T) {
	src := `memory static ( ( search "x" >=> analyze "s" <*> search "y" >=> analyze "s" ) >=> merge >=> ask "who wins?" )`
	a, err := script.Parse(context.Background(), script.Source(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := script.Resolve(context.Background(), script.CompleteRegistry(), a); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}
