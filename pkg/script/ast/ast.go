// Package ast defines the AgentScript abstract syntax tree.
//
// The AST is the output of the Parse arrow and the input to Resolve.
// It is immutable: every Resolve and Lower step produces new values,
// it never mutates input. Sum types are sealed via unexported tag
// methods so external packages cannot add new variants without
// extending the parser.
//
// Backend selection (memory vs temporal) and mode selection (static
// vs dynamic) live on the Block node — not on Pipeline or Call —
// because each block declares its own execution context. A script
// can contain multiple blocks with different annotations; the
// translator handles each independently.
//
// MVP scope: only Pipeline and Call nodes are produced today.
// Parallel is defined here as a sealed variant so future PRs can
// add it without renaming or restructuring the AST.
package ast

// === Top-level script ======================================================

// AST is a complete parsed AgentScript program: a list of blocks.
type AST struct {
	Blocks []Block
}

// Block is one top-level <backend> <mode> ( ... ) form. The Body is
// always non-nil after a successful parse.
type Block struct {
	Backend Backend
	Mode    Mode
	Body    Node
}

// Backend is the execution backend a block targets.
type Backend int

const (
	// BackendUnknown is the zero value; never produced by a valid parse.
	BackendUnknown Backend = iota
	// BackendMemory runs the block in-process via the existing tree-walking evaluator.
	BackendMemory
	// BackendTemporal compiles the block to a Sibyl DAG and submits it to a Temporal worker.
	BackendTemporal
)

// String returns the keyword form used in source (or "unknown").
func (b Backend) String() string {
	switch b {
	case BackendMemory:
		return "memory"
	case BackendTemporal:
		return "temporal"
	default:
		return "unknown"
	}
}

// Mode is the interpretation mode of a block.
type Mode int

const (
	// ModeUnknown is the zero value; never produced by a valid parse.
	ModeUnknown Mode = iota
	// ModeStatic compiles the block to a fixed graph at translation time.
	ModeStatic
	// ModeDynamic tree-walks the block's AST at runtime.
	ModeDynamic
)

// String returns the keyword form used in source (or "unknown").
func (m Mode) String() string {
	switch m {
	case ModeStatic:
		return "static"
	case ModeDynamic:
		return "dynamic"
	default:
		return "unknown"
	}
}

// === Expression nodes ======================================================

// Node is the sealed sum of expression types that can appear inside a
// Block.Body or as a sub-expression. New variants require extending the
// parser AND adding a node() method here.
type Node interface {
	node()
}

// Pipeline is `a >=> b >=> c` — a sequence of stages where each stage's
// output is the next stage's input. Stages has length >= 1; a single
// Call wrapped in a Pipeline is permitted and harmless (the lowering
// pass collapses trivial pipelines).
type Pipeline struct {
	Stages []Node
}

// Parallel is `a <*> b <*> c` — a fanout of branches that run with
// the same upstream input and produce sibling outputs. Reserved for
// future use; the MVP parser does not yet emit this variant.
type Parallel struct {
	Branches []Node
}

// Call is `name "arg1" "arg2" ...` — a builtin invocation. The name
// resolves to a BuiltinSpec at Resolve time; Args are literal values
// from the source.
type Call struct {
	Name string
	Args []Arg
}

func (Pipeline) node() {}
func (Parallel) node() {}
func (Call) node()     {}

// === Argument literals =====================================================

// Arg is a sealed sum of literal argument types. The MVP parser only
// produces StringArg; NumArg is defined so future grammar extensions
// can add numeric literals without restructuring callers.
type Arg interface {
	arg()
}

// StringArg is a double-quoted string literal.
type StringArg struct {
	Value string
}

// NumArg is a numeric literal. Reserved for future use.
type NumArg struct {
	Value float64
}

func (StringArg) arg() {}
func (NumArg) arg()    {}
