package main

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// Program represents a complete AgentScript program
type Program struct {
	Statements []*Statement `@@*`
}

// Statement can be a simple command or a parallel block
type Statement struct {
	Parallel *Parallel  `( @@ |`
	Command  *Command   `  @@ )`
	Pipe     *Statement `( "->" @@ )?`
}

// Parallel represents a block of commands to run concurrently
type Parallel struct {
	Branches []*Statement `"parallel" "{" @@* "}"`
}

// Command represents a single command
type Command struct {
	Action string `@("search" | "summarize" | "save" | "read" | "stdin" | "ask" | "analyze" | "list" | "merge" | "email" | "calendar" | "meet" | "drive_save" | "doc_create" | "sheet_append" | "sheet_create" | "task" | "contact_find" | "youtube_search" | "youtube_upload" | "youtube_shorts" | "image_generate" | "image_analyze" | "video_analyze" | "video_generate" | "images_to_video" | "text_to_speech" | "audio_video_merge")`
	Arg    string `@String?`
}

// Lexer definition
var scriptLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Keyword", Pattern: `(parallel|search|summarize|save|read|stdin|ask|analyze|list|merge|email|calendar|meet|drive_save|doc_create|sheet_append|sheet_create|task|contact_find|youtube_search|youtube_upload|youtube_shorts|image_generate|image_analyze|video_analyze|video_generate|images_to_video|text_to_speech|audio_video_merge)`},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Pipe", Pattern: `->`},
	{Name: "LBrace", Pattern: `\{`},
	{Name: "RBrace", Pattern: `\}`},
	{Name: "Whitespace", Pattern: `[ \t\n\r]+`},
})

// Parser instance
var Parser = participle.MustBuild[Program](
	participle.Lexer(scriptLexer),
	participle.Elide("Whitespace"),
	participle.Unquote("String"),
)

// Parse parses an AgentScript program from a string
func Parse(input string) (*Program, error) {
	return Parser.ParseString("", input)
}
