package script_test

import (
	"context"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// Parallel <*> must work, with the block's OWN parens wrapping the
// parallel (bare-body), matching the original internal/agentscript
// grammar. This is the exact shape the LLM emits.
func TestParse_ParallelForms(t *testing.T) {
	cases := []string{
		`(block :backend memory :mode static
		  (par
		    (a "x")
		    (b "y")))`,
		`(block :backend memory :mode static
		  (par
		    (a "x")
		    (b "y")
		    (c "z")))`,
		`(block :backend memory :mode static
		  (pipe
		    (par
		      (a "x")
		      (b "y"))
		    merge))`,
		`(block :backend memory :mode static
		  (pipe
		    (par
		      (pipe
		        a
		        b)
		      (pipe
		        c
		        d))
		    merge
		    (e "q")))`,
		`(block :backend temporal :mode static
		  (par
		    (echo "x")
		    (echo "y")))`,
	}
	for _, src := range cases {
		if _, err := script.Parse(context.Background(), script.Source(src)); err != nil {
			t.Errorf("parse failed for %q: %v", src, err)
		}
	}
}

func TestParse_BareBodyParallel(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`(block :backend memory :mode static
	  (par
	    (a "x")
	    (b "y")))`))
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

func TestParse_SequentialIsNotParallel(t *testing.T) {
	a, err := script.Parse(context.Background(), script.Source(`(block :backend memory :mode static
	  (pipe
	    (a "x")
	    (b "y")))`))
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

func TestParse_ComplexGrammarResolves(t *testing.T) {
	src := `(block :backend memory :mode static
	  (pipe
	    (par
	      (pipe
	        (search "x")
	        (analyze "s"))
	      (pipe
	        (search "y")
	        (analyze "s")))
	    merge
	    (ask "who wins?")))`
	a, err := script.Parse(context.Background(), script.Source(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := script.Resolve(context.Background(), script.CompleteRegistry(), a); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}
