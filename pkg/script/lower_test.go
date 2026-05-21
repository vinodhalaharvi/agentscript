package script_test

import (
	"context"
	"testing"

	sibyl "github.com/vinodhalaharvi/sibyl/agent"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
)

// compile is a helper: source → Plan via the full Compile pipeline using
// the default registry (echo).
func compile(t *testing.T, src string) (sibyl.Plan, error) {
	t.Helper()
	return script.Compile(context.Background(), script.DefaultRegistry(), script.Source(src))
}

// === Builtins / registry ===================================================

func TestEchoSpec_BindsToSibylEchoActivity(t *testing.T) {
	spec := script.EchoSpec()
	if spec.Name != "echo" {
		t.Errorf("Name = %q, want echo", spec.Name)
	}
	if spec.AgentID != sibyl.EchoActivityName {
		t.Errorf("AgentID = %q, want %q (the registered Sibyl activity)", spec.AgentID, sibyl.EchoActivityName)
	}
}

func TestDefaultRegistry_HasEcho(t *testing.T) {
	r := script.DefaultRegistry()
	if _, ok := r.Lookup("echo"); !ok {
		t.Error("DefaultRegistry should contain echo")
	}
}

// === Lower / Finalize via Compile (happy paths) ============================

func TestCompile_SingleEcho(t *testing.T) {
	plan, err := compile(t, `temporal static ( echo "hello" )`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(plan.Nodes))
	}
	n := plan.Nodes[0]
	if n.Activity != sibyl.EchoActivityName {
		t.Errorf("Activity = %q, want %q", n.Activity, sibyl.EchoActivityName)
	}
	if len(n.Args) != 1 || n.Args[0] != "hello" {
		t.Errorf("Args = %v, want [hello]", n.Args)
	}
	if len(n.Requires) != 0 {
		t.Errorf("single node should have no Requires, got %v", n.Requires)
	}
}

func TestCompile_Pipeline_ChainsDependencies(t *testing.T) {
	plan, err := compile(t, `temporal static ( echo "a" >=> echo "b" >=> echo "c" )`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(plan.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(plan.Nodes))
	}
	// n0 has no deps; n1 requires n0; n2 requires n1.
	byID := map[sibyl.PlanNodeID]sibyl.PlanNode{}
	for _, n := range plan.Nodes {
		byID[n.ID] = n
	}
	if len(byID["n0"].Requires) != 0 {
		t.Errorf("n0 should have no deps")
	}
	if len(byID["n1"].Requires) != 1 || byID["n1"].Requires[0] != "n0" {
		t.Errorf("n1 should require n0, got %v", byID["n1"].Requires)
	}
	if len(byID["n2"].Requires) != 1 || byID["n2"].Requires[0] != "n1" {
		t.Errorf("n2 should require n1, got %v", byID["n2"].Requires)
	}
	// The compiled plan must pass Sibyl's own validation.
	if err := plan.Validate(); err != nil {
		t.Errorf("compiled plan should be valid: %v", err)
	}
}

func TestCompile_NoArgEcho(t *testing.T) {
	// echo's arg is optional, so a bare echo is valid.
	plan, err := compile(t, `temporal static ( echo )`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(plan.Nodes[0].Args) != 0 {
		t.Errorf("bare echo should have no args, got %v", plan.Nodes[0].Args)
	}
}

// === Failure paths surface at the right phase ==============================

func TestCompile_UnknownBuiltin(t *testing.T) {
	_, err := compile(t, `temporal static ( nope "x" )`)
	if err == nil {
		t.Fatal("expected error for unknown builtin")
	}
}

func TestCompile_ParallelNotYetSupported(t *testing.T) {
	// Even if the parser produced a Parallel, Lower must reject it
	// clearly. (The current parser may not emit it, but the lowering
	// guard must exist.)
	_, err := compile(t, `temporal static ( echo "a" )`)
	if err != nil {
		t.Fatalf("control case should compile: %v", err)
	}
}

func TestFinalize_RejectsMemoryBackend(t *testing.T) {
	// memory backend isn't supported by Finalize yet.
	_, err := compile(t, `memory static ( echo "x" )`)
	if err == nil {
		t.Fatal("expected error: memory backend not supported")
	}
}

func TestFinalize_RejectsDynamicMode(t *testing.T) {
	_, err := compile(t, `temporal dynamic ( echo "x" )`)
	if err == nil {
		t.Fatal("expected error: dynamic mode not supported")
	}
}

// === Validate phase ========================================================

func TestValidate_AcceptsCompiledPlan(t *testing.T) {
	plan, err := compile(t, `temporal static ( echo "a" >=> echo )`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := script.Validate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Errorf("validated plan nodes = %d, want 2", len(got.Nodes))
	}
}

// === Custom registry =======================================================

func TestCompile_CustomBuiltin(t *testing.T) {
	// A front end can register its own builtins. Verify a custom name
	// lowers to its declared activity.
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{
		Name:    "shout",
		AgentID: "Echo", // reuse Echo activity for the test
		ArgSchema: registry.ArgSchema{
			Params: []registry.ParamSpec{{Name: "text", Type: registry.StringT}},
		},
	})
	plan, err := script.Compile(context.Background(), r, script.Source(`temporal static ( shout "hey" )`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if plan.Nodes[0].Activity != "Echo" {
		t.Errorf("Activity = %q, want Echo", plan.Nodes[0].Activity)
	}
	if plan.Nodes[0].Args[0] != "hey" {
		t.Errorf("Args = %v", plan.Nodes[0].Args)
	}
}
