# Changelog

All notable changes to `@edjbarron/netapp-chat-component` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the package adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.7] - 2026-04-25

### Added
- New optional `defaultMode` prop on `<ChatPanel>` (and matching `useChatPanel({ defaultMode })` option) for setting the initial chat mode (`'read-only'` or `'read-write'`). Users can still toggle at runtime via the existing ModeToggle UI.

### Changed
- **Default initial mode is now `'read-write'`** (was `'read-only'`). This restores backward compatibility with deployments whose MCP servers don't yet emit `ToolAnnotations` — in `'read-only'` mode the backend filters out unannotated tools, which can result in zero tools reaching the LLM. Consumers who want the previous behavior can pass `<ChatPanel defaultMode="read-only" />`.

## [0.1.6] - 2026-04-25

### Fixed
- **`sendMessage` now honors `headers` and `credentials` configured via `createChatAPI`.** Previously the streaming `POST /chat/message` request hand-rolled its own `fetch()` and silently dropped any custom headers (e.g. `Authorization: Bearer ...`, `X-Tenant`) and forced `credentials: 'include'`. Auth-gated and multi-tenant deployments could not use `<ChatPanel>` out of the box — `GET` requests authenticated, but every message send returned 401/403. Reported externally; see commit `c584782`.

### Added
- New `ChatAPI.stream(path, body, signal?): Promise<Response>` method. The default `createChatAPI` implementation routes the streaming POST through this method using the same configured `headers`/`credentials` as `get`/`post`/`delete`.

### Breaking (type-only, pre-1.0)
- Custom implementations of the `ChatAPI` interface must add a `stream()` method. Consumers using `createChatAPI` are unaffected.

## [0.1.5] - 2026-04-24

### Added
- Bookmark prompts with MCP-aware filtering: capability-gated prompt suggestions surfaced in `ChatPanel` based on which MCP tools are currently allowed.

## [0.1.4] - 2026-04-22

### Changed
- Widened Mantine peer dependency ranges from `^8.0.0` to `^8.0.0 || ^9.0.0` (`@mantine/core`, `@mantine/charts`, `@mantine/hooks`) so consumers on Mantine 9 can install without `ERESOLVE` errors. No code changes — Mantine 9 is API-compatible for the components used.

## [0.1.3] - 2026-04-22

### Fixed
- `ResourceTableBlock` object-column fix (republish of the 0.1.2 fix; see commit `bb69bdd`).

## [0.1.2] - 2026-04-22

### Added
- First release published via GitHub Actions using npm trusted publisher (OIDC).

### Fixed
- `ResourceTableBlock`: tolerate object-shaped column entries from LLM output (`{key, label}`, `{name}`, `{field}`, `{header}`, `{title}`, `{id}` are all normalized; previously a non-string column could crash React rendering).

## [0.1.0] - 2026-04-22

### Added
- Initial public release of the React chat UI component for the [`netapp-chat-service`](https://github.com/ebarron/netapp-chat-service) backend.
- Components: `ChatPanel`, `CanvasPanel`, `ModeToggle`, `CapabilityControls`, `ActionConfirmation`, `ToolStatusCard`.
- Charts: `ChartBlock`, `DashboardBlock`, `ObjectDetailBlock`, `AutoJsonBlock`.
- Hook: `useChatPanel`.
- API: `createChatAPI`, `ChatAPIProvider`, `useChatAPI`.

[0.1.7]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.7
[0.1.6]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.6
[0.1.5]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.5
[0.1.4]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.4
[0.1.3]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.3
[0.1.2]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.2
[0.1.0]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.0
