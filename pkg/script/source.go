// Package script implements the AgentScript → Sibyl translator.
//
// This package is the new arrow-first translator described in
// docs/dsl-to-sibyl-translator.md. It sits alongside the existing
// internal/agentscript runtime (the memory-backend evaluator) and
// shares no code with it.
//
// The translator is a pipeline of weft.Arrow values:
//
//	Source >>> Parse >>> Resolve >>> Lower >>> Finalize >>> Validate >>> Submit
//
// PR-A.1 implements Source, the AST types, and Parse. Subsequent PRs
// add Resolve (PR-A.2) and the lower-through-submit phases (PR-A.3).
package script

// Source is a unit of AgentScript text submitted to the translator. It
// is a thin alias around string so the type lineage in the memo
// (Source >>> AST >>> ...) lines up with actual Go types.
type Source string

// String implements fmt.Stringer for ergonomic logging.
func (s Source) String() string { return string(s) }
