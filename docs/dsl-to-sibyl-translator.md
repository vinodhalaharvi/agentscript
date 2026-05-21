# AgentScript → Sibyl: DSL Translator Design Memo

**Status:** Translator MVP complete. The full pipeline (Parse → Resolve → Lower → Finalize → Validate → Submit) is merged and importable at `pkg/script/`; `echo` runs end-to-end against Sibyl's `PlanWorkflow`. See "Implementation status" below.
**Companion repo:** [vinodhalaharvi/sibyl](https://github.com/vinodhalaharvi/sibyl) (the execution layer)
**Implements against:** Sibyl's `agents.WithOAuth` behavior, `ConvergeWorkflow`, `ToolAgentArrow`, `SupervisorWorkflow`, and the existing DAG execution model.

---

## Implementation status

The translator is being built in slices (see §14). Current state:

| Phase | Arrow | Status |
|-------|-------|--------|
| Parse | `Arrow[Source, AST]` | **Merged** — `pkg/script/parse.go` |
| Resolve | `Arrow[AST, ResolvedAST]` | **Merged** — `pkg/script/resolve.go` + `registry/` |
| Lower | `Arrow[ResolvedAST, Lowered]` | **Merged** — `pkg/script/lower.go` |
| Finalize | `Arrow[Lowered, sibyl.Plan]` | **Merged** — `pkg/script/lower.go` |
| Validate | `Arrow[sibyl.Plan, sibyl.Plan]` | **Merged** — `pkg/script/lower.go` |
| Submit | `Arrow[sibyl.Plan, WorkflowHandle]` | **Merged** — `pkg/script/submit.go` |

The full pipeline (`Compile` = Parse..Validate, `Run` = Compile + Submit)
is in `pkg/script/submit.go`, importable from outside the module. The
lowering target is Sibyl's serializable `Plan` (a DAG of named-activity
references) executed by the generic `PlanWorkflow` — not the removed
in-process closure DAG. The first builtin, `echo`, binds to Sibyl's
`Echo` activity; the `agentscript-run` CLI compiles a `.as` file to a
Plan (`--dry-run` to print it, otherwise submit to a Temporal worker).

On the Sibyl side, the convergence-primitive arrows this memo depends on
(`ConvergeArrow`, `SupervisorArrow`; `ToolAgentArrow` already existed) are
merged, as are the serializable `Plan`, `PlanWorkflow`, and `Echo`
activity the translator lowers into.

---

## 1. Purpose

This memo describes how an AgentScript program — a pipeline-flavored DSL with operators like `>=>` and `<*>` — is translated into Sibyl's execution model. The goal is not to specify the DSL syntax (which exists and continues to evolve), but to specify the **compilation pipeline**: how source code becomes an executable Sibyl DAG, what intermediate representations exist, and how the translator itself is built.

The translator is designed to be:

- **Arrow-first.** Every phase of compilation is a `weft.Arrow`. The pipeline is arrow composition. Imperative code lives only at the seams (third-party parser, Temporal SDK).
- **Functional.** No global mutation, no builder pattern. Intermediate representations are values. Lowering is fold over the AST, not mutation of a DAG-under-construction.
- **Two-backend.** The same AST lowers to two different execution backends — Sibyl's Temporal-backed DAG executor and an in-process memory evaluator. The choice is declared per block in source.
- **Composition-open.** The translator is open for composition the same way Sibyl agents are. Every phase can be wrapped with the same `weft` combinators that wrap agents (logging, retry, timeout, caching).

---

## 2. Scope and non-goals

**In scope:**

- The type lineage from `Source` to `WorkflowHandle`
- The semantics of `static` / `dynamic` (interpretation mode) and `temporal` / `memory` (execution backend) block annotations
- The mapping of DSL operators (`>=>`, `<*>`) to Sibyl DAG primitives
- The contract between the agentscript registry and Sibyl agent registration
- How `agents.WithOAuth` and future credential behaviors integrate
- How Sibyl's existing convergence primitives (`ConvergeWorkflow`, `ToolAgentArrow`, `SupervisorWorkflow`) are exposed in the DSL
- The minimum-viable slice for the first end-to-end demo

**Out of scope:**

