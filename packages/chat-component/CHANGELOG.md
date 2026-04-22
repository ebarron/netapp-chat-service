# Changelog

All notable changes to `@edjbarron/netapp-chat-component` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the package adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.2]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.2
[0.1.0]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.0
