# Changelog

## v0.1.19

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
