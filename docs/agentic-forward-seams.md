# Agentic-Forward UI seams (C1–C6)

> **Status:** implemented, additive, opt-in. Every seam below defaults to the
> prior behavior, so existing consumers of `github.com/ebarron/netapp-chat-service`
> and `@edjbarron/netapp-chat-component` are unaffected until they opt in.

This document describes four generic, backward-compatible seams added so a host
can build an "AI-forward" workspace — a persistent docked assistant beside a
canvas that also hosts the host's own pages, with everything on screen exposed
to the assistant's context and navigation drivable by prompt.

The seams are **generic**: the chat-service hosts *a hole, not app knowledge*.
No host routes, pages, or destinations are hardcoded anywhere in the engine or
component.

| # | Seam | Where | Opt-in via |
|---|------|-------|-----------|
| C1 | Docked / full-window assistant mode | component | `variant="docked"` |
| C2–C4 | Host content in a canvas tab (portal slot) | component | `ChatPanelHandle.openHostCanvasTab` + `onHostTabPortal` |
| C5 | Canvas context provider (summary + `digest`) | component + engine | `CanvasTabSummary` on a tab → `canvas_tabs` |
| C6 | Open-nav-view / navigation-by-prompt | engine + component | `agent.NewOpenNavTool()` → `open_nav` SSE → `onOpenNav` |

---

## C1 — Docked vs. drawer mode

`<ChatPanel>` gains an optional prop:

```ts
variant?: 'drawer' | 'docked'; // default: 'drawer'
```

- **`variant="drawer"` (default)** — today's left slide-over `Drawer`,
  byte-for-byte unchanged. Gated by `opened` exactly as before.
- **`variant="docked"`** — a **persistent, full-height** panel that fills its
  parent: the assistant column on the left and (when canvas tabs are open) the
  canvas filling the remaining width. Intended to sit under a host-provided
  full-width header. In docked mode the panel is **always present** — it is not
  gated by `opened`, and it fetches config/capabilities on mount.

```tsx
<div className="app-shell">
  <MyFullWidthHeader />
  <ChatPanel variant="docked" onClose={() => {}} opened />
</div>
```

Everything else (messages, mode toggle, capabilities, canvas split, narrow-
viewport behavior) is identical across variants. The empty canvas stays hidden
until a tab opens.

### Configurable and resizable split

`ChatAppShell` forwards these optional props to its docked `ChatPanel`:

```ts
assistantWidth?: number | string;       // controlled; number = px
defaultAssistantWidth?: number | string; // uncontrolled initial width
assistantMinWidth?: number;             // default 320px
assistantMaxWidth?: number;
resizableAssistant?: boolean;           // default false
onAssistantWidthChange?: (px: number) => void;
persistAssistantWidthKey?: string;
```

`assistantWidth` accepts pixels as a number or any CSS length/percentage as a
string. It is applied through the inherited `--chat-assistant-width` custom
property; the canvas flexes into the remaining space. Hosts may set that custom
property directly in CSS instead of overriding hashed CSS-module class names.

With `resizableAssistant`, a focusable vertical separator supports mouse/touch
Pointer Events, Left/Right arrow nudging (16px, or 64px with Shift), and
double-click reset. Dragged widths are clamped to the configured pixel limits and
15%–60% of the split container so the canvas remains visible.
`onAssistantWidthChange` fires with the clamped pixel width during resizing and
again at drag end. When `persistAssistantWidthKey` is present, user sizing is
stored under that exact `localStorage` key and restored in an effect, making the
read SSR-safe.

Omitting every split prop preserves the original DOM behavior: no separator and
the same 40% assistant / 60% canvas split.

---

## C2–C4 — Host content in a canvas tab (portal slot)

The component can host **arbitrary host-provided React content** in a canvas
tab **without importing host code**. It renders an empty mount node and exposes
it; the host portals its own tree in with `ReactDOM.createPortal`. This keeps
the host page inside the host's own React tree (its providers, router, theme),
while it visually lives in the canvas.

### API

Obtain the imperative handle with a ref:

```ts
export interface ChatPanelHandle {
  openHostCanvasTab(input: HostCanvasTabInput): void;
  updateHostCanvasTab(tabId: string, patch: Partial<Omit<HostCanvasTabInput, 'tabId'>>): void;
  setCanvasTabSummary(tabId: string, summary: CanvasTabSummary): void;
  closeCanvasTab(tabId: string): void;
  focusCanvasTab(tabId: string): void;
}

export interface HostCanvasTabInput {
  tabId: string;
  title: string;
  kind?: string;       // default 'host'
  qualifier?: string;
  evictable?: boolean; // false = exempt from max-tab FIFO eviction (nav tab)
  summary?: CanvasTabSummary; // C5 context (see below)
}
```

Portal mount callback (prop on `<ChatPanel>`):

```ts
onHostTabPortal?: (tabId: string, el: HTMLElement | null) => void;
// el = the mount node when the tab mounts; null when it unmounts.
```

### Usage

```tsx
const chat = useRef<ChatPanelHandle>(null);
const [mounts, setMounts] = useState<Record<string, HTMLElement | null>>({});

<ChatPanel
  ref={chat}
  variant="docked"
  opened
  onClose={() => {}}
  onHostTabPortal={(tabId, el) =>
    setMounts((m) => ({ ...m, [tabId]: el }))
  }
/>

// Open the reserved, eviction-exempt nav tab and render a host page into it:
chat.current?.openHostCanvasTab({
  tabId: 'nav',
  title: 'Alerting',
  kind: 'nav-view',
  qualifier: '/alerting',
  evictable: false,
  summary: { /* C5, see below */ },
});

// Elsewhere in the host's render tree:
{mounts['nav'] && createPortal(<AlertingPage />, mounts['nav'])}
```

