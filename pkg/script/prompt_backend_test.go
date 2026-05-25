package script_test

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/agentscript/pkg/script"
)

// The grammar prompt must teach BOTH backends and default to memory.
// Regression guard for the bug where the prompt only ever showed
// "temporal static ( ... )", so the LLM emitted temporal even when the
// user said "in memory" — and memory-only verbs were then rejected as
// "not available on this backend".
func TestBuildPrompt_TeachesBothBackendsDefaultMemory(t *testing.T) {
	p := script.BuildPrompt(script.DefaultRegistry())
	for _, want := range []string{
		"memory static",   // memory block form taught
		"temporal static", // temporal block form taught
		"DEFAULT",         // memory is the default
		"in memory",       // explicit "in memory" → memory
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt should contain %q so the LLM can pick the memory backend", want)
		}
	}
}
