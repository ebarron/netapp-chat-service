# Changelog

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
