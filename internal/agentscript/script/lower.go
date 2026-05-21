// Package script — lower.go implements the Lower and Finalize phases of
// the translator: ResolvedAST → fragment → sibyl Plan.
//
// Lower folds the resolved AST into a *fragment*: a partial plan with a
// known entry node (where upstream output flows in) and exit node (whose
// output is the fragment's result). Fragments compose:
//
//   - A single Call lowers to a one-node fragment (entry == exit).
//   - A Pipeline lowers each stage and chains them: stage[i]'s exit
//     becomes stage[i+1]'s required upstream. The fragment's entry is
//     the first stage's entry, its exit is the last stage's exit.
//
// Keeping a fragment (rather than building the Plan directly) makes
// lowerPipeline a clean fold over sub-fragments with well-defined
// splice points, and isolates node-ID generation. Finalize then flattens
// the fragment's accumulated nodes into a sibyl.Plan.
//
// Why two phases (Lower then Finalize) instead of one: Lower is pure
// agentscript (no sibyl types in its signature beyond what a node needs);
// Finalize is the single seam that produces the sibyl.Plan value. This
// keeps the sibyl dependency at one well-marked boundary.
package script

import (
	"context"
	"fmt"
	"strconv"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript/script/ast"
	"github.com/vinodhalaharvi/agentscript/internal/agentscript/script/resolved"
)

// fragment is a partial plan under construction. Nodes accumulate as the
// AST is folded; Entry and Exit mark the splice points for composition.
type fragment struct {
	nodes []sibyl.PlanNode
	// entry is the node that receives the fragment's upstream input.
	entry sibyl.PlanNodeID
	// exit is the node whose output is the fragment's result.
	exit sibyl.PlanNodeID
}

// Lowered is the output of Lower: the fragment plus the block's backend
// and mode (carried through for Finalize / future backends).
type Lowered struct {
	Fragment fragment
	Backend  ast.Backend
	Mode     ast.Mode
}

// Lower converts a resolved AST into a Lowered fragment. The MVP handles
// a single block containing a Pipeline of Calls (temporal static), which
// is what the parser currently produces. Multiple blocks and Parallel
// are rejected with a clear error until their phases are added.
func Lower(_ context.Context, in resolved.AST) (Lowered, error) {
	if len(in.Blocks) != 1 {
		return Lowered{}, fmt.Errorf("lower: expected exactly 1 block, got %d (multi-block not yet supported)", len(in.Blocks))
	}
	b := in.Blocks[0]

	g := &idgen{}
	frag, err := lowerNode(g, b.Body)
	if err != nil {
		return Lowered{}, err
	}
	return Lowered{Fragment: frag, Backend: b.Backend, Mode: b.Mode}, nil
}

// idgen issues stable, sequential node IDs (n0, n1, ...). Sequential IDs
// keep plans deterministic and readable in the Temporal UI.
type idgen struct{ n int }

func (g *idgen) next() sibyl.PlanNodeID {
	id := sibyl.PlanNodeID(fmt.Sprintf("n%d", g.n))
	g.n++
	return id
}

func lowerNode(g *idgen, n resolved.Node) (fragment, error) {
	switch node := n.(type) {
	case resolved.Call:
		return lowerCall(g, node), nil
	case resolved.Pipeline:
		return lowerPipeline(g, node)
	case resolved.Parallel:
		return fragment{}, fmt.Errorf("lower: parallel (<*>) not yet supported")
	default:
		return fragment{}, fmt.Errorf("lower: unknown resolved node type %T", n)
	}
}

// lowerCall produces a one-node fragment. The node's activity is the
// builtin's resolved AgentID (the registered Sibyl activity name); its
// args are the call's literal arguments stringified.
func lowerCall(g *idgen, c resolved.Call) fragment {
	id := g.next()
	node := sibyl.PlanNode{
		ID:       id,
		Activity: c.Spec.AgentID,
		Args:     argStrings(c.Args),
	}
	return fragment{nodes: []sibyl.PlanNode{node}, entry: id, exit: id}
}

// lowerPipeline folds the stages, chaining each stage's exit into the
// next stage's entry as a required upstream. The result fragment's entry
// is the first stage's entry; its exit is the last stage's exit.
func lowerPipeline(g *idgen, p resolved.Pipeline) (fragment, error) {
	if len(p.Stages) == 0 {
		return fragment{}, fmt.Errorf("lower: empty pipeline")
	}

	var all []sibyl.PlanNode
	var prevExit sibyl.PlanNodeID
	var entry sibyl.PlanNodeID

	for i, stage := range p.Stages {
		sf, err := lowerNode(g, stage)
		if err != nil {
			return fragment{}, fmt.Errorf("stage %d: %w", i, err)
		}
		if i == 0 {
			entry = sf.entry
		} else {
			// Link: this stage's entry requires the previous stage's exit.
			linkRequire(sf.nodes, sf.entry, prevExit)
		}
		all = append(all, sf.nodes...)
		prevExit = sf.exit
	}

	return fragment{nodes: all, entry: entry, exit: prevExit}, nil
}

// linkRequire adds `req` to the Requires of the node with id `target`
// within nodes. The entry node of a downstream fragment thereby depends
// on the exit of the upstream fragment.
func linkRequire(nodes []sibyl.PlanNode, target, req sibyl.PlanNodeID) {
	for i := range nodes {
		if nodes[i].ID == target {
			nodes[i].Requires = append(nodes[i].Requires, req)
			return
		}
	}
}

// argStrings converts resolved AST args into the []string a PlanNode
// carries. Numbers are formatted compactly; strings pass through.
func argStrings(args []ast.Arg) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch v := a.(type) {
		case ast.StringArg:
			out = append(out, v.Value)
		case ast.NumArg:
			out = append(out, strconv.FormatFloat(v.Value, 'g', -1, 64))
		}
	}
	return out
}

// === Finalize ==============================================================

// Finalize flattens a Lowered fragment into a sibyl.Plan. This is the
// single seam that produces the sibyl value. The MVP supports only the
// temporal backend; memory/dynamic are rejected here until their
// backends exist.
func Finalize(_ context.Context, l Lowered) (sibyl.Plan, error) {
	if l.Backend != ast.BackendTemporal {
		return sibyl.Plan{}, fmt.Errorf("finalize: only the temporal backend is supported (got %v)", l.Backend)
	}
	if l.Mode != ast.ModeStatic {
		return sibyl.Plan{}, fmt.Errorf("finalize: only static mode is supported (got %v)", l.Mode)
	}
	return sibyl.Plan{Nodes: l.Fragment.nodes}, nil
}

// === Validate ==============================================================

// Validate runs sibyl.Plan.Validate over the finalized plan. It is a
// distinct phase so the pipeline reads Parse >>> Resolve >>> Lower >>>
// Finalize >>> Validate >>> Submit, and so validation failures are
// attributable to this step.
func Validate(_ context.Context, p sibyl.Plan) (sibyl.Plan, error) {
	if err := p.Validate(); err != nil {
		return sibyl.Plan{}, fmt.Errorf("validate: %w", err)
	}
	return p, nil
}
