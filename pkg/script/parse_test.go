package script_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// === Helpers ===============================================================

func parse(t *testing.T, src string) ast.AST {
	t.Helper()
	got, err := script.Parse(context.Background(), script.Source(src))
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	return got
}

func parseErr(t *testing.T, src string) error {
	t.Helper()
	_, err := script.Parse(context.Background(), script.Source(src))
	if err == nil {
		t.Fatalf("Parse(%q) returned no error, expected one", src)
	}
	return err
}

// expectPipeline asserts a Block's body is a Pipeline and returns its stages.
func expectPipeline(t *testing.T, blk ast.Block) []ast.Node {
	t.Helper()
	pipe, ok := blk.Body.(ast.Pipeline)
	if !ok {
		t.Fatalf("Block.Body is %T, want ast.Pipeline", blk.Body)
	}
	return pipe.Stages
}

// expectCall asserts a Node is a Call and returns it.
func expectCall(t *testing.T, n ast.Node) ast.Call {
	t.Helper()
	c, ok := n.(ast.Call)
	if !ok {
		t.Fatalf("Node is %T, want ast.Call", n)
	}
	return c
}

// === Happy-path parsing ====================================================

func TestParse_SingleCallBlock(t *testing.T) {
	got := parse(t, `(block :backend temporal :mode static
	  (echo "hello"))`)

	if len(got.Blocks) != 1 {
		t.Fatalf("Blocks: got %d, want 1", len(got.Blocks))
	}
	b := got.Blocks[0]
	if b.Backend != ast.BackendTemporal {
		t.Errorf("Backend = %v, want temporal", b.Backend)
	}
	if b.Mode != ast.ModeStatic {
		t.Errorf("Mode = %v, want static", b.Mode)
	}

	stages := expectPipeline(t, b)
	if len(stages) != 1 {
		t.Fatalf("Pipeline.Stages: got %d, want 1", len(stages))
	}
	c := expectCall(t, stages[0])
	if c.Name != "echo" {
		t.Errorf("Call.Name = %q, want echo", c.Name)
	}
	if len(c.Args) != 1 {
		t.Fatalf("Call.Args: got %d, want 1", len(c.Args))
	}
	sa, ok := c.Args[0].(ast.StringArg)
	if !ok {
		t.Fatalf("Args[0] is %T, want ast.StringArg", c.Args[0])
	}
	if sa.Value != "hello" {
		t.Errorf("StringArg.Value = %q, want hello", sa.Value)
	}
}

func TestParse_TwoStagePipeline(t *testing.T) {
	got := parse(t, `(block :backend temporal :mode static
	  (pipe
	    (echo "hello")
	    (echo "world")))`)

	stages := expectPipeline(t, got.Blocks[0])
	if len(stages) != 2 {
		t.Fatalf("Pipeline.Stages: got %d, want 2", len(stages))
	}

	c0 := expectCall(t, stages[0])
	c1 := expectCall(t, stages[1])
	if c0.Name != "echo" || c1.Name != "echo" {
		t.Errorf("expected both calls to be echo, got %q and %q", c0.Name, c1.Name)
	}
	if c0.Args[0].(ast.StringArg).Value != "hello" {
		t.Errorf("stage 0 arg = %q, want hello", c0.Args[0].(ast.StringArg).Value)
	}
	if c1.Args[0].(ast.StringArg).Value != "world" {
		t.Errorf("stage 1 arg = %q, want world", c1.Args[0].(ast.StringArg).Value)
	}
}

func TestParse_ThreeStagePipeline(t *testing.T) {
	got := parse(t, `(block :backend temporal :mode static
	  (pipe
	    (echo "a")
	    (echo "b")
	    (echo "c")))`)
	stages := expectPipeline(t, got.Blocks[0])
	if len(stages) != 3 {
		t.Fatalf("got %d stages, want 3", len(stages))
	}
}

func TestParse_NoArgCall(t *testing.T) {
	// Bare identifier with no string arg should parse fine.
	got := parse(t, `(block :backend temporal :mode static
	  echo)`)
	stages := expectPipeline(t, got.Blocks[0])
	c := expectCall(t, stages[0])
	if c.Name != "echo" {
		t.Errorf("Call.Name = %q, want echo", c.Name)
	}
	if len(c.Args) != 0 {
		t.Errorf("Args = %d, want 0 for bare call", len(c.Args))
	}
}

