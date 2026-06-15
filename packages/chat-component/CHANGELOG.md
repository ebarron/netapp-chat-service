# Changelog

All notable changes to `@edjbarron/netapp-chat-component` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the package adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.17] - 2026-06-15

### Added
- `<ChatPanel>` gains an optional `onCanvasEvent` callback, fired whenever a
  canvas tab opens/updates or closes (`{ action: 'open' | 'close', tabId, title,
  kind }`). Lets a host react to a specific canvas — e.g. refresh a page when a
  matching canvas changes — instead of polling on every turn.
- A canvas payload whose `content.close` is truthy now **closes** the matching
  canvas tab (by `tab_id`) instead of opening one, and is reported via
  `onCanvasEvent` with `action: 'close'`. A bespoke render tool can use this to
  tear down a tab after its underlying object is deleted, in the same
  re-render-after-action flow it already uses.

## [0.1.16] - 2026-06-15

### Fixed
- Canvas renderers now **remount when a tab's content is replaced** (keyed by
  the serialized content). This resets transient form state on an in-place
  re-render: `action-form` text inputs return to their defaults instead of
  retaining the previous render's typed text, and index-keyed panel reuse can no
  longer bleed one form's input into another (e.g. a clone form's text leaking
  into an edit form after the canvas re-renders). Re-rendering with identical
  content does not remount.

## [0.1.15] - 2026-06-15

### Added
- `<ChatPanel>` gains three optional props for **host-driven prompt injection**,
  letting an embedding app open the panel and auto-send a prompt (e.g. an
  "Explain this" button elsewhere in the product):
  - `pendingPrompt?: string` — a prompt to auto-send once when the panel is
    `opened` and idle; sent as a normal user message exactly once.
  - `onPromptConsumed?: () => void` — called after the pending prompt is sent so
    the host can clear its own state.
  - `onBusyChange?: (busy: boolean) => void` — surfaces the assistant's streaming
    state so the host can disable its trigger control while a turn is in flight.
  All three are optional and additive; existing embeds are unaffected. The
  injected prompt is subject to the same mode and action-approval gating as any
  typed message. See `docs/host-prompt-injection.md`.

## [0.1.13] - 2026-05-13

### Changed
- `ActionFormBlock` now renders checkbox/switch fields inline with text and select inputs in the same responsive grid (was: switches forced onto their own row). Reclaims another row of vertical space in the canvas.

## [0.1.12] - 2026-05-13

### Changed
- `ActionFormBlock` now lays out input fields in up to 3 columns on `sm`+ viewports (was capped at 2). Reclaims vertical space in dense provisioning forms while still collapsing to 2/1 columns at `xs`/`base`.

## [0.1.8] - 2026-04-25

### Added
- `<ChatPanel>` now surfaces `capabilityError` (e.g. tool-budget rejection from `setMode` or a `POST /chat/capabilities` 409) as a dismissible red `Alert` immediately below the mode toggle, in addition to passing it down to `CapabilityControls`. Previously the error was only visible inside the capabilities popover, so users hit by the `setMode` budget block had no visible feedback in the main panel.

### Changed
- Clarified the read-write budget-block error message: `"Cannot switch to read-write: N tools would be sent (max 128). Open Capabilities and disable an MCP to free up budget."` (was `"Switching to read-write would enable N tools (max 128). Disable an MCP capability before switching mode."`).

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
8]: https://github.com/ebarron/netapp-chat-service/releases/tag/chat-component-v0.1.8
[0.1.
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
