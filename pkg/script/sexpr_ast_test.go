package script

import (
	"context"
	"reflect"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

func TestParseSExprSimplePipe(t *testing.T) {
	got, err := ParseSExpr(context.Background(), Source(`
(pipe (search "AI trends")
      summarize
      (email "you@gmail.com"))
`))
	if err != nil {
		t.Fatalf("ParseSExpr: %v", err)
	}
	want := ast.AST{Blocks: []ast.Block{{
		Backend: ast.BackendMemory,
		Mode:    ast.ModeStatic,
		Body: ast.Pipeline{Stages: []ast.Node{
			ast.Call{Name: "search", Args: []ast.Arg{ast.StringArg{Value: "AI trends"}}},
			ast.Call{Name: "summarize", Args: []ast.Arg{}},
			ast.Call{Name: "email", Args: []ast.Arg{ast.StringArg{Value: "you@gmail.com"}}},
		}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

func TestParseSExprBlockOptions(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		backend ast.Backend
		mode    ast.Mode
	}{
		{"defaults", `(pipe summarize)`, ast.BackendMemory, ast.ModeStatic},
		{"explicit memory", `(block :backend memory :mode static (pipe summarize))`,
			ast.BackendMemory, ast.ModeStatic},
		{"temporal", `(block :backend temporal (pipe summarize))`,
			ast.BackendTemporal, ast.ModeStatic},
		{"sibyl alias", `(block :backend sibyl (pipe summarize))`,
			ast.BackendTemporal, ast.ModeStatic},
		{"script alias", `(script :mode dynamic (pipe summarize))`,
			ast.BackendMemory, ast.ModeDynamic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSExpr(context.Background(), Source(tc.src))
			if err != nil {
				t.Fatalf("ParseSExpr: %v", err)
			}
			b := got.Blocks[0]
			if b.Backend != tc.backend {
				t.Errorf("backend = %v, want %v", b.Backend, tc.backend)
			}
			if b.Mode != tc.mode {
				t.Errorf("mode = %v, want %v", b.Mode, tc.mode)
			}
		})
	}
}

func TestParseSExprWhenAliasesIf(t *testing.T) {
	got, err := ParseSExpr(context.Background(),
		Source(`(pipe (stock "NVDA") (when "change > 5") (notify "slack"))`))
	if err != nil {
		t.Fatalf("ParseSExpr: %v", err)
	}
	pipe := got.Blocks[0].Body.(ast.Pipeline)
	call := pipe.Stages[1].(ast.Call)
	if call.Name != "if" {
		t.Errorf("verb = %q, want if", call.Name)
	}
}

func TestParseSExprBareBodyIsWrapped(t *testing.T) {
	got, err := ParseSExpr(context.Background(), Source(`summarize`))
	if err != nil {
		t.Fatalf("ParseSExpr: %v", err)
	}
	body, ok := got.Blocks[0].Body.(ast.Pipeline)
	if !ok {
		t.Fatalf("body is %T, want ast.Pipeline", got.Blocks[0].Body)
	}
	if len(body.Stages) != 1 {
		t.Fatalf("got %d stages, want 1", len(body.Stages))
	}
}

func TestParseSExprParallelBody(t *testing.T) {
	got, err := ParseSExpr(context.Background(),
		Source(`(par (rss "hn") (reddit "r/golang"))`))
	if err != nil {
		t.Fatalf("ParseSExpr: %v", err)
	}
	body := got.Blocks[0].Body.(ast.Pipeline)
	if len(body.Stages) != 1 {
		t.Fatalf("got %d stages, want 1", len(body.Stages))
	}
	par, ok := body.Stages[0].(ast.Parallel)
	if !ok {
		t.Fatalf("stage is %T, want ast.Parallel", body.Stages[0])
	}
	for i, br := range par.Branches {
		if _, ok := br.(ast.Pipeline); !ok {
			t.Errorf("branch %d is %T, want ast.Pipeline", i, br)
		}
	}
}

func TestParseSExprNumericArg(t *testing.T) {
	got, err := ParseSExpr(context.Background(), Source(`(retry 3)`))
	if err != nil {
		t.Fatalf("ParseSExpr: %v", err)
	}
	call := got.Blocks[0].Body.(ast.Pipeline).Stages[0].(ast.Call)
	num, ok := call.Args[0].(ast.NumArg)
	if !ok {
		t.Fatalf("arg is %T, want ast.NumArg", call.Args[0])
	}
	if num.Value != 3 {
		t.Errorf("value = %v, want 3", num.Value)
	}
}

func TestParseSExprErrors(t *testing.T) {
	cases := map[string]string{
		"empty script":       ``,
		"empty form":         `()`,
		"pipe with no stage": `(pipe)`,
		"par with one":       `(par (rss "hn"))`,
		"unknown option":     `(block :engine memory (pipe summarize))`,
		"bad backend":        `(block :backend k8s (pipe summarize))`,
		"bad mode":           `(block :mode eager (pipe summarize))`,
		"dangling option":    `(block :backend)`,
		"two body forms":     `(block (pipe a) (pipe b))`,
		"no body":            `(block :backend memory)`,
		"nested block":       `(pipe (block (pipe a)))`,
		"non-literal arg":    `(email (search "x"))`,
		"keyword head":       `(:backend memory)`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSExpr(context.Background(), Source(src)); err == nil {
				t.Fatalf("ParseSExpr(%q) succeeded, want error", src)
			}
		})
	}
}

func TestParseDispatchesByDialect(t *testing.T) {
	ctx := context.Background()
	legacy, err := Parse(ctx, Source(`memory static ( search "ai" >=> summarize )`))
	if err != nil {
		t.Fatalf("Parse legacy: %v", err)
	}
	sexprAST, err := Parse(ctx, Source(`(pipe (search "ai") summarize)`))
	if err != nil {
		t.Fatalf("Parse s-expr: %v", err)
	}
	if !reflect.DeepEqual(legacy, sexprAST) {
		t.Errorf("dialects disagree:\nlegacy %#v\nsexpr  %#v", legacy, sexprAST)
	}
}

// TestRoundTrip is the real check on the front end: take a legacy
// script, parse it, print it as s-expressions, reparse, and require an
// identical AST. It doubles as the conversion tool's correctness test.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	cases := map[string]string{
		"single call": `memory static ( summarize )`,
		"pipeline":    `memory static ( search "ai" >=> summarize >=> email "a@b.c" )`,
		"fanout":      `memory static ( ( rss "hn" <*> reddit "r/golang" ) >=> merge )`,
		"body fanout": `memory static ( rss "hn" <*> reddit "r/golang" )`,
		"temporal":    `temporal dynamic ( search "ai" >=> summarize )`,
		"conditional": `memory static ( stock "NVDA" >=> if "change > 5" >=> notify "slack" )`,
		"multi block": `memory static ( summarize )
temporal static ( search "ai" >=> merge )`,
		"nested fanout": `memory static (
			( ( search "react" >=> analyze <*> search "vue" >=> analyze ) >=> merge
			  <*> ( search "go" >=> analyze <*> search "rust" >=> analyze ) >=> merge
			) >=> ask "compare" >=> save "out.md" )`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			original, err := Parse(ctx, Source(src))
			if err != nil {
				t.Fatalf("parse legacy: %v", err)
			}
			printed, err := PrintSExpr(original)
			if err != nil {
				t.Fatalf("print: %v", err)
			}
			reparsed, err := ParseSExpr(ctx, Source(printed))
			if err != nil {
				t.Fatalf("reparse:\n%s\nerror: %v", printed, err)
			}
			if !reflect.DeepEqual(original, reparsed) {
				t.Errorf("round trip changed the AST\nprinted:\n%s\noriginal %#v\nreparsed %#v",
					printed, original, reparsed)
			}
		})
	}
}

