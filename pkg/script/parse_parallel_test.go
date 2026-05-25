package script_test

import (
	"context"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// The new parser MUST support parenthesized parallel <*>, matching the
// original internal/agentscript grammar. This is a hard regression guard:
// <*> worked in the original runtime and must work in pkg/script too.
func TestParse_ParallelForms(t *testing.T) {
	cases := []string{
		`memory static ( ( a "x" <*> b "y" ) )`,
		`memory static ( ( a "x" <*> b "y" <*> c "z" ) )`,
		`memory static ( ( a "x" <*> b "y" ) >=> merge )`,
		`memory static ( ( a >=> b <*> c >=> d ) >=> merge >=> e "q" )`,
		`temporal static ( ( echo "x" <*> echo "y" ) )`,
	}
	for _, src := range cases {
		if _, err := script.Parse(context.Background(), script.Source(src)); err != nil {
			t.Errorf("parse failed for %q: %v", src, err)
		}
	}
}

// The parser must produce an ast.Parallel node for a multi-branch group.
// Block bodies are always wrapped in a Pipeline (the uniform invariant),
// so for `( ( a <*> b ) )` the body is a one-stage Pipeline whose stage
// is the Parallel.
func TestParse_ProducesParallelNode(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`memory static ( ( a "x" <*> b "y" ) )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Blocks) != 1 {
		t.Fatalf("blocks = %d", len(a.Blocks))
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

// A single-branch group is just grouping — its body must not contain a
// Parallel node.
func TestParse_SingleGroupIsNotParallel(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`memory static ( ( a "x" ) )`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pipe, ok := a.Blocks[0].Body.(ast.Pipeline)
	if !ok {
		t.Fatalf("body should be ast.Pipeline, got %T", a.Blocks[0].Body)
	}
	if _, isPar := pipe.Stages[0].(ast.Parallel); isPar {
		t.Error("single-branch group must not be a Parallel")
	}
}

// End to end: a parallel program parses AND resolves against the full
// registry (the complex grammar the CLI examples use).
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