- Specific DSL surface syntax (covered by AgentScript's existing grammar; this memo treats the parser as a black box producing an AST)
- React/web UI (deferred; CLI is the MVP UX)
- The in-memory evaluator's internals (treated as an existing component with a known shape)
- Productization, packaging, distribution
- DSL editor tooling (LSP, autocomplete, type-on-hover)

**Explicitly deferred:**

- Multi-credential-source generalization (`WithCredential` over `WithOAuth`) — covered in §10 as a forward-compatible plan
- Auto-pause-on-MissingCredential at workflow level — covered briefly in §9
- Cross-block data flow (sharing values between top-level blocks) — covered in §12 with a placeholder
- Replacement for the deleted `coordinate` multi-agent coordination primitives — deferred until a real workload demands them (see §11)

---

## 3. Architectural premise

The single most important architectural decision underlying this memo: **agentscript depends on sibyl, not the other way around.**

AgentScript is a **presentation layer**. It owns:

- Surface syntax (lexer, grammar, parser)
- A representation of pipelines (AST)
- A translation strategy (lowering to Sibyl primitives)
- An optional in-memory evaluator for fast local iteration

Sibyl is the **execution layer**. It owns:

- Typed agent registration (`agents.Spec`, `agents.Behavior`)
- Per-agent credential resolution (`agents.WithOAuth`)
- Convergence workflows (`ConvergeWorkflow`, `ToolAgentArrow`, `SupervisorWorkflow`)
- HITL channels
- DAG composition and Temporal-backed execution

This direction is deliberate. The execution model is the load-bearing piece; it should not be coupled to a particular DSL. A DSL is a surface — it can be revised, replaced, or have peer DSLs (a YAML config form, a programmatic builder, a visual editor) — without disturbing the executor below. Sibyl evolves independently.

A consequence: AgentScript's own runtime (the existing tree-walking evaluator) is preserved as the **memory backend**. It is not displaced. Both backends are first-class. Selection is per-block, declared in source.

### 3.1 What agentscript will not contain

The load-bearing rule that follows from §3, stated explicitly so it can be enforced in code review:

> **AgentScript is presentation. AgentScript owns surface syntax and translation. AgentScript does not own behavior, state, durability, identity, or domain abstractions. If a proposed addition involves any of those, it goes in Sibyl or doesn't go in at all.**

Three concrete consequences:

1. **No execution logic in agentscript.** If a feature needs to *run*, it runs in Sibyl. AgentScript's job ends at "produce a Sibyl invocation."
2. **No domain abstractions in agentscript.** Blackboards, consensus, belief alignment, retry policies, identity — none of that lives in agentscript. AgentScript can have *syntax* for any of them, but the implementation belongs in Sibyl.
3. **AgentScript can be re-implemented or replaced** without losing capability. If the syntax becomes a YAML config, a visual editor, or a different DSL tomorrow, nothing of substance is lost.

What this rules out, going forward:

- **No runtime engines in agentscript.** If a builtin needs a coordination engine, retry engine, state machine, etc., that's a Sibyl primitive that agentscript invokes.
- **No persistent state in agentscript.** If something needs to survive a crash or be durable, that's Sibyl.
- **No agent-specific helpers in agentscript that reach into vendor SDKs directly.** Either the vendor logic is a Sibyl agent (preferred) or it's plugin-local code that registers a Sibyl agent at the seam.
- **No DSL features that can't be expressed as "compile to Sibyl primitives."** If a proposed feature can't be lowered to a DAG of Sibyl agents, the right question isn't "how do we hack agentscript to do it" — it's "should Sibyl have a new primitive for this."

Historical note: the recently-deleted `pkg/coordinate` and `pkg/intent` packages were violations of this rule. They lived in agentscript but owned execution behavior, ran their own engines, and didn't compose with the rest of the framework. The deletion (see [agentscript#3](https://github.com/vinodhalaharvi/agentscript/pull/3)) restored the layering. This rule exists to prevent the same drift from recurring.

---

## 4. Block annotation grammar

Each top-level block is annotated with a **backend** keyword and a **mode** keyword. **No defaults.** Every block declares both, explicitly.

```
<backend> <mode> ( <expression> )
```

Where:

- `<backend>` ∈ `{ memory, temporal }` — *where* the block executes
- `<mode>` ∈ `{ static, dynamic }` — *how* the block is interpreted

The two attributes are **orthogonal**. All four combinations are meaningful:

| | `memory` | `temporal` |
|---|---|---|
| **`static`** | Compile to internal representation, evaluate in-process. Fast feedback, no durability. | Compile to Sibyl DAG, submit to Temporal. Production-grade durability and observability. |
| **`dynamic`** | Tree-walk the AST in-process. REPL feel, runtime decisions. | Tree-walk inside one Temporal-backed agent. Durable workflow, dynamic internal decisions. |

Block delimiters are **parentheses**, not braces. This matches the existing DSL aesthetic where `(a <*> b)` is a sub-expression — every grouping uses the same delimiter, from inner-expression to top-level block.

### Examples

```
temporal static (
  (weather "SF" <*> stock "NVDA")
  >=> summarize
  >=> slack "#standup"
)
```

```
memory dynamic (
  ai_pick_strategy "user-prefs"
)
```

A script may contain multiple blocks with different annotations:

```
temporal static (
  fetch_data "sales-q3" >=> persist "db"
)

memory dynamic (
  preview_strategy "draft"
)
```

Each block compiles independently to its declared backend.

### Why no defaults

Reading a script tells you exactly what it does. There is no "I think this is temporal because of the file header." No surprise when a script moves between files. The few extra keywords are information, not noise — the same reasoning that produced explicit plugin registration (no `init()` side effects).

### Parsing note

`memory`, `temporal`, `static`, and `dynamic` are **reserved keywords**. The grammar treats `<backend> <mode> (` unambiguously as the start of a block. Order is enforced grammatically: backend first, then mode. (A linter may normalize to canonical order.)

---

## 5. The type lineage

The translator transforms `Source` through a series of intermediate types, ending in either a `sibyl.DAG` (for temporal blocks) or a `memory.Program` (for memory blocks).

```
Source
  │
  │  Parse
  ▼
AST
  │
  │  Resolve
  ▼
ResolvedAST
  │
  │  Lower
  ▼
DAGFragment                  (or MemoryFragment for memory backend)
  │
  │  Finalize
  ▼
sibyl.DAG                    (or memory.Program)
  │
  │  Validate                (identity on success; error on cycle/orphan)
  ▼
sibyl.DAG                    (validated)
  │
  │  Submit                  (only for temporal backend)
  ▼
WorkflowHandle
```

Each phase is a `weft.Arrow`. The composed pipeline is itself an arrow:

```go
CompileToSibyl   : Arrow[Source, sibyl.DAG]
CompileToSibyl   = Parse >>> Resolve >>> LowerToSibyl >>> Finalize >>> Validate

CompileAndSubmit : Arrow[Source, WorkflowHandle]
CompileAndSubmit = CompileToSibyl >>> Submit

CompileToMemory  : Arrow[Source, memory.Program]
CompileToMemory  = Parse >>> Resolve >>> LowerToMemory >>> FinalizeMemory
```

The two backends **share `Parse` and `Resolve`**. They diverge at `Lower`. This is the load-bearing invariant: the AST is backend-agnostic.

A script that mixes block-level backends becomes a *collection* of per-block compilation results, executed (or submitted) accordingly. The whole-script compilation arrow has shape:

```
CompileScript : Arrow[Source, Program]
```

where `Program` is a sum of per-block compiled outputs.

### Phase-by-phase summary

| Phase | Arrow signature | Purpose |
|---|---|---|
| Parse | `Arrow[Source, AST]` | Lex and parse source text into an immutable AST. Wraps `participle`. |
| Resolve | `Arrow[AST, ResolvedAST]` | Look up each `Call`'s `Name` in the registry; attach `BuiltinSpec` metadata. Reject unknown names. |
| Lower | `Arrow[ResolvedAST, DAGFragment]` (or `MemoryFragment`) | Walk the resolved AST; emit a fragment per node; combine via fragment-space combinators. |
| Finalize | `Arrow[DAGFragment, sibyl.DAG]` | Close open ports on the top-level fragment; wire entry/exit nodes; produce an executable DAG. |
| Validate | `Arrow[sibyl.DAG, sibyl.DAG]` | Reject cycles, orphan nodes, type mismatches. Identity on success. |
| Submit | `Arrow[sibyl.DAG, WorkflowHandle]` | Submit to a Sibyl worker (Temporal client). Returns a handle for tracking. |

### Where imperative code is unavoidable

Two leaves:

- **Inside `Parse`** — `participle.Parser.Parse` is a third-party function call. Wrapped in an arrow; the arrow body contains the function call, but the *caller* (the translator pipeline) only sees the arrow.
- **Inside `Submit`** — `client.ExecuteWorkflow` is a Temporal SDK call. Same treatment.

These are the only two places. Everything else — including all lowering, all fragment composition, all validation — is functional.

---

## 6. The AST

The AST is a sum type of expressions, parameterized over block annotations. Block-level annotations (`backend`, `mode`) live on the outer `Block` node; everything inside is annotation-free until lowering.

Conceptually:

```
AST           = List<Block>

Block         = { Backend, Mode, Body : Node }

Backend       = memory | temporal
Mode          = static | dynamic

Node          = Pipeline | Parallel | Call

Pipeline      = { Stages   : List<Node> }    -- a >=> b >=> c
Parallel      = { Branches : List<Node> }    -- a <*> b <*> c
Call          = { Name : String, Args : List<Arg> }

Arg           = StringLit | NumLit | ... (extensible)
```

`Node` is a sealed sum — closed for extension except via parser changes. Every `Node` is **immutable**. Lowering produces new values; it never mutates the AST.

Future operators (e.g. a grouping operator that collapses multiple things into a single agent boundary) add new `Node` variants. No existing variants change.

---

## 7. The registry and resolution

The agentscript registry is the bridge from DSL names to Sibyl agent IDs. It is **explicit** — there is no `init()`-based auto-registration. Programs construct a registry and register builtins explicitly:

```go
reg := script.NewRegistry()
reg.Register(weather.Spec())   // weather plugin exports a Spec()
reg.Register(stock.Spec())
reg.Register(summarize.Spec())
reg.Register(slack.Spec())
```

A `BuiltinSpec` declares:

```
BuiltinSpec = {
    Name      : String          -- DSL-visible (e.g. "weather")
    AgentID   : String          -- Sibyl agent ID (e.g. "agentscript/weather")
    Vendors   : List<String>    -- for documentation; advisory
    ArgSchema : ArgSchema       -- expected arg shape for validation
    AuthHint  : AuthSource      -- declared default credential source (advisory; see §10)
}
```

**The plugin separately registers a Sibyl agent.** The two registrations are paired but separate calls — the agentscript registry says "the DSL name `weather` resolves to agent ID `agentscript/weather`," and Sibyl's agent registry says "agent ID `agentscript/weather` is implemented by this `Run` arrow with these behaviors." A plugin's explicit-call API typically does both:

```go
// inside the weather plugin package
func Spec() script.BuiltinSpec {
    // Idempotent: registers with Sibyl on first call, returns the DSL-side spec.
    sibylAgent := agents.MustRegister(agents.Spec[WeatherArgs, WeatherResult]{
        ID:      "agentscript/weather",
        Vendors: []string{"weatherapi"},
        Run:     weatherRunArrow,
        Behaviors: []agents.Behavior[WeatherArgs, WeatherResult]{
            agents.WithOAuth[WeatherArgs, WeatherResult](
                store, refresh,
                agents.OAuthPolicy[WeatherArgs]{
                    Provider:        "weatherapi",
                    ResolveIdentity: agents.InvokedByIdentity[WeatherArgs]("weatherapi"),
                    Inject:          agents.StructFieldInjector(func(r *WeatherArgs, t oauth.TokenPair) { r.Token = t }),
                },
            ),
        },
    })

    return script.BuiltinSpec{
        Name:      "weather",
        AgentID:   sibylAgent.ID,
        Vendors:   sibylAgent.Vendors,
        ArgSchema: script.ArgSchema{ Params: []script.ParamSpec{{ Name: "location", Type: script.StringT }} },
        AuthHint:  script.AuthSource{ Kind: script.AuthOAuth, Provider: "weatherapi" },
    }
}
```

Resolution walks the AST and:

1. For each `Call`, looks up `Call.Name` in the registry. If absent, returns an error.
2. Attaches the `BuiltinSpec` to the call, producing `ResolvedCall`.
3. Validates arg arity and shapes against `BuiltinSpec.ArgSchema`. Wrong types or wrong arity → error.

Resolution is a pure function over the AST and registry. The output `ResolvedAST` is a new value; the input AST is unchanged.

---

## 8. Lowering and fragments

This is the centerpiece of the design. Lowering does **not** mutate a DAG-under-construction. It produces a typed value — a `DAGFragment` — by recursive composition. Operators in the DSL become combinators on fragments.

### The fragment abstraction

A `DAGFragment` is a sub-DAG with named input and output ports:

```
DAGFragment = {
    Nodes : List<DAGNode>
    Edges : List<DAGEdge>
    In    : List<PortRef>      -- input ports (where upstream connects)
    Out   : List<PortRef>      -- output ports (where downstream consumes)
}
```

A fragment is **closed over its structure** but **open at its boundaries**. Two fragments compose by port-matching:

```
sequence : Arrow[Pair[DAGFragment, DAGFragment], DAGFragment]
fanout   : Arrow[List[DAGFragment],              DAGFragment]
```

Sequencing wires the `Out` of the first to the `In` of the second. Fanout takes a list of fragments and produces a fragment whose `In` is the union of theirs and whose `Out` collects all their outputs. Both are pure value transformations.

### Per-AST-node lowering arrows

Each kind of `ResolvedNode` lowers via its own arrow:

```
lowerCall     : Arrow[ResolvedCall,     DAGFragment]
lowerPipeline : Arrow[ResolvedPipeline, DAGFragment]
lowerParallel : Arrow[ResolvedParallel, DAGFragment]
lowerBlock    : Arrow[ResolvedBlock,    DAGFragment]
```

`lowerCall` produces a single-node fragment: one `DAGNode` with the resolved `AgentID`, the bound arguments, one `In` port (for upstream data), one `Out` port (for the result).

`lowerPipeline` is a fold using `sequence`:

```
lowerPipeline(p) = foldSeq(p.Stages, lowerStage, sequence)
```

`lowerParallel` is a fold using `fanout`:

```
lowerParallel(p) = foldFan(p.Branches, lowerStage, fanout)
```

Where `lowerStage` is the polymorphic dispatch over `Node` kind, itself an arrow.

A `Parallel` followed by anything that isn't a `Parallel` synthesizes a `join` node — a no-vendor, no-OAuth, type-shaping agent that tuples the parallel branches into a single value. This is generated by `sequence` automatically when its left input has multiple `Out` ports.

### Operator → graph structure

The DSL operators map to **graph shapes**, not to arrow combinators inside an agent:

| DSL operator | AST node | Fragment combinator | DAG result |
|---|---|---|---|
| `>=>` | `Pipeline` | `sequence` | edges from upstream `Out` to downstream `In` |
| `<*>` | `Parallel` | `fanout` | sibling fragments, no edges between them; merged at next `sequence` via synthetic join |
| (future grouping operator) | TBD | `groupFragment` | multiple AST nodes collapse to a single DAG node |

This separation matters: `weft` arrow composition (`>>>`, `&&&`) is the *internal* composition of a single agent's `Run`. The DSL operators (`>=>`, `<*>`) are the *external* composition of agents in a DAG. They live at different layers and do not interfere.

### Why fragments, not a DAG builder

A `DAG.Builder` with `AddNode`, `AddEdge`, etc. is the imperative shape. It works but it's a closed system — lowering is a method on a builder, and the only way to extend lowering is to add methods.

Fragments are values. Lowering produces values. Composition of values is composition of arrows. New operators are new combinator arrows. Testing a lowering is calling an arrow on a sample input and asserting on its output. There's no builder to mock.

This is the same shift that `agents.WithOAuth` made when it chose policy-as-arrow-fields instead of policy-as-interface-methods. The translator earns the same property: every seam is an arrow.

---

## 9. Backends

### Temporal backend (production)

`LowerToSibyl : Arrow[ResolvedAST, DAGFragment]` produces a fragment using the lowering described in §8. `Finalize` then converts the fragment into a `sibyl.DAG` by closing open ports against synthesized entry/exit nodes. `Submit` sends the DAG to a Sibyl worker, which executes it as a Temporal workflow.

**Per-agent OAuth fires naturally.** Each DAG node is a Sibyl agent invocation. The agent's registered `Behaviors` include `WithOAuth`. When the node runs, it resolves identity from `AgentContext.InvokedBy` (the workflow's submitter), loads a token, refreshes if needed, injects, runs. Per-agent identity propagation comes for free.

