# Changelog

## v0.2.1

Additive and backward compatible: with `max_tokens` unset, behavior is
byte-for-byte identical to v0.2.0.

### Added

- **`ProviderConfig.MaxTokens` (optional).** A new `max_tokens` field caps the
  model's response length (completion tokens). When zero — the default, and the
  case for any config that predates this field — each provider preserves its
  prior behavior: the OpenAI / llm-proxy path sends no `max_tokens` (letting the
  endpoint apply its own default), while the Anthropic / Bedrock path keeps its
  built-in `4096`. Set a larger value to allow bigger responses (e.g.
  multi-panel dashboards) that would otherwise be truncated mid-stream, leaving
  an unclosed code fence that renders as raw JSON.
- **Truncation visibility.** The OpenAI / llm-proxy stream now logs a `WARN`
  when it ends with `finish_reason == "length"`, so response truncation is
  diagnosable from operator logs.

## v0.1.20

Agentic-forward UI seams (C5/C6 engine halves). All additive and **opt-in**:
with none of the new APIs used, behavior is byte-for-byte identical to v0.1.19.
See [docs/agentic-forward-seams.md](docs/agentic-forward-seams.md).

### Added

- **Open-nav-view seam (C6).** A generic, host-registered navigation tool:
  `agent.NewOpenNavTool()` returns an `InternalTool` named `open_nav_view` that
  takes a required, opaque `destination` string. When the model calls it, the
  agent emits a new `EventOpenNav`, relayed by the server as an `open_nav` SSE
  event (`{"destination":"…"}`) alongside existing events without changing any
  existing event shape. The engine hardcodes no destinations.