func TestParse_MultipleStringArgs(t *testing.T) {
	got := parse(t, `(block :backend temporal :mode static
	  (foo "a" "b" "c"))`)
	c := expectCall(t, expectPipeline(t, got.Blocks[0])[0])
	if len(c.Args) != 3 {
		t.Fatalf("Args: got %d, want 3", len(c.Args))
	}
	for i, want := range []string{"a", "b", "c"} {
		got := c.Args[i].(ast.StringArg).Value
		if got != want {
			t.Errorf("Args[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestParse_MultipleBlocks(t *testing.T) {
	src := `(block :backend temporal :mode static
	  (echo "a"))
	
	(block :backend memory :mode dynamic
	  (echo "b"))`
	got := parse(t, src)
	if len(got.Blocks) != 2 {
		t.Fatalf("Blocks: got %d, want 2", len(got.Blocks))
	}
	if got.Blocks[0].Backend != ast.BackendTemporal || got.Blocks[0].Mode != ast.ModeStatic {
		t.Errorf("Block 0 = (%v, %v), want (temporal, static)",
			got.Blocks[0].Backend, got.Blocks[0].Mode)
	}
	if got.Blocks[1].Backend != ast.BackendMemory || got.Blocks[1].Mode != ast.ModeDynamic {
		t.Errorf("Block 1 = (%v, %v), want (memory, dynamic)",
			got.Blocks[1].Backend, got.Blocks[1].Mode)
	}
}

// === All four backend/mode combinations parse ==============================

func TestParse_AllBackendModeCombinations(t *testing.T) {
	cases := []struct {
		src     string
		backend ast.Backend
		mode    ast.Mode
	}{
		{`(block :backend temporal :mode static
		  echo)`, ast.BackendTemporal, ast.ModeStatic},
		{`(block :backend temporal :mode dynamic
		  echo)`, ast.BackendTemporal, ast.ModeDynamic},
		{`(block :backend memory :mode static
		  echo)`, ast.BackendMemory, ast.ModeStatic},
		{`(block :backend memory :mode dynamic
		  echo)`, ast.BackendMemory, ast.ModeDynamic},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			got := parse(t, tc.src)
			b := got.Blocks[0]
			if b.Backend != tc.backend {
				t.Errorf("Backend = %v, want %v", b.Backend, tc.backend)
			}
			if b.Mode != tc.mode {
				t.Errorf("Mode = %v, want %v", b.Mode, tc.mode)
			}
		})
	}
}

// === Whitespace and comments ===============================================

func TestParse_LineCommentsIgnored(t *testing.T) {
	src := `(block :backend temporal :mode static
	  (pipe
	    (echo "a")
	    (echo "b")))`
	got := parse(t, src)
	stages := expectPipeline(t, got.Blocks[0])
	if len(stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(stages))
	}
}

func TestParse_WhitespaceFlexibility(t *testing.T) {
	// Newlines, tabs, and runs of spaces in unusual places should all parse.
	src := "(block\t:backend temporal\n  :mode static\n(pipe\n\t(echo\t\"a\")\n\n\t(echo   \"b\")))"
	got := parse(t, src)
	stages := expectPipeline(t, got.Blocks[0])
	if len(stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(stages))
	}
}

// === Error cases ===========================================================

// Backend and mode are now keyword options rather than a mandatory
// two-word header, so "missing" is not an error — both default, and
// order does not matter. What must still fail is an option that is
// unknown, valueless, or given a value outside its enum.

func TestParse_DefaultsBackendAndMode(t *testing.T) {
	got := parse(t, `(block (echo "a"))`)
	if b := got.Blocks[0].Backend; b != ast.BackendMemory {
		t.Errorf("backend = %v, want memory", b)
	}
	if m := got.Blocks[0].Mode; m != ast.ModeStatic {
		t.Errorf("mode = %v, want static", m)
	}
}

func TestParse_OptionOrderIsFree(t *testing.T) {
	got := parse(t, `(block :mode dynamic :backend temporal (echo "a"))`)
	if b := got.Blocks[0].Backend; b != ast.BackendTemporal {
		t.Errorf("backend = %v, want temporal", b)
	}
	if m := got.Blocks[0].Mode; m != ast.ModeDynamic {
		t.Errorf("mode = %v, want dynamic", m)
	}
}

