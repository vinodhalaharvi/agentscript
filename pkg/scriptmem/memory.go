// Package script — memory.go is the bridge that attaches the existing
// in-memory runtime to the unified pipeline. A `memory` block resolves
// through the same Parse → Resolve front as everything else, then — instead
// of lowering to a Sibyl Plan (the temporal path) — is handed to the
// in-process interpreter that already implements every historical verb.
//
// Nothing about the runtime changes: it still consumes the old *Program
// AST and runs exactly as it always has. This file only ADAPTS the new
// resolved.AST into that *Program and calls the existing Execute. The two
// backends now share one front end (translate/parse/resolve) and diverge
// only at the final interpreter — temporal lowers+submits, memory adapts
// +executes — which is the whole point of the unification.
package scriptmem

import (
	"context"
	"fmt"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/script/resolved"
)

// MemoryConfig configures the in-memory runtime for a RunMemory call. It
// mirrors the runtime's own config; all fields are optional (a verb that
// needs a credential the config doesn't supply fails at execution, as it
// always has).
type MemoryConfig struct {
	GeminiAPIKey       string
	LLMBackend         string // default LLM: claude-code (default) | gemini | claude
	ClaudeAPIKey       string
	SearchAPIKey       string
	Model              string
	Verbose            bool
	GoogleCredsFile    string
	GoogleTokenFile    string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubTokenFile    string
}

// RunMemory executes a resolved memory-backend program in-process and
// returns its result. It is the synchronous sibling of the temporal
// submit path: where temporal starts a durable workflow and returns a
// handle, RunMemory runs the interpreter here and now and returns the
// output directly.
//
// It expects the block to target the memory backend (callers route on the
// backend keyword); a non-memory block is rejected so a temporal program
// is never silently run in-process.
func RunMemory(ctx context.Context, cfg MemoryConfig, r resolved.AST) (string, error) {
	prog, backend, err := toProgram(r)
	if err != nil {
		return "", err
	}
	if backend != ast.BackendMemory {
		return "", fmt.Errorf("RunMemory: program targets the %s backend, not memory", backend.String())
	}

	rt, err := agentscript.NewRuntime(ctx, agentscript.RuntimeConfig{
		GeminiAPIKey:       cfg.GeminiAPIKey,
		LLMBackend:         cfg.LLMBackend,
		ClaudeAPIKey:       cfg.ClaudeAPIKey,
		SearchAPIKey:       cfg.SearchAPIKey,
		Model:              cfg.Model,
		Verbose:            cfg.Verbose,
		GoogleCredsFile:    cfg.GoogleCredsFile,
		GoogleTokenFile:    cfg.GoogleTokenFile,
		GitHubClientID:     cfg.GitHubClientID,
		GitHubClientSecret: cfg.GitHubClientSecret,
		GitHubTokenFile:    cfg.GitHubTokenFile,
	})
	if err != nil {
		return "", fmt.Errorf("RunMemory: runtime init: %w", err)
	}
	return rt.Execute(ctx, prog)
}

// toProgram adapts a resolved.AST into the in-memory runtime's *Program.
// The runtime is unchanged; this shim translates the unified pipeline's
// representation into the one Execute already understands. It returns the
// program and its backend so the caller can verify routing.
//
// The MVP pipeline produces exactly one block; multi-block programs are
// rejected (the temporal path has the same single-block assumption).
func toProgram(r resolved.AST) (*agentscript.Program, ast.Backend, error) {
	if len(r.Blocks) != 1 {
		return nil, 0, fmt.Errorf("toProgram: expected exactly one block, got %d", len(r.Blocks))
	}
	b := r.Blocks[0]
	stmt, err := nodeToStatement(b.Body)
	if err != nil {
		return nil, 0, err
	}
	return &agentscript.Program{Statements: []*agentscript.Statement{stmt}}, b.Backend, nil
}

// nodeToStatement converts a resolved.Node into the old *Statement tree.
//   - Pipeline → a chain of Statements linked by .Pipe (a >=> b >=> c)
//   - Parallel → a Statement whose .Parallel holds the branches
//   - Call     → a Statement whose .Command holds the verb + args
func nodeToStatement(n resolved.Node) (*agentscript.Statement, error) {
	switch node := n.(type) {
	case resolved.Pipeline:
		return pipelineToStatement(node)
	case resolved.Parallel:
		return parallelToStatement(node)
	case resolved.Call:
		cmd, err := callToCommand(node)
		if err != nil {
			return nil, err
		}
		return &agentscript.Statement{Command: cmd}, nil
	default:
		return nil, fmt.Errorf("nodeToStatement: unsupported node %T", n)
	}
}

// pipelineToStatement folds [s0, s1, s2] into s0 >=> s1 >=> s2 by linking
// each stage's .Pipe to the next. A single-stage pipeline is just that
// stage.
func pipelineToStatement(p resolved.Pipeline) (*agentscript.Statement, error) {
	if len(p.Stages) == 0 {
		return nil, fmt.Errorf("pipelineToStatement: empty pipeline")
	}
	stmts := make([]*agentscript.Statement, len(p.Stages))
	for i, s := range p.Stages {
		st, err := nodeToStatement(s)
		if err != nil {
			return nil, err
		}
		stmts[i] = st
	}
	// Link i → i+1 via Pipe. A stage that is itself a pipeline/parallel
	// statement keeps its own structure; we attach the continuation to
	// the tail of the chain.
	for i := 0; i < len(stmts)-1; i++ {
		tail := stmts[i]
		for tail.Pipe != nil {
			tail = tail.Pipe
		}
		tail.Pipe = stmts[i+1]
	}
	return stmts[0], nil
}

// parallelToStatement maps resolved.Parallel branches into the old
// *Parallel form ( b0 <*> b1 <*> ... ).
func parallelToStatement(p resolved.Parallel) (*agentscript.Statement, error) {
	branches := make([]*agentscript.Statement, len(p.Branches))
	for i, br := range p.Branches {
		st, err := nodeToStatement(br)
		if err != nil {
			return nil, err
		}
		branches[i] = st
	}
	return &agentscript.Statement{Parallel: &agentscript.Parallel{Branches: branches}}, nil
}

// callToCommand maps a resolved.Call into the old *Command. The old form
// holds up to four positional string args; the pipeline's permissive
// memory specs are variadic-string, so we map the first four and reject
// anything beyond what the old command shape can carry.
func callToCommand(c resolved.Call) (*agentscript.Command, error) {
	args := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		s, ok := a.(ast.StringArg)
		if !ok {
			return nil, fmt.Errorf("callToCommand: %s: non-string argument %T", c.Name, a)
		}
		args = append(args, s.Value)
	}
	if len(args) > 4 {
		return nil, fmt.Errorf("callToCommand: %s: %d args exceeds the 4 the in-memory command form supports", c.Name, len(args))
	}
	cmd := &agentscript.Command{Action: c.Name}
	if len(args) > 0 {
		cmd.Arg = args[0]
	}
	if len(args) > 1 {
		cmd.Arg2 = args[1]
	}
	if len(args) > 2 {
		cmd.Arg3 = args[2]
	}
	if len(args) > 3 {
		cmd.Arg4 = args[3]
	}
	return cmd, nil
}
