// Command agentscript-run compiles an AgentScript source file into a
// Sibyl execution Plan and either prints it (--dry-run) or submits it to
// a running Sibyl worker via Temporal.
//
// This is the translator's CLI front end — distinct from the legacy
// in-process runtime in cmd/agentscript. It exercises the full pipeline:
//
//	Source >>> Parse >>> Resolve >>> Lower >>> Finalize >>> Validate [>>> Submit]
//
// Usage:
//
//	agentscript-run --dry-run script.as        # compile + print the Plan as JSON
//	agentscript-run script.as                  # compile + submit, wait for result
//
// Submitting requires a Temporal cluster and a Sibyl worker:
//
//	temporal server start-dev
//	go run ./cmd/worker        # in the sibyl repo
//	agentscript-run demo.as    # here
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"go.temporal.io/sdk/client"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript/script"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "compile and print the Plan as JSON; do not submit")
	hostPort := flag.String("temporal", "", "Temporal host:port (default: SDK default 127.0.0.1:7233)")
	taskQueue := flag.String("queue", "", "Sibyl task queue (default: sibyl-agents)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentscript-run [--dry-run] [--temporal host:port] [--queue q] <file.as>")
		os.Exit(2)
	}
	path := flag.Arg(0)

	srcBytes, err := os.ReadFile(path)
	if err != nil {
		fatal("read %s: %v", path, err)
	}

	ctx := context.Background()
	reg := script.DefaultRegistry()

	// Compile: source → validated Plan. No Temporal needed for this.
	plan, err := script.Compile(ctx, reg, script.Source(string(srcBytes)))
	if err != nil {
		fatal("compile: %v", err)
	}

	if *dryRun {
		printPlan(plan)
		return
	}

	// Submit: needs a Temporal client + running worker.
	opts := client.Options{}
	if *hostPort != "" {
		opts.HostPort = *hostPort
	}
	c, err := client.Dial(opts)
	if err != nil {
		fatal("dial Temporal: %v (is the cluster running? try --dry-run to just compile)", err)
	}
	defer c.Close()

	handle, err := script.Submit(ctx, c, plan, "", *taskQueue)
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

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "agentscript-run: "+format+"\n", args...)
	os.Exit(1)
}
