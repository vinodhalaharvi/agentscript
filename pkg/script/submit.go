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

	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
)

// Compile runs Parse >>> Resolve >>> Lower >>> Finalize >>> Validate,
// producing a validated sibyl.Plan. It does not submit. The registry
// supplies the builtin→activity bindings Resolve and Lower need.
//
// Compile is the heart of the "AgentScript as serialization layer" idea:
// it turns source text into a validated, serializable execution plan,
// rejecting unknown builtins, arity/type errors (Resolve) and malformed
// graphs (Validate) before anything runs. A front end that has an LLM
// emit DSL gets this whole safety net for free.
func Compile(ctx context.Context, reg *registry.Registry, src Source) (sibyl.Plan, error) {
	a, err := Parse(ctx, src)
	if err != nil {
		return sibyl.Plan{}, err
	}
	r, err := Resolve(ctx, reg, a)
	if err != nil {
		return sibyl.Plan{}, err
	}
	lowered, err := Lower(ctx, r)
	if err != nil {
		return sibyl.Plan{}, err
	}
	plan, err := Finalize(ctx, lowered)
	if err != nil {
		return sibyl.Plan{}, err
	}
	return Validate(ctx, plan)
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
	src, err := Translate(ctx, complete, reg, prose)
	if err != nil {
		return sibyl.Plan{}, err
	}
	return Compile(ctx, reg, src)
}
