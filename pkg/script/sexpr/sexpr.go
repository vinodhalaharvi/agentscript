// Package sexpr implements the s-expression reader: text in, a generic
// Form tree out.
//
// This is deliberately the *only* thing this package does. It knows
// nothing about AgentScript verbs, pipelines, backends, or the AST. The
// mapping from Form to ast lives in the parent script package
// (sexpr_ast.go), so adding a surface form later never touches the
// tokenizer, and the reader can be tested without pulling in the
// translator.
//
// The grammar is the usual one:
//
//	form    = list | atom
//	list    = "(" form* ")"
//	atom    = string | number | keyword | symbol
//	string  = '"' ( escape | . )* '"'
//	keyword = ":" symbol-chars
//	symbol  = any run of non-delimiter chars
//
// Comments run to end of line and start with ';' (lisp convention) or
// '//' (what the legacy AgentScript lexer used). Both are accepted so a
// converted script keeps its comments either way.
package sexpr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// === Forms =================================================================

// Kind discriminates the Form variants.
type Kind int

const (
	// KindInvalid is the zero value; never produced by a successful read.
	KindInvalid Kind = iota
	// KindList is a parenthesized sequence of forms.
	KindList
	// KindSymbol is a bare identifier: pipe, summarize, github/clone.
	KindSymbol
	// KindKeyword is a colon-prefixed option name: :backend. Text holds
	// the name without the colon.
	KindKeyword
	// KindString is a double-quoted literal. Text holds the unescaped value.
	KindString
	// KindNumber is a numeric literal. Num holds the value, Text the
	// source spelling.
	KindNumber
)

// String implements fmt.Stringer for error messages.
func (k Kind) String() string {
	switch k {
	case KindList:
		return "list"
	case KindSymbol:
		return "symbol"
	case KindKeyword:
		return "keyword"
	case KindString:
		return "string"
	case KindNumber:
		return "number"
	default:
		return "invalid"
	}
}

// Pos is a 1-based line and column in the source text.
type Pos struct {
	Line int
	Col  int
}

// String implements fmt.Stringer, rendering as "line:col".
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Form is one node of the generic s-expression tree. Which fields are
// meaningful depends on Kind:
//
//	KindList             → Items
//	KindSymbol, Keyword  → Text
//	KindString           → Text (unescaped)
//	KindNumber           → Num, Text
type Form struct {
	Kind  Kind
	Pos   Pos
	Text  string
	Num   float64
	Items []Form
}

// Head returns the leading symbol of a list form. The second result is
// false for non-lists, empty lists, and lists whose first element is not
// a symbol.
func (f Form) Head() (string, bool) {
	if f.Kind != KindList || len(f.Items) == 0 {
		return "", false
	}
	if f.Items[0].Kind != KindSymbol {
		return "", false
	}
	return f.Items[0].Text, true
}

// IsSymbol reports whether f is the named symbol.
func (f Form) IsSymbol(name string) bool {
	return f.Kind == KindSymbol && f.Text == name
}

// === Errors ================================================================

// SyntaxError is a read failure with the position that caused it.
type SyntaxError struct {
	Pos Pos
	Msg string
}

// Error implements error.
func (e SyntaxError) Error() string {
	return fmt.Sprintf("sexpr: %s: %s", e.Pos, e.Msg)
}

// === Reader ================================================================

