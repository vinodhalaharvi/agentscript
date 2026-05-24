// Package script — grammar.go is the discovery surface: a single entry
// point a front end calls to learn the complete grammar without knowing
// anything about how it is built or what is in it.
//
// The point is decoupling. A front end (loom, a CLI, anything) should not
// name a particular registry, hardcode a verb list, or change when the
// grammar grows. It calls Grammar(), pipes the result into Translate, and
// forwards results — never inspecting the contents. When AgentScript adds
// verbs, restructures registries, or changes the grammar, the front end
// reflects it automatically, because it fetches the grammar rather than
// embedding knowledge of it.
//
// AgentScript is the single source of truth for the grammar; the front
// end is a dumb pipe that asks "what can be done?" and forwards the
// answer.
package script

import "github.com/vinodhalaharvi/agentscript/pkg/script/registry"

// GrammarInfo is the self-describing result of discovery. It carries the
// registry a caller passes straight to Translate/Compile (without
// inspecting it) plus advisory, human-readable metadata that tooling
// (docs, a CLI, a help command) can surface. A front end that only wants
// to translate prose uses Registry and ignores the rest.
type GrammarInfo struct {
	// Registry is the complete builtin catalog. Pass it to Translate and
	// Compile. Callers are not expected to inspect it.
	Registry *registry.Registry

	// Verbs is the advisory list of available verb names (sorted),
	// derived from the registry. For tooling/help surfaces; the
	// authoritative validation always happens in Resolve against
	// Registry.
	Verbs []string

	// Operators describes the composition operators the grammar supports,
	// for help/discovery surfaces.
	Operators []OperatorInfo

	// Backends lists the execution backends the grammar can target.
	Backends []string
}

// OperatorInfo describes a composition operator for discovery surfaces.
type OperatorInfo struct {
	Symbol string
	Name   string
	Desc   string
}

// Grammar returns the complete grammar a front end can use: the full
// builtin catalog plus self-describing metadata. This is THE discovery
// entry point — a front end calls it, passes GrammarInfo.Registry into
// Translate, and never needs to know which registry backs it or when it
// changes.
//
// It is backed by CompleteRegistry (every historical verb, availability
// decided per-backend at Resolve), so discovery always reflects the full
// current vocabulary.
func Grammar() GrammarInfo {
	reg := CompleteRegistry()
	return GrammarInfo{
		Registry: reg,
		Verbs:    reg.Names(),
		Operators: []OperatorInfo{
			{Symbol: ">=>", Name: "pipe", Desc: "sequential composition: left output feeds the right"},
			{Symbol: "<*>", Name: "fanout", Desc: "parallel fanout: branches run on the same input"},
		},
		Backends: []string{"memory", "temporal"},
	}
}
