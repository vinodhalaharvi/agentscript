// Package scriptmem — execute.go is the unified execution entry that
// hides the backend split behind one call. A front end hands over prose
// (plus discovery + an LLM) and gets back an Outcome that is either a
// finished in-memory result or a temporal plan to submit. The front end
// branches only on the Outcome's *execution shape* — never on the
// grammar, the verbs, or the backend keyword.
//
// This lives in scriptmem (not script) because executing the memory
// backend requires the in-process runtime and its dependency tree.
// pkg/script stays lean for callers that only translate/compile; a caller
// that wants real memory execution imports scriptmem and accepts the
// runtime deps — which is honest, since running memory IS the runtime.
package scriptmem

import (
	"context"
	"fmt"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"
)

// Backend is the execution backend an Outcome targets. It re-exports the
// pkg/script/ast notion so a caller can branch without importing ast.
type Backend int

const (
	// Memory means the program ran in-process; Outcome.Result holds the
	// output.
	Memory Backend = iota
	// Temporal means the program compiled to a durable plan; Outcome.Plan
	// holds it for the caller to submit and await.
	Temporal
)

func (b Backend) String() string {
	switch b {
	case Memory:
		return "memory"
	case Temporal:
		return "temporal"
	default:
		return "unknown"
	}
}

// Outcome is the tagged result of Execute. Exactly one arm is meaningful,
// indicated by Backend:
//
//   - Backend == Memory:   the program already ran; Result holds the
//     output. The caller posts it directly. (Plan is zero.)
//   - Backend == Temporal: the program compiled to Plan, a durable Sibyl
//     plan the caller submits and awaits. (Result is empty.)
//
// The caller branches on Backend to choose its delivery shape
// (synchronous reply vs submit-correlate-await). It never needs the
// grammar, the verbs, or how the backend was chosen.
type Outcome struct {
	Backend Backend
	// Result is the in-process output (Backend == Memory).
	Result string
	// Plan is the durable plan to submit (Backend == Temporal).
	Plan sibyl.Plan
}

// Execute is the unified entry: prose in, Outcome out. It translates the
// prose to DSL (the LLM authors it), resolves it against the discovered
// grammar (the compiler is the safety net), inspects the chosen backend,
// and then EITHER runs it in-process (memory) returning the result, OR
// compiles it to a durable plan (temporal) for the caller to submit.
//
// The backend is chosen by the DSL the LLM emits (the memory|temporal
// keyword), which is driven by the grammar prompt — not by the caller.
// Translation/compilation errors (unknown verb, not-on-backend, malformed)
// surface here before anything runs.
func Execute(ctx context.Context, complete script.CompleteFunc, g script.GrammarInfo, cfg MemoryConfig, prose string) (Outcome, error) {
	// prose → DSL (AgentScript owns the prompt; discovery supplies vocab).
	src, err := script.TranslateGrammar(ctx, complete, g, prose)
	if err != nil {
		return Outcome{}, err
	}

	// DSL → AST → resolved AST (vocabulary + per-backend availability).
	a, err := script.Parse(ctx, src)
	if err != nil {
		return Outcome{}, err
	}
	r, err := script.Resolve(ctx, g.Registry, a)
	if err != nil {
		return Outcome{}, err
	}
	if len(r.Blocks) == 0 {
		return Outcome{}, fmt.Errorf("scriptmem.Execute: empty program")
	}

	// Branch on the backend the DSL chose — the ONLY place the two paths
	// diverge.
	switch r.Blocks[0].Backend {
	case ast.BackendMemory:
		out, err := RunMemory(ctx, cfg, r)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{Backend: Memory, Result: out}, nil

	case ast.BackendTemporal:
		// Reuse the lean compile pipeline to produce the durable plan.
		plan, err := script.Compile(ctx, g.Registry, src)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{Backend: Temporal, Plan: plan}, nil

	default:
		return Outcome{}, fmt.Errorf("scriptmem.Execute: unknown backend %v", r.Blocks[0].Backend)
	}
}
