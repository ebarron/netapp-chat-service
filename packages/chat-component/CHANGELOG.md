# Changelog

All notable changes to `@edjbarron/netapp-chat-component` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the package adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.2] - 2026-07-23

Agentic-forward layout modes (C7–C8). All additive and **opt-in**: with none of
the new props used, `ChatAppShell` renders byte-for-byte as `0.2.1`. Component-only
— no engine, `/chat/*`, or binary changes. See
[docs/agentic-forward-layout-modes.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-layout-modes.md).

### Added
- **Docked navigation (C7).** `ChatAppShell` gains `navMode?: 'overlay' | 'docked'`
  (default `'overlay'`) and `navDockedWidth?: number` (default = `navOverlayWidth`,
  260). In `'docked'`, `renderNavMenu` renders as a persistent left column and no
  `Drawer` is mounted; the header hamburger's `toggleNav` collapses/expands the
  column and `navOpened` reflects its expanded/collapsed state. The
  `ChatAppShellHeaderApi` is unchanged, and in docked mode `openNav` no longer
  force-closes the column.
- **Assistant placement (C8a).** `assistantPlacement?: 'start' | 'end'`
  (default `'start'`) flips the assistant↔canvas flex order. Width, min/max,
  resize, and persistence are placement-agnostic; the drag separator inverts its
  direction so it stays natural on either side.
- **Collapsible assistant (C8b).** `assistantCollapsed` (controlled),
  `defaultAssistantCollapsed` (uncontrolled), `onAssistantCollapsedChange`, and
  `persistAssistantCollapsedKey`. Collapsing hides the assistant column via CSS
  (it does **not** unmount `ChatPanel`, so conversation, mode, and in-flight
  streams are preserved) and shows a focusable reveal rail
  (`aria-label="Show assistant"`, `aria-expanded={false}`) on the assistant's
  side; expanded mode shows a "Hide assistant" affordance.

## [0.2.1] - 2026-07-22

### Added
- **`ChatAppShell` / docked split sizing.** Opt-in props to control the assistant/canvas
  column split in the docked shell: `assistantWidth`, `defaultAssistantWidth`,
  `assistantMinWidth` (default `320`), `assistantMaxWidth`, `resizableAssistant`,
  `onAssistantWidthChange`, and `persistAssistantWidthKey`. Width is applied via
  the `--chat-assistant-width` CSS custom property; the canvas takes the remainder
  (`flex: 1 1 auto`). When `resizableAssistant` is true, a focusable vertical
  separator supports pointer drag, arrow-key nudging (±16px, Shift for ±64px),
  and double-click reset. `onAssistantWidthChange` receives the clamped pixel
  width during resize and again on release. All additive: omitting the new props
  preserves the legacy fixed `40%` / `60%` split with no handle.
- Hosts may also set the inherited `--chat-assistant-width` custom property
  directly, avoiding overrides against generated CSS-module class names.

## [0.1.20] - 2026-07-20

Agentic-forward UI seams (C1, C2–C4, C5, C6). All additive and **opt-in**: with
none of the new APIs used, the component behaves byte-for-byte as 0.1.18. See
[docs/agentic-forward-seams.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-seams.md).

### Added
- **`<ChatPanel variant="docked">` (C1).** A persistent, full-height layout
  (assistant + canvas) that fills its parent, for a full-width-header shell. The
  default `variant="drawer"` is today's slide-over, unchanged. In docked mode the
  panel is always present (not gated by `opened`).
- **Host content in a canvas tab via portal (C2–C4).** A new imperative
  `ChatPanelHandle` (via `ref`) exposes `openHostCanvasTab` / `updateHostCanvasTab`
  / `setCanvasTabSummary` / `closeCanvasTab` / `focusCanvasTab`. The component
  renders an empty mount node for a host tab and reports it via the new
  `onHostTabPortal(tabId, el)` prop; the host portals its own React tree in. The
  component never imports host pages. Host tabs support a reserved,
  **eviction-exempt-but-user-closable** tab via `evictable: false` (e.g. the
  single reused `nav` tab); the canvas still hides at zero tabs.
- **Canvas context provider (C5).** A per-tab `CanvasTabSummary`
  (`kind`/`name`/`qualifier`/`status`/`key_properties` plus a new free-text
  `digest`) can be attached to any canvas tab; it is forwarded in the existing
  `canvas_tabs` field of `/chat/message`. Empty fields are omitted cleanly.
  Legacy declarative tabs keep their exact prior wire shape.
- **Open-nav handling (C6).** The new `open_nav` SSE event is parsed and surfaced
  to the new `onOpenNav(destination)` prop; with no handler it is a safe no-op.
- New exported types: `CanvasTabSummary`, `HostCanvasTabInput`, `ChatPanelHandle`,
  `CanvasEventInfo`.

## [0.1.18] - 2026-07-18

### Fixed
- **Fence-fragmented dashboards/charts now render correctly.** When an LLM
  emitted a large `dashboard`/`chart` object with an interior ` ```json ` fence
  wrapped around its `rows`/`data` array, the object was split across the
  fence: the skeleton showed as raw JSON text and the fragmented rows rendered
  as stray standalone cards, so no panel appeared. `wrapInlineChartJson` now
  runs a fence-aware reassembly pre-pass (`reassembleFencedJson`) that stitches
  such an object back together — but only when the fence-stripped text actually
  parses as JSON and classifies as structured, so ordinary prose and legitimate
  code blocks are untouched. As defense-in-depth, the bare-JSON scanner no
  longer wraps the nested child objects (table rows, series points) of an
  unbalanced chart/dashboard fragment as separate cards; it emits the remainder
  verbatim instead.

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
