# User Guide — `@edjbarron/netapp-chat-component`

A practical, task-oriented guide to embedding and driving the React chat UI for
the [netapp-chat-service](https://github.com/ebarron/netapp-chat-service) agentic
backend (LLM + MCP tool routing).

For a high-level overview of what the package is and why, see the
[README](./README.md). This guide covers **how** to use it.

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [Initial chat mode](#initial-chat-mode)
- [Driving the panel from the host](#driving-the-panel-from-the-host)
- [Agentic-forward UI seams (C1–C6)](#agentic-forward-ui-seams-c1c6)
  - [C1 — Docked vs. drawer](#c1--docked-vs-drawer)
  - [C2–C4 — Host content in a canvas tab](#c2c4--host-content-in-a-canvas-tab)
  - [C5 — Canvas context provider](#c5--canvas-context-provider)
  - [C6 — Navigation by prompt](#c6--navigation-by-prompt)
- [The docked assistant/canvas split](#the-docked-assistantcanvas-split)
- [Layout modes (C7–C8)](#layout-modes-c7c8)
- [Optional assistant header](#optional-assistant-header)
- [Hiding the single-tab strip](#hiding-the-single-tab-strip)
- [Mobile layout](#mobile-layout)
- [Auth headers and credentials](#auth-headers-and-credentials)
- [`ChatAppShell` prop reference](#chatappshell-prop-reference)
- [Exports](#exports)
- [Backend](#backend)
- [Related documents](#related-documents)

---

## Installation

```bash
npm install @edjbarron/netapp-chat-component
```

### Peer dependencies

Install these in your host app:

```bash
npm install react react-dom \
  @mantine/core @mantine/charts @mantine/hooks \
  @tabler/icons-react
```

Supports React 18 and 19, Mantine 8.x and 9.x.

---

## Quick start

```tsx
import { MantineProvider } from '@mantine/core';
import {
  ChatAPIProvider,
  ChatPanel,
  createChatAPI,
} from '@edjbarron/netapp-chat-component';

import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import '@edjbarron/netapp-chat-component/styles.css';

const api = createChatAPI('https://your-chat-service.example.com/api');

export function App() {
  return (
    <MantineProvider>
      <ChatAPIProvider value={api}>
        <ChatPanel />
      </ChatAPIProvider>
    </MantineProvider>
  );
}
```

The three stylesheet imports are required: Mantine core, Mantine charts (for the
chart/dashboard kit), and the component's own bundled styles.

---

## Core concepts

| Piece | What it is |
|-------|-----------|
| `ChatPanel` | The assistant UI. Renders as a slide-over `Drawer` (default) or a persistent `docked` panel. When canvas tabs are open it splits into assistant + canvas. |
| `ChatAppShell` | An opt-in, app-agnostic "AI-forward" workspace shell: a host header, a docked assistant, a tabbed canvas, and a navigation surface (overlay or docked). The host plugs in through slots/callbacks; routing stays host-side. |
| `CanvasPanel` | The tabbed canvas beside the assistant. Renders engine-driven charts/dashboards and host-portaled pages. |
| `useChatPanel` | The hook powering `ChatPanel` (messages, streaming, mode, capabilities, canvas tabs). Use it directly for a fully custom UI. |
| `createChatAPI` / `ChatAPIProvider` / `useChatAPI` | The transport layer. `createChatAPI` builds a `ChatAPI` with shared headers/credentials; the provider makes it available to the component tree. |

There are two integration levels:

- **Low-level:** drop a `ChatPanel` into your own layout (`variant="drawer"` or
  `variant="docked"`).
- **High-level:** use `ChatAppShell` to get the full docked workspace (header +
  assistant + canvas + navigation) with the reserved nav-tab lifecycle managed
  for you.

---

## Initial chat mode

`<ChatPanel>` opens in **read-write mode** by default. To start in read-only mode
(information-retrieval tools only), pass `defaultMode`:

```tsx
<ChatPanel defaultMode="read-only" />
```

The user can still toggle mode at runtime via the in-panel `ModeToggle`;
`defaultMode` only sets the initial value. The backend filters tools by mode based
on each MCP tool's `ToolAnnotations.ReadOnlyHint` — if your MCP servers don't yet
emit annotations, leave `defaultMode` at its default so all their tools remain
available.

---

## Driving the panel from the host

The host can open the panel and **auto-send a prompt** programmatically — useful
for "Explain this" / "Ask about this" buttons elsewhere in your app that deep-link
a question into the assistant.

| Prop | Type | Description |
|------|------|-------------|
| `pendingPrompt` | `string` | A prompt to auto-send once. When it changes to a non-empty value while the panel is `opened` and not streaming, it is sent as a user message exactly once. |
| `onPromptConsumed` | `() => void` | Called after `pendingPrompt` has been submitted. Clear your own state here so the same prompt isn't resent on a later re-render. |
| `onBusyChange` | `(busy: boolean) => void` | Notifies the host when the assistant's busy (streaming) state changes, so you can disable your trigger control while a turn is in flight. |
| `onCanvasEvent` | `(info: { action: 'open' \| 'close'; tabId: string; title: string; kind: string }) => void` | Fired when a canvas tab opens/updates or closes. Lets the host react to a specific canvas (e.g. refresh a page when a matching canvas changes). A canvas payload whose `content.close` is truthy closes the matching tab and reports `action: 'close'`. |

```tsx
function App() {
  const [opened, setOpened] = useState(false);
  const [pendingPrompt, setPendingPrompt] = useState('');
  const [busy, setBusy] = useState(false);

  const askAssistant = (prompt: string) => {
    setPendingPrompt(prompt);
    setOpened(true);
  };

  return (
    <>
      <button disabled={busy} onClick={() => askAssistant('Explain rule X')}>
        Explain this
      </button>

      <ChatPanel
        opened={opened}
        onClose={() => setOpened(false)}
        pendingPrompt={pendingPrompt}
        onPromptConsumed={() => setPendingPrompt('')}
        onBusyChange={setBusy}
      />
    </>
  );
}
```

A common pattern is to expose `askAssistant` and `busy` through a React context so
any page can request a prompt and grey out its trigger while the assistant is
busy. If the host opens the panel mid-turn, the prompt is held until the current
turn finishes, then sent.

The injected prompt is sent as a normal **user** message and is subject to the
same mode (read-only/read-write), capability filtering, and action-approval
gating as any typed prompt — the host cannot bypass these.

---

## Agentic-forward UI seams (C1–C6)

These additive, opt-in features let you build a persistent docked assistant beside
a canvas that also hosts your own pages, with everything on screen exposed to the
assistant. All default to today's behavior. Full reference:
[docs/agentic-forward-seams.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-seams.md).

| Prop | Type | Description |
|------|------|-------------|
| `variant` | `'drawer' \| 'docked'` | **C1.** `'drawer'` (default) is the slide-over. `'docked'` renders a persistent full-height panel (assistant + canvas) that fills its parent, for a full-width-header shell. Always present (not gated by `opened`). |
| `onHostTabPortal` | `(tabId: string, el: HTMLElement \| null) => void` | **C2–C4.** Portal mount callback for host-content canvas tabs. `el` is the mount node when a host tab mounts, `null` when it unmounts. Render your page into `el` via `ReactDOM.createPortal`. |
| `onOpenNav` | `(destination: string) => void` | **C6.** Called when the engine emits an `open_nav` SSE event (from a host-registered `open_nav_view` tool). Absence is a safe no-op. |

### C1 — Docked vs. drawer

`variant="docked"` renders a persistent, full-height panel that fills its parent —
the assistant column on the left and (when canvas tabs are open) the canvas filling
the remaining width. Intended to sit under a host-provided full-width header. In
docked mode the panel is always present (not gated by `opened`) and fetches
config/capabilities on mount.

```tsx
<div className="app-shell">
  <MyFullWidthHeader />
  <ChatPanel variant="docked" onClose={() => {}} opened />
</div>
```

### C2–C4 — Host content in a canvas tab

The component can host arbitrary host-provided React content in a canvas tab
**without importing host code**. It renders an empty mount node and reports it via
`onHostTabPortal`; the host portals its own tree in with `ReactDOM.createPortal`,
keeping the page inside the host's own React tree (its providers, router, theme)
while it visually lives in the canvas.

```tsx
import { createPortal } from 'react-dom';
import type { ChatPanelHandle, CanvasTabSummary } from '@edjbarron/netapp-chat-component';

const chat = useRef<ChatPanelHandle>(null);
const [navMount, setNavMount] = useState<HTMLElement | null>(null);

<ChatPanel
  ref={chat}
  variant="docked"
  opened
  onClose={() => {}}
  onHostTabPortal={(tabId, el) => { if (tabId === 'nav') setNavMount(el); }}
/>

// Open the single reused, eviction-exempt nav tab and attach a context summary:
chat.current?.openHostCanvasTab({
  tabId: 'nav',
  title: 'Alerting',
  kind: 'nav-view',
  qualifier: '/alerting',
  evictable: false, // exempt from max-tab eviction, still user-closable
  summary: { status: 'warning', digest: '3 rules enabled, 1 disabled.' } satisfies CanvasTabSummary,
});

{navMount && createPortal(<AlertingPage />, navMount)}
```

`ChatPanelHandle` (obtained via `ref`) also exposes `updateHostCanvasTab`,
`setCanvasTabSummary`, `closeCanvasTab`, and `focusCanvasTab`.

Tab lifecycle notes:

- **Dedup by `tabId`.** Re-opening the same `tabId` replaces the tab's identity and
  refocuses it — this is how a single reused `nav` tab works.
- **Eviction-exempt but closable.** `evictable: false` exempts a tab from the
  max-tab FIFO eviction while keeping it user-closable.
- **Hidden at zero tabs.** When the last tab closes, the canvas region collapses;
  the docked assistant remains.

### C5 — Canvas context provider

So the assistant can answer questions about **anything on screen** (including
opaque host pages), attach a per-tab `CanvasTabSummary`. The component relays it in
the existing `canvas_tabs` field of `POST /chat/message`.

```ts
export interface CanvasTabSummary {
  kind?: string;
  name?: string;
  qualifier?: string;
  status?: string;
  key_properties?: Record<string, string>;
  digest?: string; // free-text summary for pages that don't fit key/values
}
```

Attach via `openHostCanvasTab({ ..., summary })`, `updateHostCanvasTab(tabId,
{ summary })`, or `setCanvasTabSummary(tabId, summary)`. Empty fields are omitted
cleanly. **Exclude secrets** from summaries/digests.

### C6 — Navigation by prompt

When the engine emits an `open_nav` SSE event (from a host-registered
`open_nav_view` tool), the component calls `onOpenNav(destination)`. With no handler
registered it is a safe no-op. Hosts typically handle it by opening the destination
in the reused nav canvas tab — making a typed "open alerting" indistinguishable
from a manual nav selection.

---

## The docked assistant/canvas split

When the assistant and canvas are side-by-side, the split defaults to **40%
assistant / 60% canvas**. `ChatAppShell` (and a docked `ChatPanel`) let hosts
control and resize it.

```tsx
<ChatAppShell
  assistantWidth="32%"            // controlled; numbers are pixels
  assistantMinWidth={320}
  assistantMaxWidth={720}
  resizableAssistant
  onAssistantWidthChange={(px) => setAssistantWidth(px)}
  persistAssistantWidthKey="planning-console-assistant-width"
  // ...existing shell props
/>
```

| Prop | Type | Description |
|------|------|-------------|
| `assistantWidth` | `number \| string` | Controlled width. Numbers are pixels; strings are any CSS length/percentage. Sets `--chat-assistant-width`; the canvas takes the remainder. Default `"40%"`. |
| `defaultAssistantWidth` | `number \| string` | Uncontrolled initial width. |
| `assistantMinWidth` | `number` | Minimum width (px). Default `320`. |
| `assistantMaxWidth` | `number` | Optional maximum width (px). |
| `resizableAssistant` | `boolean` | Render a draggable divider. Default `false`. |
| `onAssistantWidthChange` | `(px: number) => void` | Fires with the clamped pixel width during resize and on release. |
| `persistAssistantWidthKey` | `string` | Persist the user width under this `localStorage` key (SSR-safe restore). |

When resizing is enabled, pointer dragging and Left/Right arrow keys report a
clamped pixel width through `onAssistantWidthChange`; Shift changes the keyboard
step from 16px to 64px, and double-click resets the width. Widths are clamped to
`[assistantMinWidth, assistantMaxWidth]` and to 15%–60% of the container so the
canvas never disappears.

All split props are optional. Omitting them preserves the 40% / 60% layout and
renders no separator. Hosts can also set `--chat-assistant-width` from CSS without
targeting hashed module class names:

```css
.my-chat-shell {
  --chat-assistant-width: 32%;
}
```

---

## Layout modes (C7–C8)

`ChatAppShell` exposes three orthogonal, opt-in layout axes so hosts can compose
their own presets. The component ships no preset vocabulary. Full reference:
[docs/agentic-forward-layout-modes.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-layout-modes.md).

| Prop | Type | Description |
|------|------|-------------|
| `navMode` | `'overlay' \| 'docked'` | **C7.** `'overlay'` (default) is the hamburger `Drawer`. `'docked'` renders `renderNavMenu` as a persistent left column (no Drawer); the hamburger's `toggleNav` collapses/expands it and `navOpened` reflects that state. |
| `navDockedWidth` | `number` | Width (px) of the docked nav column. Defaults to `navOverlayWidth` (260). |
| `assistantPlacement` | `'start' \| 'end'` | **C8a.** Side of the canvas the assistant sits on. `'start'` (default) is left; `'end'` is right. Order-only — width/resize/persistence are unchanged. |
| `assistantCollapsed` | `boolean` | **C8b.** Controlled collapse of the assistant column. |
| `defaultAssistantCollapsed` | `boolean` | Uncontrolled initial collapsed state. |
| `onAssistantCollapsedChange` | `(collapsed: boolean) => void` | Fired on every collapse toggle. |
| `persistAssistantCollapsedKey` | `string` | Persist the collapsed boolean under this `localStorage` key (SSR-safe restore). |

Collapsing hides the assistant column via CSS — it does **not** unmount
`ChatPanel`, so the conversation, mode, and any in-flight stream survive. A
focusable reveal rail (`aria-label="Show assistant"`) is pinned to the assistant's
side; expanded mode shows a "Hide assistant" affordance.

```tsx
// Default (current) — 2 columns: assistant left, canvas; nav overlay.
<ChatAppShell /* no navMode/placement/collapsed */ … />

// 3 columns — nav left · canvas middle · chat right.
<ChatAppShell navMode="docked" assistantPlacement="end" … />

// Nav + canvas, chat tucked away behind a reveal rail on the right.
<ChatAppShell
  navMode="docked"
  assistantPlacement="end"
  defaultAssistantCollapsed
  persistAssistantCollapsedKey="host.assistantCollapsed"
  …
/>
```

> **Narrow viewport.** The `<1024px` behavior takes precedence: the split can't
> fit, so the canvas goes full width and the assistant is reached via the host's
> narrow affordance regardless of C7/C8 props. Treat C7/C8 as wide-viewport
> presentation.

---

## Optional assistant header

`ChatPanel` and `ChatAppShell` do not provide a default assistant title. In
`docked` and full-page layouts, the full-width assistant header bar renders only
when `title` and/or `subtitle` is provided:

```tsx
// No assistant header bar; the assistant + canvas use the full body height.
<ChatAppShell {...shellProps} />

// Render the assistant header bar.
<ChatAppShell {...shellProps} title="Planning Assistant" subtitle="Beta" />
```

The mode, bookmarks, capabilities/settings, clear, and hide-assistant controls
remain in the assistant column and are unaffected when the header bar is absent.
The drawer variant also omits its title/icon group when both values are absent,
while retaining the drawer close button.

> **Migration from 0.2.x:** `title` previously defaulted to `"AI Assistant"`.
> Pass `title="AI Assistant"` explicitly if you want to preserve that header.

---

## Hiding the single-tab strip

Set `hideSingleTab` to remove the canvas tab chrome while exactly one tab is
open. The tab content remains visible. The strip appears automatically when a
second tab opens and hides again after the tab count returns to one.

```tsx
<ChatAppShell
  hideSingleTab
  // ...existing shell props
/>
```

This works for either a reserved host navigation tab or an engine-created tab.
The default is `false`, so omitting the prop preserves the always-visible tab
strip whenever at least one tab is open. The prop is available directly on
`CanvasPanel` and `ChatPanel` as well as `ChatAppShell`.

---

## Mobile layout

`ChatAppShell` has **no responsive behavior by default** — below 1024px the
canvas (and any host-portal pages in it) is removed, matching `0.3.1`. Opt in
with `mobileLayout` to get a built-in single-column phone layout:

```tsx
<ChatAppShell
  mobileLayout
  mobileBreakpoint={1024}          // default
  mobileDefaultRegion="canvas"     // show the host page first
  mobileRegionSwitch="bottom-tab"  // "Page · Assistant" bar (default)
  persistMobileRegionKey="host.mobileRegion"
  // ...existing shell props
/>
```

Below the breakpoint:

- The **host-portal mount stays mounted** (inactive region is CSS-hidden, never
  unmounted — conversation, page state, and in-flight SSE survive).
- Nav is forced to a burger-driven overlay `Drawer` (existing header hamburger).
- A **"Page · Assistant" bottom-tab bar** switches the visible region
  (`mobileRegionSwitch="toggle"` is an alternative).
- The assistant is full-width; placement/resize/collapse props are inert.
- Engine-emitted canvases still inline in the chat transcript when narrow
  (unchanged from `0.3.1`); the persistent canvas region is for host pages.

With `mobileLayout` omitted, behavior is byte-for-byte `0.3.1`. Full reference:
[docs/mobile-layout.md](https://github.com/ebarron/netapp-chat-service/blob/main/docs/mobile-layout.md).

---

## Auth headers and credentials

`createChatAPI` accepts custom `headers` and a `credentials` mode that are applied
to **every** request (including the streaming `POST /chat/message`):

```ts
const api = createChatAPI('/api', {
  headers: {
    Authorization: `Bearer ${token}`,
    'X-Tenant': 'acme',
  },
  credentials: 'same-origin', // defaults to 'include'
});
```

If you implement `ChatAPI` yourself instead of using `createChatAPI`, you must
implement `stream(path, body, signal?): Promise<Response>` in addition to
`get`/`post`/`delete`. The component uses `stream()` for the SSE message endpoint
so your transport layer can apply auth uniformly.

---

## `ChatAppShell` prop reference

The shell is app-agnostic: the host plugs in only through slots and callbacks.
Routing stays host-side.

**Required slots**

| Prop | Type | Description |
|------|------|-------------|
| `renderHeader` | `(api: ChatAppShellHeaderApi) => ReactNode` | Host banner (logo, actions) including the hamburger trigger; wire it to `api.toggleNav`. |
| `destinations` | `ChatAppShellDestination[]` | The `{ id, label, route }` navigation catalog (data, not components). |
| `renderDestination` | `(api: ChatAppShellDestinationApi) => ReactNode` | Renders the host page for a destination into the canvas hole (portaled). |
| `renderNavMenu` | `(api: ChatAppShellNavApi) => ReactNode` | The host navigation menu tree, rendered in the overlay or the docked column. |

**Chat wiring** (forwarded to the docked `ChatPanel`): `chatAPI`, `title`,
`subtitle`, `defaultMode`, `suggestedPrompts`, `bookmarkPrompts`, `pendingPrompt`,
`onPromptConsumed`, `onBusyChange`, `onCanvasEvent`, `hideSingleTab`.

`title` has no default. When both `title` and `subtitle` are omitted, the docked
assistant header bar is not rendered. `hideSingleTab` defaults to `false`; when
enabled, it hides the canvas tab strip while exactly one tab is open without
hiding that tab's content.

**Mobile layout** (opt-in): `mobileLayout`, `mobileBreakpoint`,
`mobileDefaultRegion`, `mobileRegionSwitch`, `mobileRegion`,
`defaultMobileRegion`, `onMobileRegionChange`, `persistMobileRegionKey`. See
[Mobile layout](#mobile-layout).

**Header / layout**: `headerHeight` (default 60), `style`.

**Navigation**: `navOverlayTitle` (default "Navigation"), `navOverlayWidth`
(default 260), `navMode`, `navDockedWidth`. See [Layout modes](#layout-modes-c7c8).

**Split sizing**: `assistantWidth`, `defaultAssistantWidth`, `assistantMinWidth`,
`assistantMaxWidth`, `resizableAssistant`, `onAssistantWidthChange`,
`persistAssistantWidthKey`. See [The docked split](#the-docked-assistantcanvas-split).

**Placement / collapse**: `assistantPlacement`, `assistantCollapsed`,
`defaultAssistantCollapsed`, `onAssistantCollapsedChange`,
`persistAssistantCollapsedKey`. See [Layout modes](#layout-modes-c7c8).

**Route sync**: `activeDestinationId`, `onActiveDestinationChange`,
`resolveDestination`.

**Reserved nav tab**: `navTabId` (default "nav"), `navTabKind` (default
"nav-view"), `buildSummary`.

---

## Exports

- Components: `ChatPanel`, `CanvasPanel`, `ModeToggle`, `CapabilityControls`, `ActionConfirmation`, `ToolStatusCard`, `ChatAppShell`
- Charts: `ChartBlock`, `DashboardBlock`, `ObjectDetailBlock`, `AutoJsonBlock`
- API: `createChatAPI`, `ChatAPIProvider`, `useChatAPI`
- Hook: `useChatPanel`
- Types: `ChatMessage`, `Capability`, `PendingApproval`, `ChatMode`, `CanvasTab`, `CanvasEventInfo`, `CanvasTabSummary`, `CanvasControlOptions`, `HostCanvasTabInput`, `ChatPanelHandle`, `PanelData`, `DashboardData`, `ChartData`, `PanelWidth`, `ObjectDetailData`, `BookmarkPrompt`, `ChatAppShellProps`, `ChatAppShellDestination`, `ChatAppShellHeaderApi`, `ChatAppShellNavApi`, `ChatAppShellDestinationApi`

---

## Backend

This component talks to the `netapp-chat-service` Go backend. See the
[main repo](https://github.com/ebarron/netapp-chat-service) for the API contract,
configuration, and deployment.

---

## Related documents

- [Agentic-forward UI seams (C1–C6)](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-seams.md)
- [Agentic-forward layout modes (C7–C8)](https://github.com/ebarron/netapp-chat-service/blob/main/docs/agentic-forward-layout-modes.md)
- [Mobile layout](https://github.com/ebarron/netapp-chat-service/blob/main/docs/mobile-layout.md)
- [CHANGELOG](./CHANGELOG.md)
