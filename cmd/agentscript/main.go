// Command agentscript compiles and runs an AgentScript program.
//
// There is one binary and one surface syntax. Which backend a program
// runs on is a property of the source, not a flag:
//
//	(block :backend memory :mode static BODY)     runs in-process, now
//	(block :backend temporal :mode static BODY)   runs as a durable workflow
//
// A program with no explicit (block ...) wrapper defaults to memory, so
// the common case needs no ceremony:
//
//	agentscript -e '(pipe (search "go releases") summarize)'
//
// Both paths share the whole front end and diverge only at the end:
//
//	Source >>> Parse >>> Resolve >>> RunMemory                      (memory)
//	Source >>> Parse >>> Resolve >>> Lower >>> Finalize >>> Validate >>> Submit
//
// Submitting a temporal program needs a Temporal cluster and a Sibyl
// worker; --dry-run compiles and prints the Plan without either.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"go.temporal.io/sdk/client"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/scriptmem"
)

func main() {
	expr := flag.String("e", "", "execute the given program text")
	dryRun := flag.Bool("dry-run", false, "compile and print the Plan as JSON; do not run or submit")
	hostPort := flag.String("temporal", "", "Temporal host:port (default: SDK default 127.0.0.1:7233)")
	taskQueue := flag.String("queue", "", "Sibyl task queue (default: sibyl-agents)")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Usage = usage
	flag.Parse()

	src, err := readSource(*expr, flag.Args())
	if err != nil {
		fatal("%v", err)
	}

	ctx := context.Background()
	grammar := script.Grammar()

	// One front end, whatever the backend.
	parsed, err := script.Parse(ctx, src)
	if err != nil {
		fatal("%v", err)
	}
	resolvedAST, err := script.Resolve(ctx, grammar.Registry, parsed)
	if err != nil {
		fatal("resolve: %v", err)
	}
	if len(resolvedAST.Blocks) == 0 {
		fatal("program is empty")
	}

	// Branch on the backend the source chose — the only place the two
	// paths diverge.
	switch backend := resolvedAST.Blocks[0].Backend; backend {
	case ast.BackendMemory:
		if *dryRun {
			fatal("--dry-run applies to the temporal backend; a memory program has no Plan")
		}
		out, err := scriptmem.RunMemory(ctx, memoryConfig(*verbose), resolvedAST)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Println(out)

	case ast.BackendTemporal:
		plan, err := script.Compile(ctx, grammar.Registry, src)
		if err != nil {
			fatal("compile: %v", err)
		}
		if *dryRun {
			printPlan(plan)
			return
		}
		submit(ctx, plan, *hostPort, *taskQueue)

	default:
		fatal("unknown backend %v", backend)
	}
}

// readSource takes the program from -e, from a file argument, or from
// stdin when neither is given and stdin is not a terminal.
func readSource(expr string, args []string) (script.Source, error) {
	switch {
	case expr != "" && len(args) > 0:
		return "", fmt.Errorf("give either -e or a file, not both")
	case expr != "":
		return script.Source(expr), nil
	case len(args) == 1:
		b, err := os.ReadFile(args[0])
		if err != nil {
			return "", fmt.Errorf("read %s: %w", args[0], err)
		}
		return script.Source(b), nil
	case len(args) > 1:
		return "", fmt.Errorf("expected at most one file, got %d", len(args))
	}
	info, err := os.Stdin.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice == 0 {
		b, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return script.Source(b), nil
	}
	flag.Usage()
	os.Exit(2)
	return "", nil
}

// memoryConfig reads the interpreter's credentials from the environment.
// Every field is optional; a verb that needs a credential the config
// does not supply fails when it runs, as it always has.
func memoryConfig(verbose bool) scriptmem.MemoryConfig {
	return scriptmem.MemoryConfig{
		GeminiAPIKey:       os.Getenv("GEMINI_API_KEY"),
		ClaudeAPIKey:       coalesce(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("CLAUDE_API_KEY")),
		SearchAPIKey:       coalesce(os.Getenv("SEARCH_API_KEY"), os.Getenv("SERPAPI_KEY")),
		LLMBackend:         orDefault(os.Getenv("AGENTSCRIPT_LLM"), "claude-code"),
		GoogleCredsFile:    googleCreds(),
		GoogleTokenFile:    os.Getenv("GOOGLE_TOKEN_FILE"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubTokenFile:    os.Getenv("GITHUB_TOKEN_FILE"),
		Verbose:            verbose,
	}
}

func googleCreds() string {
	if v := os.Getenv("GOOGLE_CREDENTIALS_FILE"); v != "" {
		return v
	}
	if _, err := os.Stat("credentials.json"); err == nil {
		return "credentials.json"
	}
	return ""
}

func submit(ctx context.Context, plan sibyl.Plan, hostPort, taskQueue string) {
	opts := client.Options{}
	if hostPort != "" {
		opts.HostPort = hostPort
	}
	c, err := client.Dial(opts)
	if err != nil {
		fatal("dial Temporal: %v (is the cluster running? try --dry-run to just compile)", err)
	}
	defer c.Close()

	handle, err := script.Submit(ctx, c, plan, "", taskQueue)
	if err != nil {
		fatal("submit: %v", err)
	}
	fmt.Printf("submitted: workflow=%s run=%s\n", handle.GetID(), handle.GetRunID())
	fmt.Println("waiting for result...")

	var res sibyl.PlanResult
	if err := handle.Get(ctx, &res); err != nil {
		fatal("workflow failed: %v", err)
	}
	fmt.Println()
	fmt.Println("=== result ===")
	for _, leaf := range res.Leaves {
		fmt.Printf("%s: %s\n", leaf, res.Outputs[leaf])
	}
	fmt.Printf("(%d nodes, %dms)\n", len(res.Outputs), res.DurationMs)
}

func printPlan(plan sibyl.Plan) {
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fatal("marshal plan: %v", err)
	}
	fmt.Println(string(b))
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  agentscript [flags] <file>
  agentscript [flags] -e '<program>'
  cat prog.as | agentscript [flags]

flags:
  -e <program>     execute the given program text
  -v               verbose output
  --dry-run        compile and print the Plan as JSON (temporal backend only)
  --temporal host:port
  --queue <name>

examples:
  agentscript -e '(pipe (search "go releases") summarize)'
  agentscript examples/tech-digest.as
  agentscript --dry-run examples/durable-echo.as
`)
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "agentscript: "+format+"\n", args...)
	os.Exit(1)
}