**Durability and observability come for free.** The DAG is a Temporal workflow; standard Sibyl/Temporal observability applies (workflow history, replay, per-activity timing). Failures are retried per Sibyl's retry policy.

### Memory backend (fast iteration)

`LowerToMemory : Arrow[ResolvedAST, memory.Program]` produces a value consumable by AgentScript's existing tree-walking evaluator. The evaluator is **preserved as-is** — its existing behavior is the contract.

The memory backend has reduced guarantees:

- No durability
- No cross-process observability
- No Temporal-style retries
- No multi-tenancy guarantees (single-process)

But it has reduced friction:

- No Temporal cluster required
- Sub-second startup
- Trivial to embed in tests

The two backends are **not expected to be byte-identical** in observable behavior — they share the AST and resolution but differ in retry policy, observability, identity propagation. A useful cross-check (deferred work): run both backends against the same program and assert that final results agree, modulo non-determinism.

### `dynamic` mode under each backend

`dynamic` collapses the block to a single "dynamic evaluator" agent. Under `temporal`, this is one Temporal-tracked node whose `Run` walks the block's AST at runtime. Under `memory`, it's just the existing in-process evaluator running the AST directly.

A nested `dynamic` block inside a `static temporal` block becomes one DAG node in the outer DAG; the inner walking happens inside that node's invocation. A nested `static` block inside a `dynamic` block precomputes its fragment at parse time and executes that fragment inside the dynamic evaluator's scope. The boundary is always crisp: one execution model per DAG node.

