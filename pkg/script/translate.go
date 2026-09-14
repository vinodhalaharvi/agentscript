// Package script — translate.go is the prose→DSL phase: it turns a
// natural-language request into an AgentScript program using an injected
// LLM, so a front end (loom, a CLI, anything) can hand over prose and
// get back source the rest of the pipeline compiles.
//
// This lives in pkg/script, next to the grammar (Parse) and the registry
// (Resolve), on purpose: the translation prompt must stay in lockstep
// with the grammar the parser accepts and the builtins the registry
// defines. A front end should not carry its own copy of the grammar
// rules — it would drift the moment the grammar or builtins change. The
// prompt here reads reg.Names() so the available commands are always the
// real, current set.
//
// The LLM is a seam (CompleteFunc), not a dependency: this package does
// not pick a provider or read an API key. The caller passes whatever
// LLM it has (Anthropic, Gemini, a fake in tests).
package script

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
)

// CompleteFunc is the LLM seam: given a system prompt and a user
// message, return the completion. It matches Sibyl's agent.CompleteFunc,
// so a Sibyl LLM client plugs in directly, but this package depends on
// nothing in Sibyl for translation.
type CompleteFunc func(ctx context.Context, systemPrompt, userMessage string) (string, error)

// Translate converts natural-language prose into an AgentScript program
// (Source) using the given LLM and registry. It does not compile or
// validate — pass the result to Compile for that. The returned Source is
// the LLM's output with code fences and stray prose stripped.
//
// The composition discipline is encoded in the prompt: emit sequential
// pipelines (pipe) by default, and parallel fan-out (par) only for an
// unambiguous flat list of independent actions. Inferring parallelism a
// user did not clearly express is the risky case, so the prompt biases
// toward sequential. The grammar is permissive; the translation is
// conservative.
func Translate(ctx context.Context, complete CompleteFunc, reg *registry.Registry, prose string) (Source, error) {
	if complete == nil {
		return "", fmt.Errorf("translate: no LLM (CompleteFunc) provided")
	}
	out, err := complete(ctx, BuildPrompt(reg), prose)
	if err != nil {
		return "", fmt.Errorf("translate: LLM call failed: %w", err)
	}
	return Source(cleanDSL(out)), nil
}

// BuildPrompt assembles the system prompt the LLM follows. It is exported
// so callers can inspect or log the exact instruction, and so tests can
// assert its contents. It lists the registry's builtins (the LLM may use
// only commands that exist) and states the composition rules.
func BuildPrompt(reg *registry.Registry) string {
	var names []string
	if reg != nil {
		names = reg.Names()
	}
	available := "(none)"
	if len(names) > 0 {
		available = strings.Join(names, ", ")
	}

	return `You translate a user's request into a small pipeline language called AgentScript. Output ONLY the AgentScript program — no prose, no explanation, no code fences.

GRAMMAR
AgentScript is written as s-expressions. A program is a single block that names an execution backend:
  (block :backend memory :mode static BODY)      ← runs in-process, immediately
  (block :backend temporal :mode static BODY)    ← runs as a durable workflow

BODY is one of:
  (pipe STAGE STAGE ...)   sequential — each stage's output feeds the next
  (par BRANCH BRANCH ...)  parallel fan-out — branches run on the same input
  a single stage

A stage is a command call. With arguments it is a list; with none it is a bare name:
  (search "golang jobs")
  summarize

Use ` + "`par`" + ` ONLY when the request is an explicit, unambiguous flat list of independent things to do at once, and follow it with ` + "`merge`" + `. When in doubt, use ` + "`pipe`" + `.

CHOOSING THE BACKEND
- Use ` + "`memory`" + ` by DEFAULT — for research, summarizing, reading files, asking questions, analysis, and almost everything. It runs the full command set immediately.
- Use ` + "`temporal`" + ` ONLY when the user explicitly asks for a durable, long-running, scheduled, or background workflow. Currently only the ` + "`echo`" + ` command runs on temporal.
- If the user says "in memory", "locally", "quickly", or doesn't mention durability, use ` + "`memory`" + `.

PASSING CONTENT
When the user pastes text to act on (e.g. "summarize <pasted text>"), put that pasted text directly into the command's quoted argument:
  Request: summarize The quick brown fox jumped over the lazy dog.
  Output: (block :backend memory :mode static (summarize "The quick brown fox jumped over the lazy dog."))
Do NOT use a file path or ` + "`read`" + ` for pasted content — content travels in the argument so the same program is portable across backends. Verbs take their content from the pipeline input or, when there is none, from their argument.

AVAILABLE COMMANDS (you may use ONLY these — never invent a command):
  ` + available + `

RULES
1. Output exactly one block: ` + "`(block :backend memory :mode static ...)`" + ` or ` + "`(block :backend temporal :mode static ...)`" + `. Nothing else.
2. Default to the memory backend unless durability is explicitly requested.
3. Use only commands from the AVAILABLE COMMANDS list. If the request needs a command that does not exist, choose the closest available command; do not invent names.
4. Prefer ` + "`pipe`" + `. Use ` + "`par`" + ` only for a clear list of independent parallel actions.
5. String arguments are double-quoted. Pass the user's intent as the argument text.
6. Parentheses must balance. A command with no arguments is written as a bare name, not as an empty list.
7. Keep it minimal — the smallest pipeline that satisfies the request.

EXAMPLES
Request: summarize this article about climate policy
Output: (block :backend memory :mode static (summarize "this article about climate policy"))

Request: research Google and Microsoft strengths, then tell me who is winning
Output: (block :backend memory :mode static (pipe (par (pipe (search "Google strengths") (analyze "strengths")) (pipe (search "Microsoft strengths") (analyze "strengths"))) merge (ask "who is winning?")))

Request: say hello to the team
Output: (block :backend memory :mode static (echo "hello to the team"))
`
}
