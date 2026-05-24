// Package script — builtins.go provides the registry of builtins the
// translator knows how to lower. Each builtin binds a DSL name to a
// registered Sibyl activity name (BuiltinSpec.AgentID) and declares its
// argument shape for Resolve.
//
// The MVP ships one builtin, echo, mapping to Sibyl's Echo activity — a
// no-vendor activity, so the first end-to-end demo needs no LLM or OAuth.
// Adding a builtin is adding one Register call here plus registering the
// matching activity on the Sibyl worker.
package script

import (
	sibyl "github.com/vinodhalaharvi/sibyl/agent"

	"github.com/vinodhalaharvi/agentscript/pkg/script/registry"
)

// EchoSpec is the BuiltinSpec for the echo builtin. It takes one
// optional string argument and lowers to Sibyl's Echo activity.
//
// echo with an arg returns the arg; echo with upstream input passes it
// through (Unix-pipe). So `echo "hi"` produces "hi", and
// `echo "hi" >=> echo` produces "hi" again at the second stage.
func EchoSpec() registry.BuiltinSpec {
	return registry.BuiltinSpec{
		Name:    "echo",
		AgentID: sibyl.EchoActivityName,
		ArgSchema: registry.ArgSchema{
			Params: []registry.ParamSpec{
				{Name: "text", Type: registry.StringT, Optional: true},
			},
		},
		// echo is the one builtin available on BOTH backends: the
		// in-process interpreter and Sibyl's registered Echo activity.
		Backends: []registry.Backend{registry.BackendMemory, registry.BackendTemporal},
	}
}

// DefaultRegistry returns a registry pre-populated with the MVP builtins.
// Front ends (CLI, loom) start from this and may register more.
func DefaultRegistry() *registry.Registry {
	r := registry.New()
	r.MustRegister(EchoSpec())
	return r
}
