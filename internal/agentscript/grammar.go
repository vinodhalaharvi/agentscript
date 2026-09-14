// Package agentscript — grammar.go holds the in-process interpreter's
// program representation.
//
// It used to also hold a participle parser for the operator dialect
// (`search "x" >=> summarize`). That dialect is gone: s-expressions are
// the only surface syntax, and the single front end lives in pkg/script.
// Source text now reaches this interpreter one way only:
//
//	Source >>> script.Parse >>> script.Resolve >>> scriptmem.RunMemory
//
// scriptmem builds these types directly from the resolved AST, so they
// stay. The struct tags went with the parser — they described participle
// productions and mean nothing now.
package agentscript

// Program represents a complete AgentScript program.
type Program struct {
	Statements []*Statement
}

// Statement is a command or a fan-out group, optionally piping its
// output into a following statement.
type Statement struct {
	Parallel *Parallel
	Command  *Command
	Pipe     *Statement
}

// Parallel is a fan-out group: branches that run concurrently on the
// same input. Branches is consumed by runtime.executeParallel.
type Parallel struct {
	Branches []*Statement
}

// Command is a single verb invocation with up to four string arguments.
//
// The verb was formerly constrained by a parser alternation listing every
// name. Validation now happens in script.Resolve against the registry,
// which is the single source of truth for the vocabulary, so Action is a
// plain string here.
type Command struct {
	Action string
	Arg    string
	Arg2   string
	Arg3   string
	Arg4   string
}