### Tab lifecycle

- **Dedup by `tabId`.** Re-opening the same `tabId` replaces the tab's identity
  and refocuses it — this is how a single reused **nav** tab works
  (`tabId: 'nav'`): each nav selection targets the same tab.
- **Eviction-exempt-but-closable.** A tab opened with `evictable: false` is
  never auto-evicted by the max-5 FIFO rule, but the user can still close it
  manually.
- **Hidden at zero tabs.** When the last tab closes (including the nav tab), the
  canvas region collapses exactly as today; the docked assistant remains.
- Host tabs and engine-driven declarative tabs (`canvas_open`) coexist in the
  same tab strip.

> **Trade-off:** portal'd content is opaque to the LLM — which is exactly why
> **C5 is mandatory** for host tabs you want the assistant to reason about.

---

## C5 — Canvas context provider (`CanvasTabSummary` + `digest`)

So the assistant can answer questions about **anything on screen** (including
opaque host pages), the host attaches a per-tab **context summary**. The
component relays it in the **existing** `canvas_tabs` field of `POST /chat/message`;
the engine renders it into the system prompt's Canvas Context section. The
engine never introspects DOM/React — it is a dumb pipe.

### Shape

Component (`CanvasTabSummary`) and engine (`agent.CanvasTabSummary`) share the
same shape:

```ts
export interface CanvasTabSummary {
  kind?: string;
  name?: string;
  qualifier?: string;
  status?: string;
  key_properties?: Record<string, string>;
  digest?: string; // NEW: free-text summary for pages that don't fit key/values
}
```

```go
type CanvasTabSummary struct {
    TabID         string            `json:"tab_id"`
    Kind          string            `json:"kind"`
    Name          string            `json:"name"`
    Qualifier     string            `json:"qualifier"`
    Status        string            `json:"status,omitempty"`
    KeyProperties map[string]string `json:"key_properties,omitempty"`
    Digest        string            `json:"digest,omitempty"` // NEW
}
```

### Behavior

- Attach via `openHostCanvasTab({ ..., summary })`, `updateHostCanvasTab(tabId,
  { summary })`, or `setCanvasTabSummary(tabId, summary)`.
- **Absent/empty fields are omitted cleanly** — no `null`/empty noise on the
  wire. A tab with no attached summary contributes only its identity
  (`tab_id`/`kind`/`name`/`qualifier`), exactly as before.
- On the engine side, when at least one tab supplies a non-empty `digest`, an
  "Additional detail" block is appended after the Canvas Context table. When no
  tab has a digest, the prompt is **byte-for-byte identical** to the prior
  release.
- **Secrets must be excluded by the host** before building the summary/digest.

---

## C6 — Open-nav-view seam (navigation-by-prompt)

Navigation-by-prompt rides the **existing interest + `ExtraTools` + SSE** rails
already proven by render tools. There is **no new interest type and no parallel
path** — just a normal interest, a normal host-registered internal tool, and one
new SSE event.

### Engine

Register the generic `open_nav_view` tool via `ExtraTools`:

```go
deps.ExtraTools[agent.OpenNavToolName] = agent.NewOpenNavTool()
// agent.OpenNavToolName == "open_nav_view"
```

- **Tool:** `open_nav_view({ "destination": string })` — `destination` is a
  **required, opaque, host-defined** string (a route or a stable screen id).
  The engine hardcodes no destinations.
- When the LLM calls it, the tool returns a short confirmation as the tool
  result **and** emits a new agent event `EventOpenNav{ Destination }`, which the
  server relays as an SSE event:

  ```
  event: open_nav
  data: {"destination":"<opaque>"}
  ```

  This serializes alongside existing events without changing any existing event
  shape.

The generic mechanism behind this is a new optional field on `InternalTool`:

```go
// Emit, when non-nil, is called with the tool input after Handler succeeds;
// the returned Events are relayed to the SSE stream verbatim. nil = today's
// behavior (no side-channel events).
Emit func(input json.RawMessage) []Event
```

Any host tool can use `Emit` to surface a lightweight side-channel signal.

### Parameterized navigation interest

Navigation-by-prompt is driven by **one parameterized navigation interest**
(not one per destination). Its body enumerates a destination catalog and tells
the LLM to pick a destination from the user's phrasing and call
`open_nav_view(destination)`. Because it is capability-independent, it can be
**ungated** — `requires` is now **optional** in an interest file; an interest
with no `requires` is always available (existing gated interests are
unaffected).

```markdown
---
id: navigation
name: Navigate to a screen
source: builtin
triggers:
  - open
  - go to
  - bring up
  - navigate to
  - show me the
---

The user wants to navigate to a screen. Pick the matching destination from the
catalog below based on the user's phrasing and call `open_nav_view(destination)`
with that destination string.

| Screen   | Destination |
|----------|-------------|
| Overview | overview    |
| Settings | settings    |
| Reports  | reports     |
| Profile  | profile     |
```

### Component

`<ChatPanel>` gains an optional handler prop:

```ts
onOpenNav?: (destination: string) => void;
```

The component parses the `open_nav` SSE event and calls `onOpenNav(destination)`.
With **no handler registered it is a safe no-op** (no crash). A host typically
handles it by opening the destination in the reused nav canvas tab (C2–C4) —
making a typed "open alerting" indistinguishable from a manual nav selection.

---

## Backward compatibility

With **none** of the above used — no `variant`, no host tabs, no summaries, no
`digest`, no `open_nav_view` registered — the component and engine behave
byte-for-byte as the prior release. This is covered by explicit tests in both
suites (see `agent/opennav_test.go`, `server/opennav_test.go`,
`interest/navigation_test.go`, and the component's `*.test.ts(x)` for each seam).
