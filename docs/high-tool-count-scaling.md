# Scaling to High MCP Tool Counts

> **Status:** S7a (in-band supervisor) **implemented**, Layers 0–6. **S6b**
> (read-only footprint reduction), the **Layer 6** routing-quality eval harness,
> and **S8** (intra-group tool-level selection for oversized servers) are now
> **implemented** (see §S6b, §S8, §4.2 Layer 6). Deferred: S3 and S7b. Original
> design retained below for context; §4.3 records the decisions locked in during
> the build.
> **Audience:** Engineers working on netapp-chat-service
> **Scope:** Strategies for keeping the per-request tool list small, relevant,
> and under the provider cap as the number of connected MCP servers grows. All
> strategies are evaluated against the service's standing constraint: stay
> **generic and host-agnostic**, default to today's behavior, and require no
> changes in consuming apps unless an app explicitly opts in.

---

## 1. Problem

The agent flattens the tool lists of every connected MCP server into a single
array and sends it to the LLM on every turn (`Router.Tools()` →
`Agent.filteredTools()` → provider). Two pressures grow with each new server:

1. **Hard provider cap.** OpenAI (and the proxy in front of it) reject requests
   with more than **128** tools. `MaxToolsPerRequest = 128` in
   [`agent/agent.go`](../agent/agent.go) enforces this and currently surfaces it
   as `ErrTooManyTools` — the chat simply fails when too many capabilities are
   enabled.
2. **Soft degradation well below the cap.** Every tool schema is injected into
   the context window. At 40–60+ tools, selection accuracy drops (the model
   picks the wrong tool or hallucinates arguments) and per-turn token cost and
   latency climb, regardless of the hard limit.

A real deployment connecting Jira (~50 tools), Bitbucket (~35), Confluence,
PUAT, Zoom, and a strategy server is already at or past the cap before any
product-specific servers are added.

### What already mitigates this today

- **Capability gating** ([`capability/`](../capability/capability.go)): each
  MCP server maps 1:1 to a capability with state `off | ask | ask-on-write |
  allow`. `off` removes a whole server's tools from the list.
- **Mode-based write filtering** (`filteredTools()`): in `read-only` mode, write
  tools are *dropped from the list entirely* (not merely denied at call time),
  so read-only already reduces count, not just permissions.
- **Per-server `read_only_tools` allowlist** (`ServerConfig.ReadOnlyTools`):
  classifies tools the MCP doesn't annotate, feeding the mode filter above.

These are **whole-server, static** levers. They are not enough when a *single*
enabled server (Jira, Bitbucket) exceeds the comfortable budget on its own, or
when a session legitimately needs several servers at once.

### What "intent detection" already exists (and what it costs)

It is easy to assume the service already pays for an intent-classification LLM
call. It does not. Today's intent detection is two pieces, **neither of which is
a dedicated classifier round trip**:

1. **Deterministic match** — [`Catalog.Match()`](../interest/catalog.go), called
   from [`server/server.go`](../server/server.go), does substring/trigger
   matching over interest phrases in **Go**. Zero LLM.
2. **In-context self-selection** — [`Catalog.BuildIndex()`](../interest/catalog.go)
   injects a compact trigger table into the **main** agent's system prompt
   (`BuildSystemPrompt`). The main model reads it and, when a trigger matches,
   calls `get_interest(id)` as its first tool call **inside the normal agent
   loop**.

So the only extra hop is the `get_interest` tool call, which happens *within*
the existing tool-use loop (a search-then-load shape) — **not** a
separate supervisor model with its own request. This is the key fact for S7
below: a "supervisor" need **not** add a dedicated round trip. The interest
subsystem is a working precedent for a **generic, in-band, registry-driven
selector** — it builds a menu from a registry (interests declare their own
`triggers`/`requires`), filters that menu by currently-enabled capabilities at
query time (`enabled map[string]bool`, so it self-updates as servers
connect/disconnect), and lets the main model self-select with no tier-1 call.

### The "tool auto-registry" that keeps a supervisor generic

A supervisor/router only stays host-agnostic if its menu is **auto-derived**, not
hand-authored. The building block already exists: capabilities map **1:1 to MCP
servers** ([`capability/`](../capability/capability.go)). Capability *groups* can
therefore be auto-derived from the connected server list (and refined by operator
config) — never from a baked-in product/view map. The registry rebuilds as
servers connect/disconnect, exactly like interests filter by `enabled` today.

The single rule that preserves genericness: **group labels come from server/tool
metadata + operator config, never a hardcoded "view → tools" map** (the S3
guardrail). Whichever way selection happens — the in-band supervisor (S7a) or a
dedicated router model (S7b) — it reads this registry and stays generic because
the registry itself carries no host semantics.

---

## 2. Design constraints (the genericness bar)

