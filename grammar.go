package main

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// ============================================================================
// CSP-M AST Definitions
// ============================================================================

// Spec represents a complete CSP-M specification
type Spec struct {
	Declarations []*Declaration `@@*`
}

// Declaration is either a channel declaration or a process definition
type Declaration struct {
	Channel *ChannelDecl `( @@ |`
	Process *ProcessDef  `  @@ )`
}

// ChannelDecl represents: channel ch1, ch2, ch3
type ChannelDecl struct {
	Names []string `"channel" @Ident ( "," @Ident )*`
}

// ProcessDef represents: PROC = ProcessExpr
type ProcessDef struct {
	Name string       `@Ident "="`
	Expr *ProcessExpr `@@`
}

// ProcessExpr - top level, handles sequence (lowest precedence)
// P ; Q ; R
type ProcessExpr struct {
	Terms []*ParallelExpr `@@ ( ";" @@ )*`
}

// ParallelExpr handles: P ||| Q (interleave - parallel execution)
type ParallelExpr struct {
	Terms []*ChoiceExpr `@@ ( "|||" @@ )*`
}

// ChoiceExpr handles: P [] Q (external choice)
type ChoiceExpr struct {
	Terms []*PrefixExpr `@@ ( "[]" @@ )*`
}

// PrefixExpr handles: event -> event -> ... -> BaseExpr
type PrefixExpr struct {
	Prefix []*Event  `( @@ "->" )*`
	Base   *BaseExpr `@@`
}

// Event represents: ch or ch!value or ch?var
type Event struct {
	Channel string  `@Ident`
	Send    *string `( "!" @String )?`
	Recv    *string `( "?" @Ident )?`
}

// BaseExpr is the base of expressions
type BaseExpr struct {
	Stop   bool         `( @"STOP" |`
	Skip   bool         `  @"SKIP" |`
	Name   *string      `  @Ident |`
	Parens *ProcessExpr `  "(" @@ ")" )`
}

// ============================================================================
// Lexer Definition
// ============================================================================

var cspmLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `--[^\n]*`},
	{Name: "Keyword", Pattern: `\b(channel|STOP|SKIP)\b`},
	{Name: "Interleave", Pattern: `\|\|\|`},
	{Name: "ExtChoice", Pattern: `\[\]`},
	{Name: "Arrow", Pattern: `->`},
	{Name: "Semi", Pattern: `;`},
	{Name: "Send", Pattern: `!`},
	{Name: "Recv", Pattern: `\?`},
	{Name: "Comma", Pattern: `,`},
	{Name: "Equals", Pattern: `=`},
	{Name: "LParen", Pattern: `\(`},
	{Name: "RParen", Pattern: `\)`},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "Whitespace", Pattern: `[ \t\n\r]+`},
})

// ============================================================================
// Parser
// ============================================================================

var Parser = participle.MustBuild[Spec](
	participle.Lexer(cspmLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.Unquote("String"),
)

// Parse parses a CSP-M specification from a string
func Parse(input string) (*Spec, error) {
	return Parser.ParseString("", input)
}

// ParseExpr parses a single CSP-M expression (for -e mode)
func ParseExpr(input string) (*ProcessExpr, error) {
	exprParser := participle.MustBuild[ProcessExpr](
		participle.Lexer(cspmLexer),
		participle.Elide("Whitespace", "Comment"),
		participle.Unquote("String"),
	)
	return exprParser.ParseString("", input)
}
