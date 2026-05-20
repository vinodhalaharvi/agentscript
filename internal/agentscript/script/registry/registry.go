// Package registry is the explicit bridge from DSL builtin names to
// Sibyl agent IDs. It holds BuiltinSpec values registered by plugins.
//
// There is no init()-based auto-registration. A program builds a
// Registry and registers each builtin explicitly:
//
//	reg := registry.New()
//	_ = reg.Register(echo.Spec())
//	_ = reg.Register(weather.Spec())
//
// The Resolve phase of the translator looks up each Call.Name here,
// attaches the matching BuiltinSpec, and validates argument shape.
package registry

import (
	"fmt"
	"sort"
)

// === ArgType ===============================================================

// ArgType is the declared type of a builtin parameter. The MVP only has
// StringT; NumT is defined so the schema can grow without a breaking
// change to ParamSpec.
type ArgType int

const (
	// StringT is a string-valued parameter.
	StringT ArgType = iota
	// NumT is a numeric parameter. Reserved for future use.
	NumT
)

// String returns a human-readable name for the type.
func (t ArgType) String() string {
	switch t {
	case StringT:
		return "string"
	case NumT:
		return "number"
	default:
		return "unknown"
	}
}

// === Argument schema =======================================================

// ParamSpec describes a single declared parameter of a builtin.
type ParamSpec struct {
	// Name is a human-readable parameter name, used in error messages.
	Name string
	// Type is the expected argument type.
	Type ArgType
	// Optional marks a parameter that may be omitted. Optional params
	// must come after all required params in ArgSchema.Params.
	Optional bool
}

// ArgSchema describes the expected arguments of a builtin. The MVP
// validates arity (count of required vs optional) and per-arg type.
//
// Variadic is for builtins that accept any number of trailing args of
// a single type (e.g. a hypothetical concat that takes N strings).
// When Variadic is true, Params describes the leading fixed params and
// VariadicType is the type of the trailing run.
type ArgSchema struct {
	Params       []ParamSpec
	Variadic     bool
	VariadicType ArgType
}

// MinRequired returns the number of leading required params.
func (s ArgSchema) MinRequired() int {
	n := 0
	for _, p := range s.Params {
		if p.Optional {
			break
		}
		n++
	}
	return n
}

// === Auth source (advisory in the MVP) =====================================

// AuthKind enumerates credential-source kinds. Only OAuth has an
// implementation in Sibyl today; the others are placeholders so the
// BuiltinSpec shape is forward-compatible (memo §10).
type AuthKind int

const (
	// AuthNone means the builtin needs no credentials.
	AuthNone AuthKind = iota
	// AuthOAuth means the builtin's Sibyl agent uses WithOAuth.
	AuthOAuth
	// AuthEnv means a credential comes from an environment variable.
	AuthEnv
	// AuthFile means a credential is read from a file.
	AuthFile
)

// AuthSource is the declared default credential source for a builtin.
// Advisory in the MVP — the runtime doesn't act on it yet; it documents
// intent and reserves the shape for the future WithCredential work.
type AuthSource struct {
	Kind     AuthKind
	Provider string // e.g. "weatherapi" for OAuth; env var name for AuthEnv
}

// === BuiltinSpec ===========================================================

// BuiltinSpec is what a plugin registers with the agentscript registry.
// It maps a DSL-visible name to a Sibyl agent ID and declares the
// argument shape used for validation during Resolve.
type BuiltinSpec struct {
	// Name is the DSL-visible builtin name (e.g. "echo", "weather").
	Name string
	// AgentID is the Sibyl agent ID this builtin lowers to
	// (e.g. "agentscript/echo").
	AgentID string
	// Vendors lists the external services the builtin touches. Advisory;
	// used for documentation and tooling.
	Vendors []string
	// ArgSchema is the expected argument shape, validated during Resolve.
	ArgSchema ArgSchema
	// AuthHint declares the builtin's default credential source. Advisory
	// in the MVP.
	AuthHint AuthSource
}

// === Registry ==============================================================

// Registry maps DSL builtin names to their BuiltinSpec. It is not safe
// for concurrent mutation; build it once at startup, then treat it as
// read-only during resolution.
type Registry struct {
	specs map[string]BuiltinSpec
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{specs: make(map[string]BuiltinSpec)}
}

// Register adds a BuiltinSpec. It returns an error if the spec is
// invalid (empty Name or AgentID) or if a builtin with the same Name is
// already registered. Duplicate registration is an error rather than a
// silent overwrite so conflicting plugins are caught at startup.
func (r *Registry) Register(spec BuiltinSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("registry.Register: BuiltinSpec.Name is empty")
	}
	if spec.AgentID == "" {
		return fmt.Errorf("registry.Register: BuiltinSpec.AgentID is empty for %q", spec.Name)
	}
	if _, exists := r.specs[spec.Name]; exists {
		return fmt.Errorf("registry.Register: builtin %q already registered", spec.Name)
	}
	r.specs[spec.Name] = spec
	return nil
}

// MustRegister is Register that panics on error. Convenient for static
// startup registration where a duplicate or invalid spec is a
// programming error, not a runtime condition.
func (r *Registry) MustRegister(spec BuiltinSpec) {
	if err := r.Register(spec); err != nil {
		panic(err)
	}
}

// Lookup returns the BuiltinSpec for a name and whether it was found.
func (r *Registry) Lookup(name string) (BuiltinSpec, bool) {
	spec, ok := r.specs[name]
	return spec, ok
}

// Names returns all registered builtin names, sorted. Useful for error
// messages ("did you mean...") and for tooling.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.specs))
	for name := range r.specs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Len returns the number of registered builtins.
func (r *Registry) Len() int { return len(r.specs) }