Every strategy below is held to the same bar the rest of this service meets
(see [`mcp-request-header-forwarding.md`](./mcp-request-header-forwarding.md) §2):

- **Content/config-driven, not host-semantics-driven.** Tool selection may use
  the tool's own name/description/annotations and operator config. It must not
  encode knowledge of any specific product, view, or domain.
- **Backward compatible by default.** Absent configuration, behavior is
  byte-for-byte identical to today. A consuming app that changes nothing sees no
  change.
- **Opt-in for anything requiring host input.** A strategy that needs the host
  to send extra signal must degrade cleanly to today's behavior when the host
  sends nothing.

A strategy that violates the first bullet (e.g. a hardcoded "roadmap view →
jira only" map) does not belong in this service; it belongs in the host or in a
façade MCP.

---

## 3. Strategy catalog

This plan builds **three** strategies: **S3** (opt-in context hints), **S6**
(read-only reduction, already implemented), and **S7** (the supervisor we are
building). Two further strategies were added **after S7a shipped**, to address
the group-granularity ceiling it leaves behind (a single oversized server):
**S6b** (read-only footprint reduction — **now implemented**) and **S8**
(intra-group tool-level selection — **now implemented**). The strategy IDs
are stable; the gaps below are alternatives we **evaluated and will not build**:

- **S1 — per-turn relevance ranker ("tool RAG").** A Go/embedding ranker that
  trims tools per turn. Rejected: a recall cliff and per-turn non-determinism for
  a job S7a does better with the main model and no separate ranker.
- **S2 — `list_tools`/`load_tools` search meta-tools.** Rejected as a standalone
  selector; its `load_tools` mechanism is **absorbed into S7a**, so the useful
  part survives without a second selector.
- **S4 — façade / aggregating MCP** (collapse e.g. Jira's 50 tools into one
  `action`-enum tool). Rejected: encodes server-specific tool semantics in a
  bespoke per-deployment shim that erodes the service's generic posture.
- **S5 — per-server `tools:` expose allowlist.** Rejected: forces operators to
  enumerate a server's tool names in config; pulls server-specific knowledge into
  the deployment surface and goes stale as servers evolve.

The remaining sections detail only what we are building.

### S3 — Context-scoped capability hints

> **Status: ⏳ Not implemented (deferred, opt-in when an app asks).**

**What.** Let the host narrow the active capability set per request based on what
the user is doing (e.g. a PR view sends "bitbucket-ish" context; an analytics
view sends "puat-ish"). The service maps the hint to capabilities and filters.

**Where.** Read an inbound header (same mechanism as `forward_headers`) in
[`server/server.go`](../server/server.go), translate to a capability subset
before `filteredTools()`.

**Downsides.**
- **This is the one strategy that pressures genericness.** To be useful, the
  host must supply context — that's new coupling, and the mapping risks importing
  product semantics into the service.

**Genericness / app impact.** **Opt-in only.** Keep it a generic, opaque
capability allowlist supplied by the host (the service assigns no meaning to the
label, exactly like `forward_headers` treats values as opaque). Apps that send
nothing are unaffected. Apps that want it **must change** to send the header.
Do **not** bake any view→capability map into this repo.

### S6 — Read-only-as-count-reduction (already implemented)

> **Status: ✅ Implemented (pre-existing).**

`filteredTools()` already drops write tools from the list in `read-only` mode.
This is a real count reduction, not just a permission gate. **No work required**;
documented here so it isn't reinvented. It only helps in proportion to how many
write tools a server exposes.

#### S6b — shrink a group's read-only footprint (lowest-hanging follow-up)

> **Status: ✅ Implemented** — the budget/headroom benefit was already automatic
> via `computeToolBudget()` + `filteredTools()`; the routing **menu** is now
> mode-aware too (`server.buildRoutingGroups`, `server/server_test.go`
> `TestRoutingMenuRespectsReadOnlyMode`). Remaining work is per-MCP annotation,
> which lives in the MCP servers, not this service.

The per-turn cost of a large server is dominated by its **write** tools, and the
budget unit in S7a is the whole server (see §S8). Two near-zero-code levers cut a
group's footprint on read-only turns:

1. **Annotate `ReadOnlyHint` (or supply `read_only_tools`)** on each MCP so
   `filteredTools()` can drop writes. `ontap-mcp` already publishes
   `ReadOnlyHint`; `harvest-mcp` uses the `read_only_tools` allowlist label.
   Jira/Confluence read-only annotations are landing, so this benefit is now
   realized for those servers automatically.
2. **Lean toward a read-only default** for read-heavy products.

For an `ontap` that is ~80 tools total but, say, ~45 read-only, a read-only turn
loading `ontap` drops from 80 → ~45 — often enough to let `ontap` pair with
another group under `MaxToolsPerRequest` **with no new routing code at all**.
This is the cheapest mitigation for the oversized-server problem and should be
exhausted first. It does **not** help inherently-write turns (e.g. volume
provisioning), which is exactly where S8 (below) earns its keep.

> **Service-side change (done):** the routing group menu in `server.RunChat` is
> now built from the *mode-filtered* tool set (`buildRoutingGroups`). Previously
> the menu was derived from `Router.Tools()` (all tools), so in read-only mode it
> still advertised write tools the model could not call and a read-only server
> did not show its reduced footprint. The menu now drops writes in read-only mode
> exactly as `filteredTools()` does (ask-on-write capabilities still surface
> writes), so an annotated read-only server presents a smaller, accurate group.

> NABox note: `ontap` currently defaults to `ask-on-write`, which surfaces its
> write tools to the model (and the budget) regardless of mode. A product that
> wants the read-only footprint reduction must run the relevant turns in
> `read-only` mode (or set `ontap` read-only), not `ask-on-write`.

### S7 — Router / dispatcher agent (supervisor; intent-based selection)

> **Status: S7a ✅ Implemented (Layers 0–5, see §5). S7b ⏳ Not implemented
> (config-time mode, rejected at startup until built).**

**What.** A supervisor that selects which capability *groups* are active for a
turn, so the worker loop only loads the selected groups' tools. Crucially, the
selection step has **two implementation tiers with very different costs** — do
not assume a supervisor means an extra LLM call:

- **S7a — in-band supervisor (no dedicated round trip).** Inject a compact
  capability/group index into the **main** system prompt (exactly how the
  interest index works today) and expose an internal `load_tools(group)` tool.
  The main model self-selects and loads groups within the normal loop. Cost: the
  same in-loop tool round trip the `get_interest` pattern already pays — **no
  separate classifier model.** This is the recommended default supervisor: it
  reuses a proven, generic mechanism already in the codebase.
- **S7b — dedicated router model (one extra round trip).** A lightweight tier-1
  LLM call that sees **no tools** — only the auto-derived group menu — and
  returns the relevant groups; the worker then runs with only those. Worth the
  extra call only when you want selection to cost **zero** main-context tokens
  and zero tool exposure (e.g. very large group menus, or strict separation of
  routing from execution).

Both tiers read the **auto-registry** (see §1) so they stay generic, and both let
the worker call back (`request_more_capabilities` / another `load_tools`) when it
discovers mid-task that it needs another group — closing the recall-miss risk with a
model in the loop rather than a fixed ranker threshold.

**Where.** S7a: a `load_tools` internal tool (`Agent.InternalTools`, pattern from
[`interest/tool.go`](../interest/tool.go)) plus a group index in
`BuildSystemPrompt`, both backed by the capability registry. S7b: a pre-pass call
on the existing `llm.Provider` in front of the worker loop in
[`agent/agent.go`](../agent/agent.go), whose result feeds the capability filter
`filteredTools()` already applies.

**Downsides.**
- **S7b adds an LLM round trip** per turn (or per intent shift) — higher latency
  and cost. **S7a does not** (it reuses the existing in-loop tool hop).
- **Router misclassification** propagates — if the supervisor picks the wrong
  group, the worker can't recover without the `request_more_capabilities` /
  re-`load_tools` callback.
- More orchestration and state to test than a static, always-on tool list.

**S7a vs S7b.** Same selector over the *same* auto-registry, differing only in
where selection happens: **S7a** self-selects in-band (no dedicated call); **S7b**
adds a dedicated routing *model*. Pick one as primary — they are substitutes. S7a
is the sweet spot when you want model-quality intent detection without paying for
a second model call.

**Is S7a “smart enough” to route as well as a dedicated router (S7b)?** The risk
is **not** intelligence — it is routing *reliability*. In S7a the **main** agent
model makes the selection; in S7b you typically use a **cheaper/lighter** tier-1
model to justify the extra call. So S7a routes with the *more* capable model and
will not be “too dumb” to pick the right group. Its two genuine weaknesses are:
- **Divided attention.** The routing decision shares context with the task,
  history, and every other system-prompt instruction. A dedicated router (S7b)
  does one job with one instruction, so its decision can’t be crowded out.
- **Compliance / skipping.** S7a relies on the model calling `load_tools`
  *before* answering. This is the exact risk the existing interest code already
  fights — see the `**CRITICAL** … you MUST call get_interest(id) as your very
  first tool call … Do NOT skip this step` nag in `BuildSystemPrompt`. That nag
  exists *because* in-band self-selection sometimes skips. S7b cannot skip —
  routing is a separate, mandatory hop.

Mitigations that close most of the gap (and keep S7a the right first build):
- A **forced-first-step contract** (reuse the proven interest nag), optionally
  *enforced* in the agent loop — reject a final answer until at least one group
  is loaded.
- Keep the group menu **small (~6–10 labels)** so the decision is trivially
  separable from the task.
- Let the worker **re-`load_tools` mid-task** to recover from a thin initial pick.
- Treat measured **skip/misroute rates in telemetry** as the graduation signal:
  if they’re material, flip the config to S7b. Until then S7a is strictly
  cheaper and uses the better model.

**Genericness / app impact.** **Safe, no app changes** — provided the group menu
is auto-derived from the capability registry + operator config (§1), not from
host semantics. Default (supervisor disabled) → identical to today; apps opt in
by operator config (e.g. `tool_routing: { mode: in-band | router | off }`).

### S8 — Intra-group (tool-level) selection for oversized groups

> **Status: ✅ Implemented** — `capability/group.go` (`BuildGroupsExpanding`,
> expandable `RenderGroupIndex`), `agent/agent.go` (`load_tools` `tools` field,
> tool-level activation in `filteredTools`, telemetry `ToolsLoaded`),
> `config/config.go` (`tool_routing.group_expand_threshold`), `server/server.go`
> (`buildRoutingGroups` threshold), `cmd/chat-service/main.go` wiring. Disabled
> by default (`group_expand_threshold: 0` → pure group-level S7a). Tests:
> `capability/group_test.go`, `agent/routing_test.go`, `config/config_test.go`,
> `server/server_test.go`, `eval/`.

**Why.** S7a routes at **group (= MCP server) granularity**: `load_tools(group)`
activates *all* of that server's tools. This is correct and generic, but it has
no lever when a **single server is itself large**, because the budget unit is the
whole server. Concretely (NABox): `ontap-mcp` exposes ~80 tools. A turn that
loads `ontap` alone (~80 + ~8 internal ≈ 88) fits, but a turn that legitimately
needs `ontap` **and** `harvest` (~80 + ~44 = 124 MCP + internal ≈ 132) exceeds
`MaxToolsPerRequest` *even with routing on*. Group-level routing cannot split one
server, so this is the ceiling S7a leaves behind.

**What.** Extend the in-band selector to optionally operate at **tool
granularity for groups that exceed a size threshold**, keeping whole-group
loading for small servers (a **hybrid**). The proven `load_tools` mechanism is
reused; only its grain changes:

- **Small groups** (≤ `group_expand_threshold`, e.g. 25) behave exactly as today:
  one menu row, load the whole server.
- **Oversized groups** are rendered as an **expandable sub-index** of
  `tool_name — one-line description` rows (descriptions auto-derived from the
  tool's own MCP metadata — the same content-derived rule used for group
  descriptions). `load_tools` accepts individual tool names for these, so the
  model pulls in only the handful it needs (e.g. 8–12 of `ontap`'s 80).

**Contract.** `load_tools` gains an additive `tools: []string` field alongside
`groups: []string`. Loading stays additive and idempotent, and the mid-task
re-`load_tools` recovery path is unchanged. `groups` remains valid (and is the
only thing offered for small servers), so the change is **backward compatible**
with the S7a contract. *(As built: at least one of `groups`/`tools` is required;
an individually-loaded tool activates only that tool, and its owning group is
reported in `RoutingStats.GroupsLoaded` while the tool itself appears in the new
`RoutingStats.ToolsLoaded`. Tool-level loads also satisfy the forced-first-step
nudge.)*

**Where.** [`capability/group.go`](../capability/group.go) (emit a per-tool
sub-index for oversized groups), `BuildSystemPromptWithRouting` (render the
nested rows under a collapsible group heading), and `Agent.handleLoadTools` /
`filteredTools()` (activate individual tools, not just whole groups). The budget
assertion (`≤ MaxToolsPerRequest`) is unchanged — it simply sees a smaller routed
set.

**Downsides.**
- **Prompt menu grows** for expanded groups (one row per tool, not per server).
  Bounded by only expanding oversized servers and by listing `name + 1-line
  desc`, not full schemas. At extreme total tool counts this trends toward the
  S1 "tool RAG" regime, which remains the right tool for thousands-of-tools
  deployments.
- **Selection recall risk on names/descriptions** — the model picks tools from
  one-liners, not full schemas (the same risk S1 carried). Here it is (a) limited
  to oversized servers, (b) made by the *main* model, and (c) recoverable via the
  existing mid-task re-`load_tools` callback, so there is **no fixed-threshold
  recall cliff**.

**Why this is not the rejected S1/S5.** S8 is **model-driven** (no embedding
ranker, no recall cliff — re-load recovers) and **auto-derived** (no operator
tool enumeration). It is the natural extension of S7a's proven in-band mechanism
to a finer grain, triggered *only* when a group is too big to load wholesale.
That is precisely what S1 (ranker) and S5 (operator allowlist) gave up.

**Genericness / app impact.** **Safe, no app changes.** Tool names/descriptions
are content-derived; no host semantics. The default threshold leaves
small-server deployments behaving exactly like today's group-level S7a.

---

## 4. Recommended sequencing

### 4.1 Chosen direction

**Build S7a (in-band supervisor) as the primary selector.** It delivers
model-quality intent detection using the *main* (most capable) model, adds **no
dedicated round trip**, and reuses a mechanism already proven in the codebase
(the interest index + `get_interest` self-selection). S7b is **not a separate
build** — it is kept as a **config-time mode** (`tool_routing: { mode: in-band |
router }`) reading the same auto-registry and feeding the same `filteredTools()`
filter, to be enabled later only if telemetry shows S7a skip/misroute rates are
material.

Around that primary selector:

Around that primary selector:

- **S6 (read-only count reduction) — IN USE.** Already implemented and generic;
  no work, keep relying on it.
- **S3 (context hints) — OPT-IN, allowed.** Acceptable *only* as an opaque,
  host-supplied capability allowlist (modeled on `forward_headers`: header name
  is config, value/label is meaningless to this service). Apps that send nothing
  are unaffected. Never bake a view→tools map into this repo.

S1, S2, S4, and S5 are **not built** (see §3, rejected alternatives). The only
part that survives is S2's `load_tools` mechanism, reused as a building block
*inside* S7a (the supervisor's internal `load_tools` tool plus the mid-task
re-`load_tools` recovery callback).

### 4.2 Order of work

1. ✅ **Primary build (done):** implement **S7a** (see §5 for the layered plan).
2. ✅ **In use, no work:** **S6** read-only reduction stays as-is.
3. ✅ **Lowest-hanging follow-up (done):** **S6b** shrink big servers' read-only
   footprint via `ReadOnlyHint` / `read_only_tools` annotations + read-only-
   leaning defaults. The service now also builds the routing menu from the
   mode-filtered tool set so read-only servers present their reduced footprint
   (`server.buildRoutingGroups`). Remaining per-MCP annotation work lives in the
   MCP servers (Jira/Confluence landing now). Try this **before** S8 — it may
   relieve the oversized-server pressure with no further routing changes.
4. ✅ **Intra-group tool-level selection (done):** **S8** for oversized servers
   (the `ontap` ~80 case), with a hybrid size threshold
   (`tool_routing.group_expand_threshold`) so small servers keep loading
   wholesale. Disabled by default; set the threshold to enable per-tool loading
   from large groups.
5. ⏳ **Opt-in, when an app asks (not built):** **S3** context hints as an opaque
   capability allowlist.
6. ⏳ **Later, only if telemetry warrants (not built):** **S7b** behind the existing
   `tool_routing.mode: router` flag — no rearchitecture, a second selector path
   on the same registry.

### 4.3 Decisions locked in during implementation

These resolve the open questions raised before the build. They are grounded in
the two adjacent consumers (RTB-Platform/CADENCE — `static` discovery,
`puat`/`zoom`/`strategy`, the cap-pressure case; NABox — `docker` discovery,
`harvest`/`ontap`/`grafana`, interest-heavy), both of which already map each MCP
server 1:1 to a capability.

1. **Scope = S7a, Layers 0–5.** S3 (context hints) and S7b (router model) are
   deferred; S7b stays a config-time mode that fails fast at startup until built.
   *(Update: the Layer 6 routing-quality eval and S6b read-only footprint
   reduction have since been implemented as follow-ups — see §S6b and §4.2
   Layer 6.)*
2. **1:1 server = capability = group; no operator group-merging.** Every MCP
   server is its own group, toggled on/off globally. Users get only the
   read/read-write toggle; there is no per-tool user control and no
   merge-servers-into-named-group config. The group menu *is* the capability
   list. (Supersedes Layer 1's "operator config may merge servers" option.)
3. **Loaded-group state is per user message, not sticky.** The model re-selects
   each message (mirrors `get_interest`); state lives on the per-message `Agent`
   and resets each `Run`.
4. **Group descriptions: config + auto-derive fallback.** Optional
   `capability_name` / `capability_description` per server; when a description is
   absent it is auto-derived from the server's own tool names. No host semantics.
5. **In-band routing coexists with the interest pre-match.** The deterministic
   `Catalog.Match()` capability narrowing in `server.go` still runs; the group
   menu is built from the *post*-pre-match enabled set, so the two compose rather
   than one superseding the other.
6. **Forced-first-step enforcement ships on by default for in-band.** Prompt nag
   **plus** loop enforcement: one corrective nudge if the model answers
   tool-lessly before loading any group, then a graceful fallback. Always-on
   groups count as active, so a configured baseline never trips the nudge.

---

## 5. Implementation plan — S7a (in-band supervisor)

> **Status: ✅ Implemented — Layers 0–5 built, tested, and wired. Layer 6
> (routing-quality eval) ✅ now implemented as an opt-in/non-CI harness
> (`eval/`).** Each layer below carries its own status marker and an
> *As built* note.

This is the build plan for the chosen primary selector. It is layered so each
layer lands independently, behind config, with its own tests, and with
**byte-for-byte identical behavior when `tool_routing.mode` is unset/`off`**.

Guiding principle for testing: **test each layer at the lowest level it can be
tested deterministically.** The registry and filter layers are pure Go and get
unit tests; the system-prompt layer is a golden-string test; the agent-loop
behavior is tested with the existing `MockRouter` / mock provider; only the
end-to-end intent quality needs an LLM and is gated as a separate eval suite.

### Layer 0 — Config surface (`config/`)

> **Status: ✅ Implemented** — `config/config.go`, `config/config_test.go`.

**Build.** Add `tool_routing` to the config schema:

```yaml
tool_routing:
  mode: off            # off (default) | in-band | router
  max_tools: 0         # 0 = no cap beyond MaxToolsPerRequest
  always_on: []        # capability/group IDs always loaded
```

Default `mode: off` ⇒ today's behavior. `router` is parsed and validated as a
legal mode now but **fails fast at startup wiring** (`cmd/chat-service` exits
with a clear "not implemented" message) until S7b.

*As built:* `config.ToolRoutingConfig` with `normalize()` validation; mode
constants (`agent.ToolRoutingOff/InBand/Router`) are the single source of truth.
Two optional per-server fields feed the group menu's labels/descriptions:
`capability_name` and `capability_description` (decision §4.3-4).

**Tests (`config/config_test.go`).**
- Unmarshal with the field absent → `mode: off`, no error (back-compat).
- Each valid `mode` parses; an invalid mode errors with a clear message.
- `router` parses but is flagged unimplemented at startup wiring (not a config
  error).
- Round-trip marshal/unmarshal stability.

### Layer 1 — Capability/group auto-registry (`capability/`)

> **Status: ✅ Implemented** — `capability/group.go`, `capability/group_test.go`.

**Build.** A pure function that derives the group menu from the **connected
servers + capability states** (the §1 auto-registry), with no host semantics.
Grouping is **strictly 1:1 with MCP servers / capabilities** (decision §4.3-2):
operator group-merging was evaluated and **not built** — every MCP server is its
own group, toggled on/off globally exactly like a capability today. Output: an
ordered list of `{ id, label, description, toolNames[] }`, filtered to
currently-enabled capabilities at call time (mirrors `Catalog.BuildIndex(enabled)`).

*As built:* `capability.BuildGroups(caps, enabled, toolsByCap)` →
`[]capability.Group` (ordered by capability ID), plus `RenderGroupIndex` for the
prompt table. `Label` is the capability name (falling back to the ID);
`Description` is the operator-configured `capability_description` when set, else
auto-derived from the server's own tool names (decision §4.3-4). Lives in
`capability/group.go`.

**Tests (`capability/group_test.go`) — implemented:**
- Group derivation from a fixed server/capability set → expected groups (pure,
  table-driven), ordered by ID.
- `off`/absent capability omitted from the menu; re-enabling re-adds it.
- Server connect/disconnect changes the menu deterministically (no restart).
- Auto-derived description truncates for fan-out servers; empty toolset →
  placeholder description.
- **No host semantics leak:** a property-style assertion that group IDs/labels
  derive only from capability IDs/names + server-supplied tool names.

(The originally-planned "operator-config merge of two servers into one group"
test was dropped — group-merging is **not built**, per decision §4.3-2.)

### Layer 2 — System-prompt group index (`agent/BuildSystemPrompt`)

> **Status: ✅ Implemented** — `agent/agent.go` (`BuildSystemPromptWithRouting`),
> `agent/agent_test.go`.

**Build.** When `mode: in-band`, emit a compact group index (same shape as the
interest index) plus the forced-first-step contract: the model MUST call
`load_tools(group)` before answering when a group is relevant. Reuse the proven
`**CRITICAL** … very first tool call … Do NOT skip` phrasing already validated by
the interest path.

*As built:* added `BuildSystemPromptWithRouting(cfg, router, interestIndex,
groupIndex, canvasTabs…)`; the original `BuildSystemPrompt` is preserved and
delegates with an empty `groupIndex`, so all existing call sites are untouched and
`mode: off` is byte-identical. The group index coexists with the interest index
(decision §4.3-5).

**Tests (`agent/agent_test.go`, golden string).**
- `mode: off` → prompt contains **no** group index (byte-identical to today).
- `mode: in-band` with N groups → index lists exactly those groups, in order,
  with the forced-first-step contract present.
- Empty registry (no enabled groups) → no index block, no dangling header.

### Layer 3 — `load_tools` internal tool (`agent/`, pattern from `interest/tool.go`)

> **Status: ✅ Implemented** — `agent/agent.go`, `agent/routing_test.go`.

**Build.** Register `load_tools(groups: []string)` as an internal tool (reuse
`Agent.InternalTools` / `appendInternalTools`). Its handler marks the named
groups active **for the current user message** (decision §4.3-3: per-message,
not sticky across turns — the model re-selects each message, mirroring
`get_interest`); subsequent `filteredTools()` calls include those groups' tools.
Unknown group → structured, recoverable result (returned as a normal tool result
the model can read and retry, not a hard error). Idempotent; repeated calls
union the active set (enables mid-task re-`load_tools` recovery).

*As built:* state lives on the per-message `Agent` (`loadedGroups` under a mutex,
since tool calls run in parallel); the tool is registered in `New()` after all
options are applied so option order can't clobber it. State is reset at the start
of each `Run`.

**Tests (`agent/*_test.go`, `MockRouter`).**
- Calling `load_tools(["jira"])` activates exactly the jira group's tools on the
  next `filteredTools()`.
- Unknown group → error result, no panic, set unchanged.
- Idempotent union: `load_tools(["jira"])` then `["bitbucket"]` → both active.
- `always_on` groups are active from turn 1 without a call.
- Interaction with capability `off` and read-only mode: a loaded group still
  honors mode/capability filtering (write tools stay filtered in read-only).

### Layer 4 — Agent-loop enforcement & budget (`agent/filteredTools`)

> **Status: ✅ Implemented** — `agent/agent.go`, `agent/routing_test.go`.

**Build.** With `mode: in-band`, `filteredTools()` returns only
`always_on ∪ loaded` groups (subject to existing capability/mode filters). Two
guardrails:
1. **Forced-first-step enforcement (optional, recommended):** if no group is
   loaded and the model attempts a final answer, inject a corrective nudge
   instead (one retry) before allowing a tool-less answer.
2. **Budget:** the post-routing set is asserted `≤ MaxToolsPerRequest` (and
   `≤ max_tools` when set); if a single loaded group still exceeds it, surface a
   precise error naming the group (the signal that one server's fan-out is
   irreducible and needs an operator-side fix).

*As built:* `filteredTools()` recomputes at the top of each in-band iteration so
a `load_tools` call mid-loop takes effect on the next turn; for `mode: off` the
tool list is computed once and never recomputed (byte-identical). Forced-first-
step enforcement ships **on by default for in-band** (decision §4.3-6): one
corrective nudge, then a graceful tool-less answer is allowed.

**Tests (`agent/agent_test.go`).**
- `mode: off` → `filteredTools()` identical to today (regression guard).
- `mode: in-band`, nothing loaded → only `always_on` (or empty) returned.
- After `load_tools`, the merged set is exactly the expected union and within
  budget.
- Over-budget single group → `ErrTooManyTools` (or successor) naming the group.
- Forced-first-step: simulated skip → one corrective retry, then graceful
  fallback.

### Layer 5 — Telemetry (graduation signal for S7b)

> **Status: ✅ Implemented** — `agent/agent.go` (`RoutingStats`), `agent/routing_test.go`.

**Build.** Emit structured logs/metrics per turn: groups offered, groups loaded,
whether a load happened before the first substantive answer (skip detection),
and mid-task re-loads. These are the numbers that later justify flipping
`mode: router` (S7b).

*As built:* `agent.RoutingStats` (mode, groups offered, groups loaded, load
calls, reloads, skipped, compliant) is accumulated during `Run`, logged once at
turn end (`"tool routing summary"`), and exposed via `(*Agent).LastRoutingStats()`
for deterministic assertions in tests. Skip is a heuristic: a tool-less final
answer with zero load calls and no `always_on` baseline.

**Tests.**
- A skip (answer before `load_tools`) is recorded as a skip.
- A correct first-call load is recorded as compliant.
- Counters are stable under the mock provider (no flakiness).

### Layer 6 — End-to-end intent-quality eval (LLM, separate suite)

> **Status: ✅ Implemented (harness + seed fixtures)** — `eval/routing_eval.go`,
> `eval/scenarios.go`, `eval/routing_eval_test.go`. The live-provider run is
> opt-in / non-CI by design (gated behind `CHAT_EVAL_*` env vars).

**Build.** A small, curated fixture set of representative user turns mapped to
the group(s) they *should* load (`eval.DefaultScenarios` — a fixed
jira/confluence/bitbucket/harvest(ONTAP)/zoom environment). `eval.RunScenario`
spins up the real agent loop with in-band routing over a `MockRouter`
environment, runs one turn against the supplied `llm.Provider`, and scores the
groups the model loaded (`RoutingStats.GroupsLoaded`) against the expected set.
`eval.RunSuite` aggregates into `Top1Accuracy` (recall == 1.0 per turn),
`ExactAccuracy` (no over/under-load), and `SkipRate` — the empirical basis for
the S7a→S7b decision. The harness performs no network I/O itself; pass a real
provider for an accuracy run or a `MockProvider` for deterministic self-tests.

**Run the live eval (opt-in, non-CI):**

```bash
CHAT_EVAL_PROVIDER=openai \
CHAT_EVAL_ENDPOINT=... CHAT_EVAL_MODEL=gpt-4.1 CHAT_EVAL_API_KEY=... \
go test ./eval/ -run TestRealProviderEval -v
```

Without the `CHAT_EVAL_*` vars the live test skips, so CI stays hermetic.

**Tests.**
- Deterministic mock-provider tests pin the harness scoring (exact hit, misroute,
  over-load = hit-not-exact, multi-group, skip, correct-skip, suite aggregation,
  fixture well-formedness) — these run in CI.
- `TestRealProviderEval` produces the fixture-driven accuracy report and asserts
  a permissive top-1 floor (advisory, not a hard gate); skipped unless enabled.

### Definition of done

> **Status: ✅ Met for S7a.** Plus end-to-end wiring (`server/RunChat` builds the
> menu post interest pre-match and passes `WithToolRouting`; `cmd/chat-service`
> populates `tool_routing` and rejects `router`) with `server/server_test.go`
> coverage. Full suite green, including `go test -race ./agent/`.

- ✅ `mode: off` is byte-for-byte identical to today (proven by golden + filter
  regression tests).
- ✅ Layers 0–5 are deterministic and covered without an LLM.
- ✅ `in-band` keeps any single deployment within `MaxToolsPerRequest` for its
  configured groups, or fails with a precise, actionable error.
- ✅ Telemetry exposes skip/misroute rates so S7b can be enabled by config alone.

---

## 6. Direct answers to the recurring questions

**"Is there a downside to any of these? Should we just do them all now?"**
No — don't do all at once. The plan deliberately builds **one** durable selector
(**S7a**) rather than stacking competing ones. **S7a**'s cost is orchestration
and state to test, plus a routing-reliability risk (misclassification), which the
mid-task `load_tools` recovery callback bounds to a self-correcting extra hop —
not silent capability loss. **S7b** would add a dedicated round trip per turn, so
it stays behind a config flag until telemetry justifies it. **S3** adds host
coupling, so it is opt-in only. **S6** is free but only helps proportional to
write-tool count. The chosen plan keeps the generic core small: **S7a** is the
durable build, **S6** is free and already in use, and **S3** is opt-in.

**"netapp-chat-service is generic and used by many apps — do these risk that, and
will apps need changes?"**
Held to the bar in §2, **S6 and S7 keep the service generic and require no app
changes** because they are content/config-driven and default to today's behavior.
**S3 is the only strategy that pressures genericness**: it needs the host to send
context. Keep it strictly opt-in and opaque (model it on `forward_headers` —
header name is config, value/label is meaningless to this service). Apps that opt
in must change to send the signal; every other app is unaffected.

---

## 7. Code touchpoints (for implementers)

| Strategy | File(s) | Insertion point |
|----------|---------|-----------------|
| S3 context hints | [`server/server.go`](../server/server.go), [`capability/`](../capability/capability.go) | read inbound header → capability subset before `filteredTools()` |
| S6 read-only | implemented | `filteredTools()` mode branch |
| S6b read-only footprint (implemented) | MCP `ReadOnlyHint` / [`mcpclient`](../mcpclient/router.go) `read_only_tools`; product default mode; `server.buildRoutingGroups` | annotate writes so `filteredTools()` drops them; routing menu now mode-filtered; lean read-only for read-heavy turns |
| S8 intra-group tool selection (implemented) | [`capability/group.go`](../capability/group.go) (`BuildGroupsExpanding`), [`agent/agent.go`](../agent/agent.go) (`loadToolsDef`/`handleLoadTools`/`filteredTools`), `BuildSystemPromptWithRouting`, [`config/config.go`](../config/config.go) (`group_expand_threshold`) | expand oversized groups into a per-tool sub-index; additive `tools: []string` on `load_tools`; activate individual tools; off by default |
| S7 supervisor | [`agent/agent.go`](../agent/agent.go), [`capability/group.go`](../capability/group.go), [`config/config.go`](../config/config.go), [`server/server.go`](../server/server.go), [`cmd/chat-service/main.go`](../cmd/chat-service/main.go) | **S7a (built):** `tool_routing` config; `capability.BuildGroups`/`RenderGroupIndex`; `BuildSystemPromptWithRouting` group index; internal `load_tools` tool + per-message state; `filteredTools()` restriction/recompute/budget; forced-first-step nudge; `RoutingStats` telemetry. **S7b (deferred):** pre-pass LLM call → capability subset → existing `filteredTools()` filter |
