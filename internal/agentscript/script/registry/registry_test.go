package registry_test

import (
	"testing"

	"github.com/vinodhalaharvi/agentscript/internal/agentscript/script/registry"
)

func echoSpec() registry.BuiltinSpec {
	return registry.BuiltinSpec{
		Name:    "echo",
		AgentID: "agentscript/echo",
		ArgSchema: registry.ArgSchema{
			Params: []registry.ParamSpec{{Name: "message", Type: registry.StringT}},
		},
	}
}

func TestRegister_Basic(t *testing.T) {
	r := registry.New()
	if err := r.Register(echoSpec()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
	spec, ok := r.Lookup("echo")
	if !ok {
		t.Fatal("Lookup(echo) not found")
	}
	if spec.AgentID != "agentscript/echo" {
		t.Errorf("AgentID = %q, want agentscript/echo", spec.AgentID)
	}
}

func TestRegister_RejectsEmptyName(t *testing.T) {
	r := registry.New()
	err := r.Register(registry.BuiltinSpec{AgentID: "x"})
	if err == nil {
		t.Fatal("expected error for empty Name")
	}
}

func TestRegister_RejectsEmptyAgentID(t *testing.T) {
	r := registry.New()
	err := r.Register(registry.BuiltinSpec{Name: "echo"})
	if err == nil {
		t.Fatal("expected error for empty AgentID")
	}
}

func TestRegister_RejectsDuplicate(t *testing.T) {
	r := registry.New()
	if err := r.Register(echoSpec()); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register(echoSpec())
	if err == nil {
		t.Fatal("expected error registering duplicate name")
	}
}

func TestLookup_Missing(t *testing.T) {
	r := registry.New()
	_, ok := r.Lookup("nope")
	if ok {
		t.Error("Lookup of unregistered name should return false")
	}
}

func TestNames_Sorted(t *testing.T) {
	r := registry.New()
	r.MustRegister(registry.BuiltinSpec{Name: "zebra", AgentID: "a/zebra"})
	r.MustRegister(registry.BuiltinSpec{Name: "alpha", AgentID: "a/alpha"})
	r.MustRegister(registry.BuiltinSpec{Name: "mango", AgentID: "a/mango"})

	got := r.Names()
	want := []string{"alpha", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("Names len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMustRegister_PanicsOnDuplicate(t *testing.T) {
	r := registry.New()
	r.MustRegister(echoSpec())
	defer func() {
		if recover() == nil {
			t.Error("MustRegister should panic on duplicate")
		}
	}()
	r.MustRegister(echoSpec())
}

func TestArgSchema_MinRequired(t *testing.T) {
	cases := []struct {
		name   string
		schema registry.ArgSchema
		want   int
	}{
		{
			name:   "all required",
			schema: registry.ArgSchema{Params: []registry.ParamSpec{{Type: registry.StringT}, {Type: registry.StringT}}},
			want:   2,
		},
		{
			name: "one optional trailing",
			schema: registry.ArgSchema{Params: []registry.ParamSpec{
				{Type: registry.StringT},
				{Type: registry.StringT, Optional: true},
			}},
			want: 1,
		},
		{
			name:   "no params",
			schema: registry.ArgSchema{},
			want:   0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.schema.MinRequired(); got != tc.want {
				t.Errorf("MinRequired = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestArgType_String(t *testing.T) {
	if registry.StringT.String() != "string" {
		t.Errorf("StringT.String() = %q", registry.StringT.String())
	}
	if registry.NumT.String() != "number" {
		t.Errorf("NumT.String() = %q", registry.NumT.String())
	}
}
