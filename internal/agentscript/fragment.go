// Package agentscript — fragment.go parses s-expression fragments into
// the interpreter's own Program representation.
//
// Two places need to parse source at execution time rather than at
// compile time: RunDSL (the Executor seam, used by the `agent` verb to
// run a pipeline an LLM just wrote) and a matched `match` arm, whose
// body is carried as a string. Neither can go through pkg/script,
// because that would mean importing scriptmem to get back to a
// *Program, and scriptmem imports this package.
//
// So this file depends only on pkg/script/sexpr — the reader, which
// imports nothing but the standard library — and does the small mapping
// to Program itself. The surface syntax is identical to the one
// pkg/script accepts, minus the (block ...) wrapper, which makes no
// sense for a fragment that is already executing in-process.
//
//	(pipe (search "ai") summarize)
//	(par (rss "hn") (reddit "r/golang"))
//	(notify "slack")
//	summarize
package agentscript

import (
	"fmt"

	"github.com/vinodhalaharvi/agentscript/pkg/script/sexpr"
)

// Parse parses an s-expression fragment into a Program. Each top-level
// form becomes one Statement.
func Parse(input string) (*Program, error) {
	forms, err := sexpr.Read(input)
	if err != nil {
		return nil, err
	}
	prog := &Program{}
	for i, f := range forms {
		st, err := statementFromForm(f)
		if err != nil {
			return nil, fmt.Errorf("statement %d: %w", i, err)
		}
		prog.Statements = append(prog.Statements, st)
	}
	return prog, nil
}

func statementFromForm(f sexpr.Form) (*Statement, error) {
	switch f.Kind {
	case sexpr.KindSymbol:
		return &Statement{Command: &Command{Action: f.Text}}, nil
	case sexpr.KindList:
		return statementFromList(f)
	default:
		return nil, fmt.Errorf("%s: expected a call or a form, got %s", f.Pos, f.Kind)
	}
}

func statementFromList(f sexpr.Form) (*Statement, error) {
	if len(f.Items) == 0 {
		return nil, fmt.Errorf("%s: empty form ()", f.Pos)
	}
	head := f.Items[0]
	if head.Kind != sexpr.KindSymbol {
		return nil, fmt.Errorf("%s: form head must be a symbol, got %s", head.Pos, head.Kind)
	}
	switch head.Text {
	case "pipe":
		return pipeStatement(f)
	case "par":
		return parStatement(f)
	default:
		cmd, err := commandFromList(f)
		if err != nil {
			return nil, err
		}
		return &Statement{Command: cmd}, nil
	}
}

// pipeStatement builds the .Pipe chain the interpreter walks: stage one
// holds a pointer to stage two, and so on.
func pipeStatement(f sexpr.Form) (*Statement, error) {
	rest := f.Items[1:]
	if len(rest) == 0 {
		return nil, fmt.Errorf("%s: pipe needs at least one stage", f.Pos)
	}
	stages := make([]*Statement, 0, len(rest))
	for i, item := range rest {
		st, err := statementFromForm(item)
		if err != nil {
			return nil, fmt.Errorf("pipe stage %d: %w", i, err)
		}
		stages = append(stages, st)
	}
	for i := 0; i < len(stages)-1; i++ {
		stages[i].Pipe = stages[i+1]
	}
	return stages[0], nil
}

func parStatement(f sexpr.Form) (*Statement, error) {
	rest := f.Items[1:]
	if len(rest) < 2 {
		return nil, fmt.Errorf("%s: par needs at least two branches, got %d", f.Pos, len(rest))
	}
	branches := make([]*Statement, 0, len(rest))
	for i, item := range rest {
		st, err := statementFromForm(item)
		if err != nil {
			return nil, fmt.Errorf("par branch %d: %w", i, err)
		}
		branches = append(branches, st)
	}
	return &Statement{Parallel: &Parallel{Branches: branches}}, nil
}

// commandFromList fills Arg..Arg4 positionally. The interpreter has
// always taken at most four arguments; a fifth is a source error rather
// than something to silently drop.
func commandFromList(f sexpr.Form) (*Command, error) {
	head := f.Items[0]
	args := f.Items[1:]
	if len(args) > 4 {
		return nil, fmt.Errorf("%s: %s takes at most 4 arguments, got %d",
			head.Pos, head.Text, len(args))
	}
	cmd := &Command{Action: head.Text}
	slots := []*string{&cmd.Arg, &cmd.Arg2, &cmd.Arg3, &cmd.Arg4}
	for i, a := range args {
		switch a.Kind {
		case sexpr.KindString:
			*slots[i] = a.Text
		case sexpr.KindNumber:
			*slots[i] = a.Text
		default:
			return nil, fmt.Errorf("%s: %s expects literal arguments, got %s",
				a.Pos, head.Text, a.Kind)
		}
	}
	return cmd, nil
}
