// Package script — sexpr_print.go renders an AST back as s-expression
// source.
//
// This exists to make converting the existing corpus mechanical rather
// than manual: parse a legacy script with the operator dialect, print it
// here, and the result is a valid s-expression script with an identical
// AST. It is also what makes the round-trip test in sexpr_test.go
// possible, which is the real check that the new front end is faithful.
package script

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

const indentUnit = "  "

// PrintSExpr renders a complete AST as s-expression source. The output
// reparses via ParseSExpr to an AST equal to the input.
func PrintSExpr(a ast.AST) (string, error) {
	var sb strings.Builder
	for i, b := range a.Blocks {
		if i > 0 {
			sb.WriteString("\n")
		}
		s, err := renderBlock(b)
		if err != nil {
			return "", fmt.Errorf("block %d: %w", i, err)
		}
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func renderBlock(b ast.Block) (string, error) {
	backend := b.Backend
	if backend == ast.BackendUnknown {
		backend = ast.BackendMemory
	}
	mode := b.Mode
	if mode == ast.ModeUnknown {
		mode = ast.ModeStatic
	}
	if b.Body == nil {
		return "", fmt.Errorf("block has no body")
	}
	body, err := renderNode(b.Body, 1)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s :backend %s :mode %s\n%s)",
		formBlock, backend, mode, body), nil
}

func renderNode(n ast.Node, depth int) (string, error) {
	indent := strings.Repeat(indentUnit, depth)
	switch v := n.(type) {
	case ast.Pipeline:
		// A one-stage pipeline is pure wrapping; print the stage itself
		// so the output stays readable. It reparses to the same shape
		// because asPipeline re-adds the wrapper.
		if len(v.Stages) == 1 {
			return renderNode(v.Stages[0], depth)
		}
		return renderSeq(formPipe, v.Stages, depth)
	case ast.Parallel:
		return renderSeq(formPar, v.Branches, depth)
	case ast.Call:
		return indent + renderCall(v), nil
	default:
		return "", fmt.Errorf("cannot print node of type %T", n)
	}
}

func renderSeq(head string, children []ast.Node, depth int) (string, error) {
	indent := strings.Repeat(indentUnit, depth)
	parts := make([]string, 0, len(children))
	for i, c := range children {
		s, err := renderNode(c, depth+1)
		if err != nil {
			return "", fmt.Errorf("%s child %d: %w", head, i, err)
		}
		parts = append(parts, s)
	}
	return indent + "(" + head + "\n" + strings.Join(parts, "\n") + ")", nil
}

func renderCall(c ast.Call) string {
	name := surfaceName(c.Name)
	if len(c.Args) == 0 {
		// A no-argument call prints as a bare symbol, which is how a
		// pipeline stage like "summarize" reads best.
		return name
	}
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, name)
	for _, a := range c.Args {
		parts = append(parts, renderArg(a))
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func renderArg(a ast.Arg) string {
	switch v := a.(type) {
	case ast.StringArg:
		return quote(v.Value)
	case ast.NumArg:
		return strconv.FormatFloat(v.Value, 'g', -1, 64)
	default:
		// Arg is a sealed sum; this is unreachable short of a new
		// variant being added without updating the printer.
		return quote(fmt.Sprintf("%v", a))
	}
}

// surfaceName is the inverse of verbName: registry verb to surface
// spelling, so "if" prints as "when".
func surfaceName(verb string) string {
	if s, ok := verbSurface[verb]; ok {
		return s
	}
	return verb
}

// quote renders a string literal with the escapes the reader accepts.
func quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, ch := range s {
		switch ch {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case '\r':
			sb.WriteString(`\r`)
		default:
			sb.WriteRune(ch)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