- **`InternalTool.Emit` hook.** An optional `Emit func(input json.RawMessage)
  []Event` field lets any host-registered tool surface lightweight side-channel
  agent events (e.g. `EventOpenNav`) in addition to its tool result. `nil` = no
  extra events (today's behavior).
- **`CanvasTabSummary.Digest` (C5).** The `canvas_tabs` context now accepts an
  optional free-text `digest` field for tabs whose content doesn't decompose
  into `key_properties` (e.g. host-rendered portal tabs). When present, an
  "Additional detail" block is appended after the Canvas Context table; when
  absent, the system prompt is byte-for-byte unchanged.
- **Ungated interests.** `requires` is now **optional** in an interest file. An
  interest with no `requires` is always available (capability-independent) —
  used by the parameterized navigation interest. Gated interests (non-empty
  `requires`) are unaffected.

## v0.1.19

All three additions are inert by default: with `tool_routing.mode` at its
default `off`, behavior is byte-for-byte identical to v0.1.18. They affect only
deployments that have in-band routing enabled.

### Changed

- Tool routing (S6b, read-only footprint reduction): the in-band routing **group
  menu** is now built from the *mode-filtered* tool set instead of every tool the
  router knows about. Previously, in read-only mode the menu still advertised a
  server's write tools (which the model could not call) and a read-only-annotated
  server did not present its reduced footprint. The menu now drops write tools in
  read-only mode exactly as `filteredTools()` does (ask-on-write capabilities
  still surface their writes), so read-only-annotated servers (e.g. Jira/
  Confluence as their `ReadOnlyHint` annotations land) show a smaller, accurate
  group and the model is never offered tools it cannot use this turn. The
  per-turn budget/headroom benefit was already automatic via `computeToolBudget`
  / `filteredTools`; this completes the menu side. No UI change required.

### Added

- Tool routing (S8, intra-group tool-level selection): `load_tools` now accepts
  an additive `tools: []string` field alongside `groups`, letting the model load
  **individual tools** from a large MCP server instead of the whole group. A new
  `tool_routing.group_expand_threshold` config (default `0` = disabled) controls
  this: any capability group whose tool count exceeds the threshold is rendered
  in the routing menu as an expandable per-tool sub-index, so the model pulls in
  only the handful of tools it needs. This lifts the group-granularity ceiling
  S7a left behind — a single oversized server (e.g. an ~80-tool ONTAP MCP) can
  now share a turn with other groups under the 128-tool cap by loading a subset.
  Loading an individual tool activates only that tool (not its group), counts as
  a forced-first-step selection, and surfaces its owning group in
  `RoutingStats.GroupsLoaded` plus the tool in the new `RoutingStats.ToolsLoaded`.
  Backward compatible: `groups` still works, and with the threshold at `0` every
  group loads wholesale exactly as before. No UI change required.
  - `capability.BuildGroupsExpanding` + expandable `RenderGroupIndex`; agent
    `load_tools` contract, tool-level activation in `filteredTools`, and
    telemetry; `tool_routing.group_expand_threshold` config wired through
    `cmd/chat-service`. Eval harness (`eval/`) gains an `ExpandThreshold` knob
    and a tool-level scenario.
- Routing-quality eval harness (S7a Layer 6) in `eval/`. `eval.RunScenario`
  drives the real agent loop with in-band routing over a synthetic multi-MCP
  environment and scores which capability groups the model loaded against an
  expected set; `eval.RunSuite` aggregates top-1 / exact / skip metrics — the
  empirical basis for the S7a→S7b decision. `eval.DefaultScenarios` seeds a
  representative jira/confluence/bitbucket/harvest(ONTAP)/zoom fixture set.
  Deterministic mock-provider tests run in CI; the live-provider run
  (`TestRealProviderEval`) is opt-in and gated behind `CHAT_EVAL_*` env vars so
  CI stays hermetic. Test-only — no runtime impact.

## v0.1.18

### Fixed

- Tool routing: the `/chat/capabilities` budget endpoints now account for
  in-band routing. Previously they always summed every enabled capability's
  tools, so with routing enabled the UI still reported e.g. "140 tools would be
  sent (max 128)" and blocked the read-write toggle / capability enables — even
  though each routed turn stays under the cap. When `tool_routing.mode` is
  `in-band`, `GetChatCapabilities` and `PostChatCapabilities` now report the
  binding limit as the **largest single capability** (the smallest set the
  model can load via `load_tools`) instead of the sum. A deployment can now
  enable more than 128 tools total; only a single server whose own tools exceed
  the cap is rejected (routing cannot split one server). No chat-component /
  UI change required — the component reads the server's `tool_budgets` directly.

## v0.1.17

### Docs

- Document the S7a in-band tool-routing supervisor in
  `docs/chatbot-architecture.md`: new §2.7 (tool routing), the group-index
  block in the system prompt (§2.6), group routing as a third tool-filtering
  stage (§6.2), the `tool_routing` config block (§10.4), and file-index /
  related-docs entries. No code changes.

## v0.1.16

### Added

- High-tool-count scaling: the **S7a in-band tool-routing supervisor**. As the
  number of connected MCP servers grows, the per-turn tool list (and the hard
  128-tool provider cap) becomes a problem. When enabled, the main model
  self-selects which capability groups it needs via an internal `load_tools`
  tool before those groups' tools are loaded — no dedicated routing LLM call.
  Disabled by default; absent configuration, behavior is byte-for-byte
  identical to before. Server-side only — the chat UI component
  (`@edjbarron/netapp-chat-component`) is unchanged. See
  `docs/high-tool-count-scaling.md`.
  - `tool_routing` config block (`mode: off | in-band | router`, `max_tools`,
    `always_on`). `off` is the default; `router` (S7b) is parsed but rejected
    at startup until implemented.
  - Optional per-server `capability_name` / `capability_description` to label
    and describe a capability/group in the routing menu (auto-derived from the
    server's tool names when absent).
  - `capability.BuildGroups` / `capability.RenderGroupIndex` — a pure,
    host-agnostic group registry derived 1:1 from connected capabilities.
  - `agent.WithToolRouting`, the internal `load_tools` tool, group-aware
    `agent.BuildSystemPromptWithRouting`, per-message routed-tool filtering with
    a budget guard, an optional forced-first-step nudge (on by default for
    in-band), and `agent.RoutingStats` / `(*Agent).LastRoutingStats` telemetry
    (groups offered/loaded, load calls, reloads, skip/compliant).

## v0.1.15

### Added

- Per-request MCP header forwarding. Each MCP server config gains an optional
  `forward_headers` allowlist of inbound HTTP header names. When a `/chat/*`
  request carries a listed header, its value is relayed onto the outbound MCP
  requests made while serving that request (per-server opt-in; per-request
  value; opaque to this service). Absent any `forward_headers`, behavior is
  byte-for-byte identical to before. Enables a host application to authorize
  the end user behind each chat turn without per-user MCP sessions. See
  `docs/mcp-request-header-forwarding.md`.
  - `config.MCPServer.ForwardHeaders` / `mcpclient.ServerConfig.ForwardHeaders`
    (yaml `forward_headers`).
  - `mcpclient.WithForwardedHeaders` + `Router.CollectForwardableHeaders`.

## v0.1.14

### Breaking changes

- Removed the built-in `render_volume_detail` tool (`render/volume.go`) and its
  `MetricsFetcher` interface. Products that need a volume-detail render tool
  must register one via `agent.ChatDeps.ExtraTools`. NABox owns this from its
  own `internal/render` package.
- Removed ONTAP-specific vocabulary (SVM/aggregate/cluster examples, qualifier
  conventions, `ontap-cli` proposal format, the `harvest`/`ontap`/`grafana`
  capability examples) from the hardcoded system prompt in `agent/agent.go`.
  Products inject equivalent guidance through the new
  `SystemPromptConfig.Vocabulary` field. Consumers that did not need ONTAP
  guidance benefit from a smaller default prompt.

### Added

- `agent.SystemPromptConfig.Vocabulary` (string, optional) — free-form markdown
  block appended after the generic Guidelines and before the connected-data-
  sources list. Defaults to empty.

### Changed

- Renamed `interest/testdata/interests/` → `interest/examples/` and added a
  README clarifying these are reference fixtures, not auto-loaded interests.
  The chat service ships with **zero** built-in interests; each product
  supplies its own via `config.interests.dirs`.
- MCP client default name `"nabox-chatbot"` → `"netapp-chat-service"`.
- `llm.ProviderConfig` comment no longer hardcodes `/etc/nabox/ai.yaml` as the
  storage location; the path is host-product-specific.
