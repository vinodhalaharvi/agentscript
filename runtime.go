package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Runtime executes CSP-M processes
type Runtime struct {
	gemini    *GeminiClient
	procs     map[string]*ProcessDef
	verbose   bool
	results   map[string]string // channel name -> last value (for data flow)
	resultsMu sync.Mutex
}

// RuntimeConfig holds runtime configuration
type RuntimeConfig struct {
	GeminiAPIKey string
	Model        string
	Verbose      bool
}

// NewRuntime creates a new Runtime instance
func NewRuntime(cfg RuntimeConfig) *Runtime {
	var client *GeminiClient
	if cfg.GeminiAPIKey != "" {
		client = NewGeminiClient(cfg.GeminiAPIKey, cfg.Model)
	}

	return &Runtime{
		gemini:  client,
		procs:   make(map[string]*ProcessDef),
		verbose: cfg.Verbose,
		results: make(map[string]string),
	}
}

// Execute runs a complete CSP-M specification
func (r *Runtime) Execute(ctx context.Context, spec *Spec) (string, error) {
	// Collect process definitions
	for _, decl := range spec.Declarations {
		if decl.Process != nil {
			r.procs[decl.Process.Name] = decl.Process
		}
	}

	// Find and run MAIN process, or the last defined process
	var mainProc *ProcessDef
	if p, ok := r.procs["MAIN"]; ok {
		mainProc = p
	} else {
		// Use last defined process
		for _, decl := range spec.Declarations {
			if decl.Process != nil {
				mainProc = decl.Process
			}
		}
	}

	if mainProc == nil {
		return "", fmt.Errorf("no process defined")
	}

	r.log("Starting process: %s", mainProc.Name)
	return r.executeExpr(ctx, mainProc.Expr, "")
}

// ExecuteExpr runs a single CSP-M expression
func (r *Runtime) ExecuteExpr(ctx context.Context, expr *ProcessExpr) (string, error) {
	return r.executeExpr(ctx, expr, "")
}

func (r *Runtime) executeExpr(ctx context.Context, expr *ProcessExpr, input string) (string, error) {
	// Sequence: P ; Q ; R -> execute in order, passing results
	result := input
	for _, term := range expr.Terms {
		var err error
		result, err = r.executeParallel(ctx, term, result)
		if err != nil {
			return "", err
		}
	}
	return result, nil
}

func (r *Runtime) executeParallel(ctx context.Context, par *ParallelExpr, input string) (string, error) {
	if len(par.Terms) == 1 {
		return r.executeChoice(ctx, par.Terms[0], input)
	}

	// Interleave: P ||| Q ||| R -> run concurrently with goroutines
	r.log("PARALLEL: starting %d branches", len(par.Terms))

	type branchResult struct {
		index  int
		result string
		err    error
	}

	results := make(chan branchResult, len(par.Terms))
	var wg sync.WaitGroup

	for i, term := range par.Terms {
		wg.Add(1)
		go func(idx int, t *ChoiceExpr) {
			defer wg.Done()
			res, err := r.executeChoice(ctx, t, input)
			results <- branchResult{index: idx, result: res, err: err}
		}(i, term)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	orderedResults := make([]string, len(par.Terms))
	for res := range results {
		if res.err != nil {
			return "", fmt.Errorf("branch %d failed: %w", res.index, res.err)
		}
		orderedResults[res.index] = res.result
	}

	// Combine results
	var combined []string
	for i, res := range orderedResults {
		if res != "" {
			combined = append(combined, fmt.Sprintf("=== Branch %d ===\n%s", i+1, res))
		}
	}

	r.log("PARALLEL: completed %d branches", len(par.Terms))
	return strings.Join(combined, "\n\n"), nil
}

func (r *Runtime) executeChoice(ctx context.Context, choice *ChoiceExpr, input string) (string, error) {
	if len(choice.Terms) == 1 {
		return r.executePrefix(ctx, choice.Terms[0], input)
	}

	// External choice: for now, just execute first option
	// (Full implementation would use select on channels)
	r.log("CHOICE: selecting first of %d options", len(choice.Terms))
	return r.executePrefix(ctx, choice.Terms[0], input)
}

func (r *Runtime) executePrefix(ctx context.Context, prefix *PrefixExpr, input string) (string, error) {
	result := input

	// Execute each event in the prefix chain
	for _, event := range prefix.Prefix {
		var err error
		result, err = r.executeEvent(ctx, event, result)
		if err != nil {
			return "", err
		}
	}

	// Execute base
	return r.executeBase(ctx, prefix.Base, result)
}

func (r *Runtime) executeEvent(ctx context.Context, event *Event, input string) (string, error) {
	channel := strings.ToLower(event.Channel)
	arg := ""
	if event.Send != nil {
		arg = *event.Send
	}

	r.log("EVENT: %s%s", event.Channel, formatArg(event))

	switch channel {
	case "search":
		return r.search(ctx, arg)
	case "summarize":
		return r.summarize(ctx, input)
	case "analyze":
		return r.analyze(ctx, arg, input)
	case "ask":
		return r.ask(ctx, arg, input)
	case "save":
		return r.save(arg, input)
	case "read":
		return r.read(arg)
	case "list":
		return r.list(arg)
	case "merge":
		// merge is a no-op, just passes through combined parallel results
		return input, nil
	case "email":
		return r.email(ctx, arg, input)
	default:
		// Unknown channel - just log and pass through
		r.log("Unknown channel: %s (passing through)", channel)
		return input, nil
	}
}

func (r *Runtime) executeBase(ctx context.Context, base *BaseExpr, input string) (string, error) {
	switch {
	case base.Stop:
		r.log("STOP - deadlock")
		select {} // Block forever
	case base.Skip:
		r.log("SKIP - terminate")
		return input, nil
	case base.Name != nil:
		// Process call
		proc, ok := r.procs[*base.Name]
		if !ok {
			return "", fmt.Errorf("undefined process: %s", *base.Name)
		}
		r.log("Calling process: %s", *base.Name)
		return r.executeExpr(ctx, proc.Expr, input)
	case base.Parens != nil:
		return r.executeExpr(ctx, base.Parens, input)
	}
	return input, nil
}

// ============================================================================
// Built-in actions (same as before)
// ============================================================================

func (r *Runtime) search(ctx context.Context, query string) (string, error) {
	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for search")
	}
	r.log("SEARCH: %q", query)
	return r.gemini.GenerateContent(ctx, "Please provide information about: "+query)
}