// Read parses source text into a sequence of top-level forms. An empty
// (or comment-only) source reads as zero forms without error; rejecting
// that is the caller's business.
func Read(src string) ([]Form, error) {
	r := &reader{src: []rune(src), line: 1, col: 1}
	var out []Form
	for {
		r.skipSpace()
		if r.eof() {
			return out, nil
		}
		f, err := r.readForm()
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
}

// Detect reports whether src is s-expression dialect rather than the
// legacy operator dialect. The two are trivially distinguishable: a
// legacy block opens with the backend keyword (an identifier), an
// s-expression script opens with '('. Comments and whitespace are
// skipped first.
//
// This exists so Parse can accept both during the conversion window
// without a flag, a file extension, or a pragma.
func Detect(src string) bool {
	r := &reader{src: []rune(src), line: 1, col: 1}
	r.skipSpace()
	return !r.eof() && r.peek() == '('
}

type reader struct {
	src  []rune
	i    int
	line int
	col  int
}

func (r *reader) eof() bool { return r.i >= len(r.src) }

func (r *reader) peek() rune {
	if r.eof() {
		return 0
	}
	return r.src[r.i]
}

func (r *reader) pos() Pos { return Pos{Line: r.line, Col: r.col} }

func (r *reader) next() rune {
	ch := r.src[r.i]
	r.i++
	if ch == '\n' {
		r.line++
		r.col = 1
	} else {
		r.col++
	}
	return ch
}

// skipSpace consumes whitespace and line comments (';' or '//').
func (r *reader) skipSpace() {
	for !r.eof() {
		ch := r.peek()
		switch {
		case unicode.IsSpace(ch):
			r.next()
		case ch == ';':
			r.skipLine()
		case ch == '/' && r.i+1 < len(r.src) && r.src[r.i+1] == '/':
			r.skipLine()
		default:
			return
		}
	}
}

func (r *reader) skipLine() {
	for !r.eof() && r.peek() != '\n' {
		r.next()
	}
}

func (r *reader) readForm() (Form, error) {
	r.skipSpace()
	if r.eof() {
		return Form{}, SyntaxError{Pos: r.pos(), Msg: "unexpected end of input"}
	}
	switch ch := r.peek(); {
	case ch == '(':
		return r.readList()
	case ch == ')':
		return Form{}, SyntaxError{Pos: r.pos(), Msg: "unexpected ')'"}
	case ch == '"':
		return r.readString()
	default:
		return r.readAtom()
	}
}

func (r *reader) readList() (Form, error) {
	start := r.pos()
	r.next() // consume '('
	list := Form{Kind: KindList, Pos: start}
	for {
		r.skipSpace()
		if r.eof() {
			return Form{}, SyntaxError{Pos: start, Msg: "unclosed '('"}
		}
		if r.peek() == ')' {
			r.next()
			return list, nil
		}
		item, err := r.readForm()
		if err != nil {
			return Form{}, err
		}
		list.Items = append(list.Items, item)
	}
}

func (r *reader) readString() (Form, error) {
	start := r.pos()
	r.next() // consume opening quote
	var sb strings.Builder
	for {
		if r.eof() {
			return Form{}, SyntaxError{Pos: start, Msg: "unterminated string"}
		}
		ch := r.next()
		if ch == '"' {
			return Form{Kind: KindString, Pos: start, Text: sb.String()}, nil
		}
		if ch != '\\' {
			sb.WriteRune(ch)
			continue
		}
		if r.eof() {
			return Form{}, SyntaxError{Pos: start, Msg: "unterminated escape sequence"}
		}
		esc := r.next()
		switch esc {
		case 'n':
			sb.WriteRune('\n')
		case 't':
			sb.WriteRune('\t')
		case 'r':
			sb.WriteRune('\r')
		case '"':
			sb.WriteRune('"')
		case '\\':
			sb.WriteRune('\\')
		default:
			return Form{}, SyntaxError{
				Pos: r.pos(),
				Msg: fmt.Sprintf("unknown escape sequence \\%s", string(esc)),
			}
		}
	}
}

func (r *reader) readAtom() (Form, error) {
	start := r.pos()
	var sb strings.Builder
	for !r.eof() && !isDelim(r.peek()) {
		sb.WriteRune(r.next())
	}
	text := sb.String()
	if text == "" {
		return Form{}, SyntaxError{Pos: start, Msg: "empty atom"}
	}
	if strings.HasPrefix(text, ":") {
		name := text[1:]
		if name == "" {
			return Form{}, SyntaxError{Pos: start, Msg: "keyword has no name"}
		}
		return Form{Kind: KindKeyword, Pos: start, Text: name}, nil
	}
	if looksNumeric(text) {
		if n, err := strconv.ParseFloat(text, 64); err == nil {
			return Form{Kind: KindNumber, Pos: start, Text: text, Num: n}, nil
		}
	}
	return Form{Kind: KindSymbol, Pos: start, Text: text}, nil
}

// isDelim reports whether ch terminates an atom.
func isDelim(ch rune) bool {
	return ch == '(' || ch == ')' || ch == '"' || ch == ';' || unicode.IsSpace(ch)
}

// looksNumeric is a cheap guard so ParseFloat is only consulted for
// atoms that could plausibly be numbers. Without it, symbols like "inf"
// and "nan" would parse as floats.
func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if c == '-' || c == '+' || c == '.' {
		return len(s) > 1 && s[1] >= '0' && s[1] <= '9'
	}
	return c >= '0' && c <= '9'
}
