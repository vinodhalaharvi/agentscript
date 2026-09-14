// Package script — parse.go implements the Parse phase of the
// translator: text in, immutable AST out.
//
// There is one surface syntax, s-expressions, and Parse is a thin
// wrapper over it. The reader lives in the sexpr subpackage and the
// Form→ast mapping in sexpr_ast.go; this file exists to keep the phase
// name and the error type stable for callers.
//
// The operator dialect (`memory static ( search "x" >=> summarize )`)
// and its participle grammar have been removed. Nothing downstream
// noticed, which was the point of doing the front-end work behind the
// AST boundary.
package script

import (
	"context"
	"fmt"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
)

// === Parse arrow ===========================================================

// Parse converts source text into the canonical AST. It is the first
// phase of the translator pipeline: Source >>> Parse >>> Resolve >>> ...
//
// Errors are ParseError values carrying the original source; callers can
// errors.As to recover the underlying sexpr.SyntaxError, which has the
// line and column.
func Parse(ctx context.Context, src Source) (ast.AST, error) {
	return ParseSExpr(ctx, src)
}

// ParseError wraps a reader error with the original source for improved
// error reporting downstream. Implements error and errors.Unwrap.
type ParseError struct {
	Err    error
	Source string
}

// Error implements error.
func (e ParseError) Error() string {
	return fmt.Sprintf("script.Parse: %v", e.Err)
}

// Unwrap returns the underlying reader error.
func (e ParseError) Unwrap() error { return e.Err }
