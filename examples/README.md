# Examples

AgentScript programs are s-expressions. Run one with:

    agentscript examples/tech-digest.as

Or inline:

    agentscript -e '(pipe (search "go releases") summarize)'

| File | Shape it demonstrates |
| --- | --- |
| `hello.as` | the smallest program; implicit memory block |
| `research.as` | `(pipe ...)` — sequential composition |
| `tech-digest.as` | `(par ...)` + `merge` — fan-out |
| `stock-alert.as` | `(when ...)` — the conditional stage |
| `framework-compare.as` | nested fan-out |
| `mcp-issues.as` | calling MCP server tools |
| `durable-echo.as` | `(block :backend temporal ...)` |

## Syntax in one screen

    (pipe STAGE STAGE ...)    each stage's output feeds the next
    (par BRANCH BRANCH ...)   branches run concurrently on the same input
    (verb "arg" "arg")        a command call
    verb                      a command call with no arguments

    (block :backend memory|temporal|sibyl :mode static|dynamic BODY)

The `(block ...)` wrapper is optional. Without it a program runs on the
memory backend in static mode. Backend selection lives in the source,
never in a runtime flag, so a program always runs the same way wherever
it is invoked from.

Comments start with `;` or `//` and run to end of line.

## One wrinkle

`echo` is a temporal activity, not an interpreter verb. It resolves on
either backend but only runs on temporal, so `(echo "hi")` without a
`(block :backend temporal ...)` wrapper fails at execution rather than
at compile time. The memory examples use `exec` instead.