func (r *Runtime) summarize(ctx context.Context, input string) (string, error) {
	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for summarize")
	}
	r.log("SUMMARIZE: %d bytes", len(input))
	return r.gemini.GenerateContent(ctx, "Summarize the following concisely:\n\n"+input)
}

func (r *Runtime) analyze(ctx context.Context, focus, input string) (string, error) {
	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for analyze")
	}
	prompt := "Analyze the following"
	if focus != "" {
		prompt += " focusing on " + focus
	}
	prompt += ":\n\n" + input
	r.log("ANALYZE: %q (%d bytes)", focus, len(input))
	return r.gemini.GenerateContent(ctx, prompt)
}

func (r *Runtime) ask(ctx context.Context, question, input string) (string, error) {
	if r.gemini == nil {
		return "", fmt.Errorf("GEMINI_API_KEY required for ask")
	}
	prompt := question
	if input != "" {
		prompt = question + "\n\nContext:\n" + input
	}
	r.log("ASK: %q", question)
	return r.gemini.GenerateContent(ctx, prompt)
}

func (r *Runtime) save(path, content string) (string, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	r.log("SAVE: %s (%d bytes)", path, len(content))
	return fmt.Sprintf("Saved %d bytes to %s", len(content), path), nil
}

func (r *Runtime) read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	r.log("READ: %s (%d bytes)", path, len(data))
	return string(data), nil
}

func (r *Runtime) list(path string) (string, error) {
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}
	var lines []string
	for _, entry := range entries {
		prefix := "F"
		if entry.IsDir() {
			prefix = "D"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", prefix, entry.Name()))
	}
	r.log("LIST: %s (%d entries)", path, len(entries))
	return strings.Join(lines, "\n"), nil
}

func (r *Runtime) email(ctx context.Context, to, content string) (string, error) {
	r.log("EMAIL: to %s", to)

	// Format email with Gemini
	var formattedEmail string
	if r.gemini != nil {
		prompt := fmt.Sprintf("Format this as a professional email to %s. Include subject line:\n\n%s", to, content)
		var err error
		formattedEmail, err = r.gemini.GenerateContent(ctx, prompt)
		if err != nil {
			formattedEmail = content
		}
	} else {
		formattedEmail = content
	}

	// Simulate sending
	fmt.Printf("\n📧 ========== EMAIL TO: %s ==========\n", to)
	fmt.Println(formattedEmail)
	fmt.Println("📧 ======================================\n")

	return fmt.Sprintf("Email sent to %s", to), nil
}

func (r *Runtime) log(format string, args ...any) {
	if r.verbose {
		fmt.Printf("[cspm] "+format+"\n", args...)
	}
}

func formatArg(event *Event) string {
	if event.Send != nil {
		return fmt.Sprintf("!%q", *event.Send)
	}
	if event.Recv != nil {
		return fmt.Sprintf("?%s", *event.Recv)
	}
	return ""
}