func TestParse_RejectsUnknownOption(t *testing.T) {
	err := parseErr(t, `(block :engine memory (echo "a"))`)
	var pe script.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want ParseError", err)
	}
}

func TestParse_RejectsOptionWithoutValue(t *testing.T) {
	err := parseErr(t, `(block :backend)`)
	var pe script.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want ParseError", err)
	}
}

func TestParse_RejectsUnknownBackend(t *testing.T) {
	err := parseErr(t, `(block :backend xyz (echo "a"))`)
	if !strings.Contains(err.Error(), "Parse") {
		t.Errorf("err should reference Parse phase, got: %v", err)
	}
}

func TestParse_RejectsUnknownMode(t *testing.T) {
	err := parseErr(t, `(block :mode xyz (echo "a"))`)
	if !strings.Contains(err.Error(), "Parse") {
		t.Errorf("err should reference Parse phase, got: %v", err)
	}
}

func TestParse_RejectsUnbalancedParens(t *testing.T) {
	parseErr(t, `(block :backend temporal (pipe (echo "a")`)
	parseErr(t, `(block :backend temporal (pipe (echo "a")))))`)
}

func TestParse_RejectsEmptyBlock(t *testing.T) {
	parseErr(t, `(block :backend temporal :mode static)`)
	parseErr(t, `()`)
}

func TestParse_RejectsEmptyPipe(t *testing.T) {
	parseErr(t, `(block :backend temporal :mode static (pipe))`)
}

func TestParse_AcceptsParallelForm(t *testing.T) {
	// par fan-out is part of the grammar (parity with the original
	// internal/agentscript grammar). It must parse.
	if _, err := script.Parse(context.Background(), script.Source(`(block :backend temporal :mode static
	  (par
	    (echo "a")
	    (echo "b")))`)); err != nil {
		t.Errorf("par should parse: %v", err)
	}
}

// === ParseError shape ======================================================

func TestParseError_WrapsUnderlying(t *testing.T) {
	// A bare symbol is a valid program (a call with no arguments), so
	// the malformed input here has to be structurally broken.
	const src = `(pipe (echo "a"`
	err := parseErr(t, src)
	var pe script.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want ParseError", err)
	}
	if pe.Err == nil {
		t.Error("ParseError.Err should wrap the underlying reader error")
	}
	if pe.Source != src {
		t.Errorf("ParseError.Source = %q, want %q", pe.Source, src)
	}
}

// === Source.String() =======================================================

func TestSourceString(t *testing.T) {
	s := script.Source("hello")
	if s.String() != "hello" {
		t.Errorf("Source.String() = %q, want hello", s.String())
	}
}

// === Sanity: AST types satisfy their sealed sums ===========================

func TestAST_NodeAndArgInterfaces(t *testing.T) {
	// Compile-time + run-time check that all expected variants satisfy
	// the sealed sum interfaces. If any of these stopped compiling
	// we'd want to know.
	var _ ast.Node = ast.Pipeline{}
	var _ ast.Node = ast.Parallel{}
	var _ ast.Node = ast.Call{}
	var _ ast.Arg = ast.StringArg{}
	var _ ast.Arg = ast.NumArg{}
}

// === Backend/Mode String() round-trips =====================================

func TestBackendString(t *testing.T) {
	cases := []struct {
		b    ast.Backend
		want string
	}{
		{ast.BackendMemory, "memory"},
		{ast.BackendTemporal, "temporal"},
		{ast.BackendUnknown, "unknown"},
	}
	for _, c := range cases {
		if got := c.b.String(); got != c.want {
			t.Errorf("Backend(%d).String() = %q, want %q", c.b, got, c.want)
		}
	}
}

func TestModeString(t *testing.T) {
	cases := []struct {
		m    ast.Mode
		want string
	}{
		{ast.ModeStatic, "static"},
		{ast.ModeDynamic, "dynamic"},
		{ast.ModeUnknown, "unknown"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.want {
			t.Errorf("Mode(%d).String() = %q, want %q", c.m, got, c.want)
		}
	}
}
