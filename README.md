# AgentScript - CSP-M for AI Agents 🔬

A formal process algebra (CSP-M) interpreter for orchestrating AI agents. Built for the **Gemini 3 Hackathon**.

## The Big Idea

Instead of ad-hoc workflow languages, AgentScript uses **real CSP-M** (Communicating Sequential Processes - Machine readable) - a mathematically rigorous process algebra used in formal verification.

```csp
-- Parallel AI research with formal semantics
(search!"Google" -> analyze -> SKIP ||| search!"Microsoft" -> analyze -> SKIP) 
  ; merge -> ask!"compare these companies" -> SKIP
```

## Why CSP-M?

| Feature | Benefit |
|---------|---------|
| **Formal semantics** | Mathematically defined behavior |
| **Parallel composition** | `P \|\|\| Q` runs concurrently |
| **Sequential composition** | `P ; Q` runs in order |
| **Academic credibility** | Real process algebra, not made-up DSL |
| **Tool compatibility** | Can verify with FDR4 model checker |

## Quick Start

```bash
export GEMINI_API_KEY="your-key"

# Build
go build -o agentscript .

# Simple expression
./agentscript -e 'search!"golang" -> summarize -> save!"out.md" -> SKIP'

# Parallel execution
./agentscript -e '(search!"A" -> SKIP ||| search!"B" -> SKIP) ; merge -> SKIP'

# From file
./agentscript -f examples/competitor-analysis.csp

# Natural language (translates to CSP-M)
./agentscript -n "compare Google and Microsoft"

# Interactive REPL
./agentscript -i
```

## CSP-M Syntax

### Processes
```csp
SKIP                -- terminate successfully
STOP                -- deadlock (avoid!)
event -> P          -- do event, then P
P ; Q               -- sequential: P then Q
P ||| Q             -- parallel: P and Q concurrently
P [] Q              -- choice: either P or Q
(P)                 -- grouping
```

### Events (Built-in Actions)
```csp
search!"query"      -- search for information
summarize           -- summarize input
analyze!"focus"     -- analyze with optional focus
ask!"question"      -- ask a question
save!"filename"     -- save to file
read!"filename"     -- read from file
list!"path"         -- list directory
merge               -- combine parallel results
email!"address"     -- send email
```

### Process Definitions
```csp
-- Define reusable processes
channel search, analyze, merge, ask, save

BRANCH1 = search!"topic A" -> analyze -> SKIP
BRANCH2 = search!"topic B" -> analyze -> SKIP

COMPARE = (BRANCH1 ||| BRANCH2) ; merge -> ask!"compare" -> SKIP

MAIN = COMPARE
```

## Examples

### Simple Research
```csp
search!"golang best practices" -> summarize -> save!"guide.md" -> SKIP
```

### Parallel Comparison
```csp
(search!"Tesla" -> analyze -> SKIP 
 ||| search!"Ford" -> analyze -> SKIP 
 ||| search!"GM" -> analyze -> SKIP) 
; merge -> ask!"who is winning the EV race?" -> SKIP
```

### Full Workflow File
```csp
-- competitor-analysis.csp
channel search, analyze, merge, ask, save

GOOGLE = search!"Google strengths" -> analyze -> SKIP
MICROSOFT = search!"Microsoft strengths" -> analyze -> SKIP

MAIN = (GOOGLE ||| MICROSOFT) ; merge -> ask!"compare" -> save!"analysis.md" -> SKIP
```

## Architecture

```
                    ┌─────────────────────┐
                    │   Natural Language  │
                    │  "compare A and B"  │
                    └──────────┬──────────┘
                               │ Gemini translates
                               ▼
┌─────────────────────────────────────────────────────────┐
│                      CSP-M                              │
│  (search!"A" -> SKIP ||| search!"B" -> SKIP) ; merge   │
└──────────┬──────────────────────────────────────────────┘
           │ Participle parser
           ▼
┌─────────────────────────────────────────────────────────┐
│                       AST                               │
│  ProcessExpr → ParallelExpr → PrefixExpr → Event       │
└──────────┬──────────────────────────────────────────────┘
           │ Runtime interprets
           ▼
┌─────────────────────────────────────────────────────────┐
│                    Execution                            │
│  ┌─────────┐  goroutines  ┌─────────┐                  │
│  │ Branch1 │ ──────────── │ Branch2 │                  │
│  └────┬────┘   sync.WG    └────┬────┘                  │
│       └──────────┬─────────────┘                       │
│                  ▼                                      │
│              merge → ask → output                       │
└─────────────────────────────────────────────────────────┘
```

## Project Structure

```
agentscript/
├── main.go          # CLI entry point & REPL
├── grammar.go       # CSP-M parser (Participle)
├── runtime.go       # AST interpreter with parallel execution
├── translator.go    # Natural language → CSP-M
├── client.go        # Gemini API client
├── examples/
│   ├── competitor-analysis.csp
│   ├── ai-comparison.csp
│   ├── simple-research.csp
│   └── executive-report.csp
└── README.md
```

## Formal Verification (Future)

CSP-M specifications can be verified with FDR4 model checker:
- Deadlock freedom
- Livelock freedom
- Refinement checking

```csp
-- This could be checked for deadlocks in FDR4
assert MAIN :[deadlock free]
```

## Built With

- [Participle](https://github.com/alecthomas/participle) - Parser generator for Go
- [Gemini API](https://ai.google.dev/) - Google's AI model
- [CSP](https://en.wikipedia.org/wiki/Communicating_sequential_processes) - Hoare's process algebra

## License

MIT

---

Built with 🔬 for the Gemini 3 Hackathon
