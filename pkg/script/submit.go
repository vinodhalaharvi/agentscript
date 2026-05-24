// Package script — submit.go provides the Submit phase and the
// end-to-end Compile/Run entry points that compose every phase.
//
// The full translator pipeline:
//
//	Source >>> Parse >>> Resolve >>> Lower >>> Finalize >>> Validate >>> Submit
//
// Compile runs everything up to (not including) Submit, producing a
// validated sibyl.Plan the caller can submit, inspect, or serialize.
// Run does Compile then Submit, returning a workflow handle. Both are the
// library entry points a front end (CLI, or loom) calls.
package script

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"
	"github.com/vinodhalaharvi/weft/weft"

	"github.com/vinodhalaharvi/agentscript/pkg/script/ast"
	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
	"github.com/vinodhalaharvi/agentscript/pkg/script/resolved"
)

// TranslateGrammar is the discovery-driven translate entry: it takes a
// GrammarInfo (from Grammar()) instead of a registry, so a front end can
// pipe discovery straight into translation without ever touching or
// naming a registry. This is the "dumb pipe" path — loom calls Grammar(),
// passes the result here, and forwards the output.
func TranslateGrammar(ctx context.Context, complete CompleteFunc, g GrammarInfo, prose string) (Source, error) {
	return Translate(ctx, complete, g.Registry, prose)
}

// CompileGrammar compiles source against a GrammarInfo's registry. Pairs
// with TranslateGrammar so a discovery-driven front end never names a
// registry for either phase.
func CompileGrammar(ctx context.Context, g GrammarInfo, src Source) (sibyl.Plan, error) {
	return Compile(ctx, g.Registry, src)
}

// Compile runs Parse >=> Resolve >=> Lower >=> Finalize >=> Validate,
// producing a validated sibyl.Plan. It does not submit.
//
// The pipeline is built by composing weft.Arrow values with weft.Pipe —
// not by hand-threading errors. Each phase is an Arrow[A,B]; composition
// is type-checked at every seam (a phase whose output type doesn't match
// the next phase's input simply won't compile), and weft.Compose handles
// the error short-circuiting. This is the "everything composes" contract
// made literal: Compile IS the composition of the phase arrows.
//
// Compile is the heart of the "AgentScript as serialization layer" idea:
// source text in, validated executable Plan out, with unknown builtins,
// arity/type errors (Resolve) and malformed graphs (Validate) all
// rejected before anything runs.
func Compile(ctx context.Context, reg *registry.Registry, src Source) (sibyl.Plan, error) {
	return CompileArrow(reg)(ctx, src)
}

// CompileArrow returns the compile pipeline as a single composed
// weft.Arrow[Source, sibyl.Plan], curried over the registry. Callers that
// want to compose compilation into a larger arrow (or submit via
// SubmitWith) use this directly; Compile is the convenience that applies
// it.
func CompileArrow(reg *registry.Registry) weft.Arrow[Source, sibyl.Plan] {
	return weft.Pipe5(
		weft.Arrow[Source, ast.AST](Parse),
		weft.Arrow[ast.AST, resolved.AST](ResolveWith(reg)),
		weft.Arrow[resolved.AST, Lowered](Lower),
		weft.Arrow[Lowered, sibyl.Plan](Finalize),
		weft.Arrow[sibyl.Plan, sibyl.Plan](Validate),
	)
}

// Submit starts the compiled plan as a Sibyl PlanWorkflow via the given
// Temporal client and returns the run handle. It does not block on the
// result; callers await it (or correlate, like loom).
func Submit(ctx context.Context, c client.Client, plan sibyl.Plan, workflowID, taskQueue string) (client.WorkflowRun, error) {
	if c == nil {
		return nil, fmt.Errorf("submit: client is nil")
	}
	return sibyl.SubmitPlan(ctx, c, plan, workflowID, taskQueue)
}

// SubmitWith curries Submit over a client and queue, producing an arrow
// from a Plan to a handle — the shape the translator pipeline uses.
func SubmitWith(c client.Client, taskQueue string) func(context.Context, sibyl.Plan) (client.WorkflowRun, error) {
	return func(ctx context.Context, plan sibyl.Plan) (client.WorkflowRun, error) {
		return Submit(ctx, c, plan, "", taskQueue)
	}
}

// TranslateAndCompile is the full front-half for a prose-driven caller:
// prose → DSL (via the LLM) → validated Plan. It is Translate followed by
// Compile, the two phases a front end like loom runs before submitting.
// A translation failure (LLM error) and a compile failure (unknown
// builtin, bad arity, malformed graph) are both returned here, before
// anything executes.
func TranslateAndCompile(ctx context.Context, complete CompleteFunc, reg *registry.Registry, prose string) (sibyl.Plan, error) {
	// prose → Source via the LLM, curried as an arrow, then composed with
	// the compile pipeline. Translate >=> Compile, both Arrow values.
	translateArrow := func(ctx context.Context, p string) (Source, error) {
		return Translate(ctx, complete, reg, p)
	}
	pipeline := weft.Pipe2(
		weft.Arrow[string, Source](translateArrow),
		CompileArrow(reg),
	)
	return pipeline(ctx, prose)
}
