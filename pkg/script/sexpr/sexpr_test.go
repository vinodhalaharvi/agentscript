package sexpr

import "testing"

func TestReadAtoms(t *testing.T) {
	forms, err := Read(`alpha :backend "a string" 42 -1.5 github/clone`)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(forms) != 6 {
		t.Fatalf("got %d forms, want 6", len(forms))
	}
	want := []struct {
		kind Kind
		text string
		num  float64
	}{
		{KindSymbol, "alpha", 0},
		{KindKeyword, "backend", 0},
		{KindString, "a string", 0},
		{KindNumber, "42", 42},
		{KindNumber, "-1.5", -1.5},
		{KindSymbol, "github/clone", 0},
	}
	for i, w := range want {
		if forms[i].Kind != w.kind {
			t.Errorf("form %d: kind = %v, want %v", i, forms[i].Kind, w.kind)
		}
		if forms[i].Text != w.text {
			t.Errorf("form %d: text = %q, want %q", i, forms[i].Text, w.text)
		}
		if w.num != 0 && forms[i].Num != w.num {
			t.Errorf("form %d: num = %v, want %v", i, forms[i].Num, w.num)
		}
	}
}

func TestReadNestedList(t *testing.T) {
	forms, err := Read(`(pipe (search "ai") summarize)`)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(forms) != 1 {
		t.Fatalf("got %d forms, want 1", len(forms))
	}
	head, ok := forms[0].Head()
	if !ok || head != "pipe" {
		t.Fatalf("head = %q ok=%v, want pipe", head, ok)
	}
	if n := len(forms[0].Items); n != 3 {
		t.Fatalf("got %d items, want 3", n)
	}
	inner := forms[0].Items[1]
	if inner.Kind != KindList {
		t.Fatalf("item 1 kind = %v, want list", inner.Kind)
	}
	if got := inner.Items[1].Text; got != "ai" {
		t.Errorf("inner arg = %q, want ai", got)
	}
	if !forms[0].Items[2].IsSymbol("summarize") {
		t.Errorf("item 2 is not the symbol summarize")
	}
}

func TestReadComments(t *testing.T) {
	src := `
; lisp comment
// legacy comment
(pipe summarize) ; trailing
`
	forms, err := Read(src)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(forms) != 1 {
		t.Fatalf("got %d forms, want 1", len(forms))
	}
}

func TestReadStringEscapes(t *testing.T) {
	forms, err := Read(`(ask "say \"hi\"\nthen\tstop")`)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := forms[0].Items[1].Text
	want := "say \"hi\"\nthen\tstop"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string]string{
		"unclosed list":  `(pipe summarize`,
		"stray close":    `summarize)`,
		"unterminated":   `(ask "no end`,
		"bad escape":     `(ask "\q")`,
		"empty keyword":  `(block : memory)`,
		"dangling paren": `(pipe (search "a")`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(src); err == nil {
				t.Fatalf("Read(%q) succeeded, want error", src)
			}
		})
	}
}

func TestReadPositions(t *testing.T) {
	forms, err := Read("\n\n  (pipe\n    summarize)")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := forms[0].Pos; got.Line != 3 || got.Col != 3 {
		t.Errorf("list pos = %v, want 3:3", got)
	}
	if got := forms[0].Items[1].Pos; got.Line != 4 || got.Col != 5 {
		t.Errorf("symbol pos = %v, want 4:5", got)
	}
}

func TestReadEmpty(t *testing.T) {
	forms, err := Read("  ; nothing here\n")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(forms) != 0 {
		t.Fatalf("got %d forms, want 0", len(forms))
	}
}

func TestDetect(t *testing.T) {
	cases := map[string]bool{
		`(pipe summarize)`:                true,
		"\n; comment\n(block summarize)":  true,
		"// legacy comment\n(pipe a)":     true,
		`memory static ( search "a" )`:    false,
		"\n\nmemory static ( summarize )": false,
		``:                                false,
		`; only a comment`:                false,
	}
	for src, want := range cases {
		if got := Detect(src); got != want {
			t.Errorf("Detect(%q) = %v, want %v", src, got, want)
		}
	}
}