// TestPrintSExprEscapes covers escaped string literals, which only the
// s-expression dialect can express: the legacy lexer's string token is
// `"[^"]*"` and has no escape support at all.
func TestPrintSExprEscapes(t *testing.T) {
	ctx := context.Background()
	src := `(ask "say \"hi\"\nthen stop")`
	first, err := ParseSExpr(ctx, Source(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	printed, err := PrintSExpr(first)
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	second, err := ParseSExpr(ctx, Source(printed))
	if err != nil {
		t.Fatalf("reparse:\n%s\nerror: %v", printed, err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("escapes did not survive the round trip:\n%s", printed)
	}
}

// TestPrintStable checks that printing is idempotent: printing a
// reparsed AST yields byte-identical source.
func TestPrintStable(t *testing.T) {
	ctx := context.Background()
	src := `(block :backend sibyl :mode dynamic
  (pipe
    (par
      (rss "hn")
      (reddit "r/golang"))
    merge
    (when "count > 3")
    (email "a@b.c")))`
	first, err := ParseSExpr(ctx, Source(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out1, err := PrintSExpr(first)
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	second, err := ParseSExpr(ctx, Source(out1))
	if err != nil {
		t.Fatalf("reparse:\n%s\nerror: %v", out1, err)
	}
	out2, err := PrintSExpr(second)
	if err != nil {
		t.Fatalf("reprint: %v", err)
	}
	if out1 != out2 {
		t.Errorf("printing is not idempotent:\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("reparse changed the AST")
	}
}
