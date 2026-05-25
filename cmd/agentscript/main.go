package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript"
)

func main() {
	// Flags
	verbose := flag.Bool("v", false, "Verbose output")
	interactive := flag.Bool("i", false, "Interactive REPL mode")
	natural := flag.Bool("n", false, "Natural language mode (translates input to DSL)")
	script := flag.String("e", "", "Execute DSL script directly")
	file := flag.String("f", "", "Execute DSL script from file")
	llmBackend := flag.String("llm", "claude-code", "default LLM backend: claude-code | gemini | claude")
	flag.Parse()

	ctx := context.Background()

	// Get API keys and credentials from environment
	geminiKey := os.Getenv("GEMINI_API_KEY")
	googleCreds := os.Getenv("GOOGLE_CREDENTIALS_FILE")
	if googleCreds == "" {
		// Check default location
		if _, err := os.Stat("credentials.json"); err == nil {
			googleCreds = "credentials.json"
		}
	}

	// A key is only required when the selected backend needs one.
	// claude-code (the default) uses the local `claude` CLI — no key.
	if *llmBackend == "gemini" && geminiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: GEMINI_API_KEY required for -llm=gemini")
		os.Exit(1)
	}

	// Create runtime
	rt, err := agentscript.NewRuntime(ctx, agentscript.RuntimeConfig{
		GeminiAPIKey:       geminiKey,
		ClaudeAPIKey:       coalesce(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("CLAUDE_API_KEY")),
		SearchAPIKey:       coalesce(os.Getenv("SEARCH_API_KEY"), os.Getenv("SERPAPI_KEY")),
		LLMBackend:         *llmBackend,
		GoogleCredsFile:    googleCreds,
		GoogleTokenFile:    os.Getenv("GOOGLE_TOKEN_FILE"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubTokenFile:    os.Getenv("GITHUB_TOKEN_FILE"),
		Verbose:            *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating runtime: %v\n", err)
		os.Exit(1)
	}

	// Create translator for natural language mode
	var trans *agentscript.Translator
	if *natural || *interactive {
		trans, err = agentscript.NewTranslator(ctx, geminiKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating translator: %v\n", err)
			os.Exit(1)
		}
	}

	// Execute based on mode
	switch {
	case *script != "":
		executeScript(ctx, rt, *script)
	case *file != "":
		executeFile(ctx, rt, *file)
	case *interactive:
		runREPL(ctx, rt, trans, *natural)
	default:
		// Check for piped input or remaining args
		if flag.NArg() > 0 {
			input := strings.Join(flag.Args(), " ")
			if *natural {
				executeNatural(ctx, rt, trans, input)
			} else {
				executeScript(ctx, rt, input)
			}
		} else {
			printUsage()
		}
	}
}

func executeScript(ctx context.Context, rt *agentscript.Runtime, script string) {
	result, err := rt.RunDSL(ctx, script)
	if err != nil {
		// Check if it is a parse error vs execution error
		if strings.Contains(err.Error(), "DSL parse error") {
			fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Println(result)
}

func executeFile(ctx context.Context, rt *agentscript.Runtime, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	content := string(data)

	executeScript(ctx, rt, content)
}

func executeNatural(ctx context.Context, rt *agentscript.Runtime, trans *agentscript.Translator, input string) {
	dsl, err := trans.Translate(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Translation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📝 DSL: %s\n\n", dsl)
	executeScript(ctx, rt, dsl)
}

func runREPL(ctx context.Context, rt *agentscript.Runtime, trans *agentscript.Translator, naturalMode bool) {
	fmt.Println("🤖 AgentScript REPL")
	fmt.Println("Commands: :help, :mode, :quit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if naturalMode {
			fmt.Print("🗣️  > ")
		} else {
			fmt.Print("📜 > ")
		}

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle REPL commands
		switch input {
		case ":quit", ":q":
			fmt.Println("Goodbye!")
			return
		case ":help", ":h":
			printHelp()
			continue
		case ":mode", ":m":
			naturalMode = !naturalMode
			if naturalMode {
				fmt.Println("Switched to natural language mode 🗣️")
			} else {
				fmt.Println("Switched to DSL mode 📜")
			}
			continue
		}

		// Execute input
		if naturalMode {
			dsl, err := trans.Translate(ctx, input)
			if err != nil {
				fmt.Printf("❌ Translation error: %v\n", err)
				continue
			}
			fmt.Printf("📝 DSL: %s\n", dsl)
			input = dsl
		}

		program, err := agentscript.Parse(input)
		if err != nil {
			fmt.Printf("❌ Parse error: %v\n", err)
			continue
		}

		result, err := rt.Execute(ctx, program)
		if err != nil {
			fmt.Printf("❌ Execution error: %v\n", err)
			continue
		}

		fmt.Printf("\n%s\n\n", result)
	}
}

func printUsage() {
	fmt.Print(`AgentScript - A DSL for commanding AI agents

Usage:
  agentscript [flags] [script]
  agentscript -i              # Interactive REPL
  agentscript -n "natural language command"
  agentscript -e 'search "topic" >=> summarize'
  agentscript -f script.as

Flags:
  -i    Interactive REPL mode
  -n    Natural language mode (translates to DSL)
  -e    Execute DSL script directly
  -f    Execute DSL script from file
  -v    Verbose output

Environment:
  GEMINI_API_KEY   Required. Your Gemini API key
  SEARCH_API_KEY   Optional. API key for web search (SerpAPI, etc.)

DSL Commands:
  SEARCH "query"     Search the web
  SUMMARIZE          Summarize input content
  SAVE "file"        Save to file
  READ "file"        Read from file
  ASK "question"     Ask a question with context
  ANALYZE "focus"    Analyze content
  LIST "path"        List directory contents
  MERGE              Combine parallel results
  EMAIL "address"    Send email with content

Fan-out (parallel execution):
  ( search "topic A" >=> analyze
    <*> search "topic B" >=> analyze
  ) >=> merge >=> ask "compare these"

Sequential pipeline with >=>>:
  search "golang tutorials" >=> summarize >=> save "notes.md"

Examples:
  agentscript -e 'read "doc.txt" >=> summarize'
  agentscript -e '( search "Google" >=> analyze "strengths" <*> search "Microsoft" >=> analyze "strengths" ) >=> merge >=> ask "who is winning?"'
  agentscript -n "compare Apple and Samsung and email the results to me"
  agentscript -i
`)
}

func printHelp() {
	fmt.Print(`
REPL Commands:
  :help, :h   Show this help
  :mode, :m   Toggle natural language / DSL mode  
  :quit, :q   Exit REPL

DSL Syntax:
  SEARCH "query"     - Search the web
  SUMMARIZE          - Summarize piped content
  SAVE "filename"    - Save to file
  READ "filename"    - Read from file
  ASK "question"     - Ask with context
  ANALYZE "focus"    - Analyze content
  LIST "path"        - List directory
  MERGE              - Combine parallel results
  EMAIL "address"    - Send email

Fan-out (parallel):
  ( search "A" >=> analyze
    <*> search "B" >=> analyze
  ) >=> merge >=> ask "compare"

Sequential pipeline:
  search "topic" >=> summarize >=> save "out.md"
`)
}

// coalesce returns the first non-empty string.
func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
