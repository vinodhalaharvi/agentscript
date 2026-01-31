package main

import (
	"context"
	"fmt"
	"strings"
)

// Translator converts natural language to CSP-M
type Translator struct {
	gemini *GeminiClient
}

// NewTranslator creates a new Translator
func NewTranslator(apiKey string) *Translator {
	return &Translator{
		gemini: NewGeminiClient(apiKey, ""),
	}
}

const systemPrompt = `You are a translator that converts natural language into CSP-M (Communicating Sequential Processes - Machine readable) syntax.

CSP-M is a formal process algebra. Here's the syntax:

PROCESSES:
- SKIP                     -- terminate successfully  
- STOP                     -- deadlock (never use this)
- event -> P               -- do event, then process P
- P ; Q                    -- sequential: P then Q
- P ||| Q                  -- parallel/interleave: P and Q run concurrently
- P [] Q                   -- external choice: either P or Q
- (P)                      -- grouping

EVENTS (these are the available actions):
- search!"query"           -- search for information
- summarize                -- summarize the input
- analyze!"focus"          -- analyze with optional focus
- ask!"question"           -- ask a question
- save!"filename"          -- save to file
- read!"filename"          -- read from file
- list!"path"              -- list directory
- merge                    -- combine parallel results
- email!"address"          -- send email

RULES:
1. Output ONLY valid CSP-M, no explanation
2. Every process must end with SKIP
3. Use ||| for parallel/concurrent tasks
4. Use ; for sequential steps
5. Always use merge after parallel branches before continuing
6. Events with arguments use ! (e.g., search!"query")
7. Events without arguments are bare (e.g., summarize, merge)

EXAMPLES:

Input: "search for golang tutorials and save them"
Output: search!"golang tutorials" -> save!"tutorials.md" -> SKIP

Input: "compare Google and Microsoft"
Output: (search!"Google company" -> analyze -> SKIP ||| search!"Microsoft company" -> analyze -> SKIP) ; merge -> ask!"compare these companies" -> SKIP

Input: "research AI trends and email the summary to boss@company.com"
Output: search!"AI trends 2024" -> summarize -> email!"boss@company.com" -> SKIP

Input: "compare Tesla, Ford, and GM then save the analysis"
Output: (search!"Tesla" -> analyze -> SKIP ||| search!"Ford" -> analyze -> SKIP ||| search!"GM" -> analyze -> SKIP) ; merge -> ask!"compare these automakers" -> save!"comparison.md" -> SKIP

Input: "read my notes and summarize them"
Output: read!"notes.txt" -> summarize -> SKIP
`

// Translate converts natural language to CSP-M
func (t *Translator) Translate(ctx context.Context, naturalLanguage string) (string, error) {
	prompt := fmt.Sprintf("%s\n\nConvert this to CSP-M:\n%s", systemPrompt, naturalLanguage)

	result, err := t.gemini.GenerateContent(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("translation failed: %w", err)
	}

	// Clean up the response
	cspm := strings.TrimSpace(result)

	// Remove markdown code blocks if present
	cspm = strings.TrimPrefix(cspm, "```cspm")
	cspm = strings.TrimPrefix(cspm, "```csp")
	cspm = strings.TrimPrefix(cspm, "```")
	cspm = strings.TrimSuffix(cspm, "```")
	cspm = strings.TrimSpace(cspm)

	return cspm, nil
}
