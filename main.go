package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	// Flags
	verbose := flag.Bool("v", false, "Verbose output")
	interactive := flag.Bool("i", false, "Interactive REPL mode")
	natural := flag.Bool("n", false, "Natural language mode")
	expr := flag.String("e", "", "Execute CSP-M expression directly")
	file := flag.String("f", "", "Execute CSP-M file")
	parseOnly := flag.Bool("parse", false, "Parse only, don't execute")
	flag.Parse()

	ctx := context.Background()
	geminiKey := os.Getenv("GEMINI_API_KEY")

	// Create runtime
	rt := NewRuntime(RuntimeConfig{
		GeminiAPIKey: geminiKey,
		Verbose:      *verbose,
	})

	// Create translator for natural language mode
	var trans *Translator
	if geminiKey != "" {
		trans = NewTranslator(geminiKey)
	}

	switch {
	case *expr != "":
		executeExpr(ctx, rt, *expr, *parseOnly)
	case *file != "":
		executeFile(ctx, rt, *file, *parseOnly)
	case *interactive:
		runREPL(ctx, rt, trans, *natural)
	case *natural && flag.NArg() > 0:
		input := strings.Join(flag.Args(), " ")
		executeNatural(ctx, rt, trans, input, *parseOnly)
	case flag.NArg() > 0:
		input := strings.Join(flag.Args(), " ")
		executeExpr(ctx, rt, input, *parseOnly)
	default:
		printUsage()
	}
}

func executeExpr(ctx context.Context, rt *Runtime, input string, parseOnly bool) {
	expr, err := ParseExpr(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	if parseOnly {
		fmt.Println("Parse successful!")
		return
	}

	result, err := rt.ExecuteExpr(ctx, expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func executeFile(ctx context.Context, rt *Runtime, path string, parseOnly bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	spec, err := Parse(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	if parseOnly {
		fmt.Println("Parse successful!")
		fmt.Printf("Channels: ")
		for _, decl := range spec.Declarations {
			if decl.Channel != nil {
				fmt.Printf("%v ", decl.Channel.Names)
			}
		}
		fmt.Println()
		fmt.Printf("Processes: ")
		for _, decl := range spec.Declarations {
			if decl.Process != nil {
				fmt.Printf("%s ", decl.Process.Name)
			}
		}
		fmt.Println()
		return
	}

	result, err := rt.Execute(ctx, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func executeNatural(ctx context.Context, rt *Runtime, trans *Translator, input string, parseOnly bool) {
	if trans == nil {
		fmt.Fprintln(os.Stderr, "Error: GEMINI_API_KEY required for natural language mode")
		os.Exit(1)
	}

	cspm, err := trans.Translate(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Translation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📝 CSP-M: %s\n\n", cspm)

	if parseOnly {
		return
	}

	executeExpr(ctx, rt, cspm, false)
}

func runREPL(ctx context.Context, rt *Runtime, trans *Translator, naturalMode bool) {
	fmt.Println("🔬 AgentScript CSP-M REPL")
	fmt.Println("Commands: :help, :mode, :quit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if naturalMode {
			fmt.Print("🗣️  > ")
		} else {
			fmt.Print("csp> ")
		}

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// REPL commands
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
				fmt.Println("Switched to CSP-M mode 🔬")
			}
			continue
		}

		// Natural language translation
		if naturalMode {
			if trans == nil {
				fmt.Println("❌ GEMINI_API_KEY required for natural language mode")
				continue
			}
			cspm, err := trans.Translate(ctx, input)
			if err != nil {
				fmt.Printf("❌ Translation error: %v\n", err)
				continue
			}
			fmt.Printf("📝 CSP-M: %s\n", cspm)
			input = cspm
		}

		// Parse and execute
		expr, err := ParseExpr(input)
		if err != nil {
			fmt.Printf("❌ Parse error: %v\n", err)
			continue
		}

		result, err := rt.ExecuteExpr(ctx, expr)
		if err != nil {
			fmt.Printf("❌ Execution error: %v\n", err)
			continue
		}

		fmt.Printf("\n%s\n\n", result)
	}
}

func printUsage() {
	fmt.Println(`AgentScript - CSP-M for AI Agent Orchestration

Usage:
  agentscript [flags] [expression]
  agentscript -e 'search!"golang" -> summarize -> SKIP'
  agentscript -f workflow.csp
  agentscript -n "compare Google and Microsoft"
  agentscript -i

Flags:
  -e string    Execute CSP-M expression directly
  -f string    Execute CSP-M file
  -n           Natural language mode (translates to CSP-M)
  -i           Interactive REPL mode
  -v           Verbose output
  -parse       Parse only, don't execute

Environment:
  GEMINI_API_KEY   Required for AI operations (search, summarize, etc.)

CSP-M Syntax:
  SKIP                      Terminate successfully
  event -> P                Do event, then P
  P ; Q                     Sequential: P then Q  
  P ||| Q                   Parallel: P and Q concurrently
  P [] Q                    Choice: either P or Q
  (P)                       Grouping

Events:
  search!"query"            Search for information
  summarize                 Summarize input
  analyze!"focus"           Analyze with focus
  ask!"question"            Ask a question
  save!"file"               Save to file
  read!"file"               Read from file
  list!"path"               List directory
  merge                     Combine parallel results
  email!"address"           Send email

Examples:
  # Simple search and save
  agentscript -e 'search!"golang" -> save!"go.md" -> SKIP'

  # Parallel comparison
  agentscript -e '(search!"Google" -> analyze -> SKIP ||| search!"Microsoft" -> analyze -> SKIP) ; merge -> ask!"compare" -> SKIP'

  # Natural language
  agentscript -n "research AI trends and email summary to boss@company.com"

  # From file
  agentscript -f workflow.csp
`)
}

func printHelp() {
	fmt.Println(`
REPL Commands:
  :help, :h    Show this help
  :mode, :m    Toggle natural language / CSP-M mode
  :quit, :q    Exit REPL

CSP-M Quick Reference:
  search!"query" -> summarize -> save!"out.md" -> SKIP
  
  (P ||| Q) ; merge -> ask!"compare" -> SKIP
  
  read!"file.txt" -> analyze!"trends" -> SKIP
`)
}