### Failure modes per backend

**Translator-level (both backends):**

- Parse error → fail before resolution
- Unknown builtin or arg shape mismatch → fail before lowering
- Cycle or orphan in the lowered fragment → fail at `Validate`

**Runtime-level (temporal):**

- `agents.MissingCredentialError` from a node → the workflow either errors out or (deferred work) pauses pending re-auth via the existing `OAuthFlowWorkflow` infrastructure
- Activity timeout → Sibyl/Temporal retry policy applies
- Worker crash → Temporal resumes the workflow from durable state

**Runtime-level (memory):**

- `agents.MissingCredentialError` → returned as a program error to the caller
- Panic in a builtin → propagated to the caller; no recovery

The memory backend deliberately does not attempt to emulate Temporal-style durability. Users who need durability use the temporal backend. The two backends serve different stages of the development lifecycle.

---

## 10. Authentication: orthogonal to backend and mode

A fourth orthogonal axis exists: **how a builtin acquires credentials.**

| Axis | Choices |
|---|---|
| Backend | `memory` | `temporal` |
| Mode | `static` | `dynamic` |
| Auth source | OAuth | env var | file | secrets manager | static value | workload identity | ... |
| Vendor | github | slack | weather | ... |

The current `agents.WithOAuth` behavior is one implementation of credential injection (OAuth specifically). It is forward-compatible with a generalized `agents.WithCredential[Req, Resp, Cred]` behavior parameterized over a `CredentialSource[Req, Cred] = Arrow[Req, Cred]`. Different sources are different arrows:

| Source | Cred type | Arrow body |
|---|---|---|
| OAuth | `oauth.TokenPair` | resolve identity, load from store, refresh if needed |
| Env var | `string` | `os.Getenv(name)`; error if empty |
| File | `[]byte` | read from path; error if missing |
| Secrets manager | provider-specific | call the SDK |
| Static value | `Cred` | constant arrow |

`WithOAuth` becomes a constructor on top of `WithCredential`. The OAuth-specific retry-on-401 logic moves into the OAuth `CredentialSource`'s implementation; the generic behavior just orchestrates source → inject → call.

**When this generalization happens:** not now. The trigger is the first non-OAuth agent. Specifically, when a plugin needs an env-var-based API key (e.g. WeatherAPI's `WEATHERAPI_KEY`), introducing `WithEnvCredential` as a sibling to `WithOAuth` is the right scope. The full generalization comes when there are two non-OAuth sources in use — the two-data-point rule.

**Forward compatibility:** all currently-shipping OAuth behavior continues to work unchanged. The `BuiltinSpec.AuthHint` field accommodates non-OAuth sources from day one (it's a sum type `AuthSource = OAuth | Env | File | ...`, even if only `OAuth` has an implementation initially). DSL-level auth annotations (e.g. `weather "SF" with env "WEATHERAPI_KEY"`) are not in MVP scope; the registry's declared `AuthHint` is sufficient until a use case forces overrides.

---

## 11. Sibyl convergence primitives in the DSL

Sibyl already implements three convergence-loop primitives, each at a different layer of abstraction:

| Sibyl primitive | What it does | Location |
|---|---|---|
| `ConvergeWorkflow` | Researcher/Critic loop — iterate until critic approves or `MaxRounds` hit | `agent/workflow.go` |
| `ToolAgentArrow` | ReAct loop — reason/act/observe, capped by `MaxSteps`. Already exposed as a `weft.Arrow` | `agent/tool_agent.go` |
| `SupervisorWorkflow` | Fanout of convergence loops — decompose a question, run one `ConvergeWorkflow` per subquestion in parallel, synthesize | `agent/supervisor.go` |

The DSL exposes these as native builtins:

```
temporal static (
  converge (
    question: "What is the right pricing model?"
    researcher: my_researcher
    critic: my_critic
    max_rounds: 5
  )
)
```

```
temporal static (
  tool_agent (
    task: "Find the bug in the deployment"
    tools: [github, slack, logs]
    max_steps: 20
  )
)
```

```
temporal static (
  supervisor (
    question: "Compare pricing strategies"
    max_subquestions: 8
    max_rounds_per_child: 5
  )
)
```

### Required Sibyl-side work

Two of the three primitives are not yet exposed as `weft.Arrow` values. To compose uniformly in agent pipelines:

- **`ToolAgentArrow`** — already exists. No work needed.
- **`ConvergeArrow`** — small wrapper, ~40 LoC. Wraps `workflow.ExecuteChildWorkflow(ctx, "ConvergeWorkflow", q)` in an arrow signature `Arrow[Question, Answer]`.
- **`SupervisorArrow`** — same shape, ~40 LoC. Wraps `SupervisorWorkflow` invocation.

These wrappers land in Sibyl in their own PR (see [sibyl](https://github.com/vinodhalaharvi/sibyl)) before the translator needs them.

### AgentScript-side work

Each primitive becomes a registered plugin in agentscript: a `BuiltinSpec` that points to the underlying Sibyl agent ID, plus argument shape declarations. No new translator logic — these are just builtins.

### Memory backend story

The memory backend's existing tree-walking evaluator can implement the same primitives in-process (without Temporal). For `tool_agent`, the existing ReAct loop logic ports over. For `converge` and `supervisor`, the memory backend evaluates the loop in a Go `for` loop without durability. Same DSL syntax targets both backends; the implementation differs by backend.

### What the deleted `converge` and `coordinate` keywords gave us

The legacy `converge` DSL keyword (deleted in [agentscript#3](https://github.com/vinodhalaharvi/agentscript/pull/3)) was Researcher/Critic-flavored — its replacement is the new `converge` builtin backed by Sibyl's `ConvergeWorkflow`. Functionally equivalent at the user level, with a clean implementation underneath.

The legacy `coordinate` DSL keyword had richer ambitions — blackboard patterns, multiple convergence notions (consensus, belief alignment, gossip, state equilibrium). **It does not have a direct replacement.** Sibyl's existing primitives (`Converge`, `Supervisor`, channels) compose to express most of these patterns at the script level, but there is no first-class `coordinate` primitive in Sibyl. Three positions on this:

1. **Decompose into existing primitives.** Most coordination patterns are compositions of `<*>` (fanout), reducers, `converge`, and shared state. Express them at the script level; ship convenience macros (`consensus`, `quorum`, etc.) only after they're written by hand three times.
2. **Build a real `coordinate` primitive in Sibyl** if a workload demands it. This is a separate design memo and a significant chunk of work; per §3.1, the right home for it would be Sibyl, not agentscript.
3. **Defer entirely** until a real workload needs it. The deleted code was speculation about future needs; if nobody hits the wall, it never needed to exist.

The current position is (3) — defer — with (1) as the documented escape hatch. Per §3.1, if a coordination primitive ever becomes warranted, it lives in Sibyl, and agentscript surfaces it through a builtin.

---

## 12. Public API surface

The agentscript module's exported surface should be minimal:

```go
package script

// Registry — the explicit builtin registry.
type Registry
func NewRegistry() *Registry
func (r *Registry) Register(spec BuiltinSpec) error

// BuiltinSpec — what a plugin registers.
type BuiltinSpec struct {
    Name      string
    AgentID   string
    Vendors   []string
    ArgSchema ArgSchema
    AuthHint  AuthSource
}

// Compile — the translator entry point. Backend selection happens
// per-block inside the source; Compile returns a Program that knows
// which blocks target which backend.
func Compile(src Source, opts CompileOpts) (*Program, error)

type CompileOpts struct {
    Registry  *Registry
    SibylAddr string                   // empty if no temporal blocks present
}

// Program — the compiled output.
type Program

func (p *Program) Run(ctx context.Context, in RunInput) (RunResult, error)

type RunInput struct {
    InvokedBy string    // canonical identity of the user running the script
}

type RunResult struct {
    BlockResults []BlockResult     // one per block, in source order
}
```

`Compile` is itself the composed arrow `Parse >>> Resolve >>> Lower`; `Run` is `Submit` (for temporal blocks) and direct evaluation (for memory blocks).

### CLI

```
agentscript run script.as --as=user:alice [--sibyl-addr=...]
```

- `--as` sets `InvokedBy` (required when any block is temporal and uses user-delegated OAuth)
- `--sibyl-addr` is required if the script contains any `temporal` blocks; absent if all blocks are `memory`

No `--backend` flag. The script declares its backends per block. The CLI just runs whatever `Compile` produces.

---

## 13. Cross-block data flow (deferred)

This memo treats top-level blocks as independent — each block compiles and runs in isolation. A natural follow-up question: should blocks share data? E.g.:

```
temporal static (
  fetch_data "sales-q3" >=> persist "db"
) as data

memory dynamic (
  preview data
)
```

This is **deferred**. It introduces let-binding semantics, lifetime questions across backend boundaries, and serialization concerns. The MVP does not need it. Most useful scripts will be one block. When cross-block data flow becomes necessary, the design should be revisited as a separate memo.

---

## 14. Minimum-viable slice

What gets built first, to prove the architecture end-to-end:

1. **Parse, Resolve, Lower (Sibyl backend), Finalize, Validate, Submit** — all six arrows, but covering only:
   - `Call` and `Pipeline` AST nodes (no `Parallel` yet)
   - `temporal static` blocks only (no `memory`, no `dynamic`)
   - One builtin (e.g. a no-vendor `echo` agent — so OAuth wiring isn't a prerequisite for the first demo)
2. **CLI**: `agentscript run` for the above
3. **One end-to-end demo script**: a two-stage pipeline that compiles, submits, and shows in Temporal's UI

This is the smallest set that demonstrates the lineage from source to durable execution. Subsequent PRs add: `Parallel`, the memory backend, the `dynamic` mode, the synthesized `join` node, more builtins, OAuth-wired plugins, DSL-level auth annotations.

Each addition is an isolated change because the abstractions are already in place. Adding `Parallel` is adding `lowerParallel` and `fanout` — one arrow and one combinator. Adding the memory backend is adding `LowerToMemory` and the `memory.Program` type. Adding `dynamic` is adding `lowerDynamicBlock` which produces a single-node fragment wrapping the existing memory evaluator. Adding OAuth-wired plugins is adding `WithOAuth` to the plugin's agent registration — no translator changes.

The architecture compounds. Each piece earns its weight; each piece opens space for the next.

---

## 15. Open questions

1. **Argument flow between stages of `>=>`.** Currently the DSL is surface-untyped. Type inference at the join is doable but non-trivial. Initial implementation should emit runtime type errors; structural inference can come as a follow-up.
2. **Synthesized join node behavior.** Should `(a <*> b) >=> c` pass `c` a tuple `(A, B)` or two separate args? Tuple is simpler; two-args is more ergonomic but requires `c` to be variadic-aware. Lean: tuple. Confirmable post-MVP.
3. **Cycle detection in `Validate`.** Standard topological sort; not novel. Just needs to be in the spec.
4. **Error reporting format.** Parse errors should include file/line/col from participle. Resolution and lowering errors should reference AST source positions, which means the AST should carry source positions through. Worth specifying before implementation; left as TODO for the first PR.
5. **Plugin packaging.** One module or multi-module for the agentscript plugins? Cleaner answer once the first real plugin lands. Defer.

---

## 16. Summary

The translator is a composition of typed arrows: `Parse >>> Resolve >>> Lower >>> Finalize >>> Validate >>> Submit`. Each phase is replaceable, testable in isolation, and composable with the same `weft` middleware that wraps Sibyl agents. The AST is backend-agnostic. The two backends — temporal (durable, production) and memory (fast, local) — diverge at `Lower` and converge at "something executable." Block-level annotations (`temporal`/`memory`, `static`/`dynamic`) are orthogonal and explicit, never defaulted. Authentication is a fourth orthogonal axis, currently implemented for OAuth, forward-compatible with env vars, files, secrets managers, and other sources. Sibyl's existing convergence primitives are exposed as DSL builtins; new convergence/coordination behavior, if needed, goes in Sibyl per the load-bearing rule in §3.1.

The DSL is presentation; Sibyl is execution. The translation is value-to-value, arrow-by-arrow. Adding language features adds arrows; adding execution features adds backends; adding credential types adds sources. Each axis evolves independently.
