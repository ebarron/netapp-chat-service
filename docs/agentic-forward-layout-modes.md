# Agentic-Forward UI layout modes (C7–C8)

> **Status:** implemented in `@edjbarron/netapp-chat-component` `0.3.0`
> (C7–C8 landed in `0.2.2`; `0.3.0` adds optional header title + `hideSingleTab`).
> C7–C8 and `hideSingleTab` are additive/opt-in. **Header title is a behavior
> change:** `title` no longer defaults to `"AI Assistant"` — hosts that want a
> title bar must pass `title` (and/or `subtitle`) explicitly. **Component-only**
> — the Go engine, the `/chat/*` HTTP surface, and the `chat-service` binary are
> not touched.
>
> Extends [`agentic-forward-seams.md`](./agentic-forward-seams.md) (C1–C6).
> Companion host spec: `RTB-Platform/specs/rtb/rtb-108-agentic-shell-layout-control.md`.

## Why this exists

The C1 docked shell renders exactly one arrangement: a persistent assistant
column on the **left**, a canvas filling the rest, and navigation as a
hamburger-driven **overlay** (`Drawer`). Hosts have asked for user-selectable
arrangements on top of that same shell:

1. Navigation that can **stay docked** as a persistent left column instead of an
   overlay.
2. An assistant column the user can **collapse** (and reveal again) so the
   screen becomes just navigation + canvas.
3. When navigation and the assistant are both docked, the ability to place the
   assistant on the **right** (nav left · canvas middle · chat right).

These are presentation choices. Keeping with the seams philosophy — *the
component hosts a hole, not app knowledge* — the component exposes three small,
**orthogonal** layout props and the host composes them into whatever named
"layout presets" it wants. The component ships no preset vocabulary.

| # | Seam | Where | Opt-in via |
|---|------|-------|-----------|
| C7 | Docked navigation column (vs. overlay) | component | `navMode="docked"` |
| C8a | Assistant placement (left/right of canvas) | component | `assistantPlacement="end"` |
| C8b | Collapsible assistant + reveal rail | component | `assistantCollapsed` / `defaultAssistantCollapsed` |
| — | Hide canvas tab strip when only one tab is open | component | `hideSingleTab` |

All three C7/C8 seams are independent; any combination is valid. The host's
presets are just points in this 3-axis space. `hideSingleTab` is orthogonal to
placement/collapse/nav mode.

---

## Header title (docked / full-page)

`ChatPanel` / `ChatAppShell` `title?: string` is truly optional — **there is no
default**. In docked and full-page variants the full-width header bar
(`fullPageHeader`: chatbot icon + title + optional subtitle) renders **only**
when `title` and/or `subtitle` is provided. When neither is set, the
assistant+canvas region fills the full height and nothing spans above the
canvas tabs. Panel controls (mode, bookmarks, settings, clear, hide-assistant)
live in the assistant column and are unaffected.

Drawer variant: when `title`/`subtitle` are absent the title/icon group is
omitted, but the Drawer's close button remains.

> **Migration from ≤0.2.x:** hosts that relied on the implicit `"AI Assistant"`
> default must pass `title` explicitly if they still want a header bar.

---

## Hide single-tab strip (`hideSingleTab`)

```ts
hideSingleTab?: boolean; // default: false
```

Threaded `ChatAppShell` → `ChatPanel` → `CanvasPanel`. When `true` and exactly
one canvas tab is open, the Mantine `Tabs.List` (the tab chrome) is not
rendered — there is no `[role="tablist"]` — but the tab's `Tabs.Panel` content
still shows. With two or more tabs the strip appears as usual; dropping back to
one tab hides it again. Works for the reserved nav tab and engine tabs alike.
Default `false` preserves today's always-visible strip.

---

## C7 — Docked navigation column

`<ChatAppShell>` gains one optional prop:

```ts
navMode?: 'overlay' | 'docked';        // default: 'overlay'
navDockedWidth?: number;               // px; default = navOverlayWidth (260)
navOpen?: boolean;                     // controlled nav-open (0.4.1)
onNavOpenChange?: (open: boolean) => void; // (0.4.1)
```

- **`navMode="overlay"` (default)** — the hamburger `toggleNav` opens/closes the
  left `Drawer`; `renderNavMenu` is rendered inside it; `navOverlayTitle` /
  `navOverlayWidth` apply.
- **`navMode="docked"`** — `renderNavMenu` is rendered as a **persistent
  left column** (width `navDockedWidth`) in the body flex row, before the
  assistant/canvas split. The `Drawer` is not mounted.

**Live switching (0.4.1).** `navMode` may be changed at runtime as a normal prop.
The shell re-syncs nav-open on a real `overlay`↔`docked` transition (open for
docked, closed for overlay; a user's manual collapse/expand within a mode is not
overridden) and renders both modes from **one** flex-row structure so the
assistant + reserved-nav portal keep a stable tree position. React therefore
preserves the `ChatPanel` instance — chat `messages`, `sessionId`, and open
canvas / host-portal tabs survive the switch, so hosts no longer need a remount
`key`. To keep this guarantee the desktop **overlay** body now shares the docked
row/column wrapper (one extra flex nesting vs. 0.4.0); the visual result is
unchanged. Supply the optional controlled `navOpen` / `onNavOpenChange` pair to
own the open state yourself (this bypasses the auto re-sync).

### Header API is unchanged

The `renderHeader` slot still receives the same `ChatAppShellHeaderApi`
(`navOpened`, `toggleNav`, `closeNav`, `openNav`) so a host banner's hamburger
keeps working with **no code change** across modes:

- In `overlay` mode, `navOpened` reflects the Drawer's open state (as today).
- In `docked` mode, `navOpened` reflects whether the docked column is
  **expanded** (`true`) or **rail-collapsed** (`false`); `toggleNav` toggles it.
  This lets the same hamburger collapse/expand the docked rail.

`ChatAppShellNavApi` handed to `renderNavMenu` is unchanged. In docked mode
`openNav` no longer force-closes the column (there is nothing to close); it
still opens the destination and syncs the URL exactly as before.

> **Reserved nav tab is orthogonal.** C7 only changes how the host's *menu*
> (`renderNavMenu`) is presented. The reserved **nav canvas tab** (C2–C4, where
> the selected destination's page is portaled) is unaffected — the menu picks a
> destination; the destination still renders in the canvas.

---

## C8 — Assistant placement and collapse

### C8a — Placement

```ts
assistantPlacement?: 'start' | 'end'; // default: 'start'
```

The docked assistant↔canvas split is a flex row driven by
`--chat-assistant-width` (see C1 "Configurable and resizable split"). Placement
sets the flex order only:

- **`'start'` (default)** — assistant before canvas (left), as today.
- **`'end'`** — assistant after canvas (right).

Width, min/max, resizing (`resizableAssistant`), and persistence
(`persistAssistantWidthKey`) are placement-agnostic and behave identically. The
drag separator stays between the two columns and clamps the same way.

### C8b — Collapse + reveal rail

```ts
assistantCollapsed?: boolean;              // controlled
defaultAssistantCollapsed?: boolean;       // uncontrolled initial
onAssistantCollapsedChange?: (collapsed: boolean) => void;
persistAssistantCollapsedKey?: string;     // localStorage key (SSR-safe restore)
```

- When **collapsed**, the assistant column is removed from the split so the
  canvas (and docked nav, if any) take the full width. A slim, focusable
  **reveal rail** (icon button, `aria-label="Show assistant"`,
  `aria-expanded={false}`) is pinned to the assistant's side
  (`assistantPlacement`). Activating it expands the assistant.
- When **expanded**, the assistant renders normally; a collapse affordance in
  the assistant header (`aria-label="Hide assistant"`) collapses it.
- Controlled/uncontrolled follows the same convention as `assistantWidth`:
  pass `assistantCollapsed` to control it, or `defaultAssistantCollapsed` to let
  the shell own it. `onAssistantCollapsedChange` fires on every toggle;
  `persistAssistantCollapsedKey`, when set, stores the boolean and restores it
  in an effect (SSR-safe).
- **Chat state survives collapse.** Collapsing hides the column (CSS), it does
  not unmount `ChatPanel` — the conversation, mode, and in-flight stream are
  preserved, and background SSE handling continues.

> **Narrow viewport.** The existing `<1024px` behavior (C1) is unchanged and
> takes precedence: the split can't fit, so the canvas goes full width and the
> assistant is reached via the host's narrow affordance regardless of C7/C8
> props. Hosts should treat C7/C8 as wide-viewport presentation.

---

## Composed examples

```tsx
// Default (current) — 2 columns: assistant left, canvas; nav overlay.
<ChatAppShell /* no navMode/placement/collapsed */ … />

// 3 columns — nav left · canvas middle · chat right.
<ChatAppShell
  navMode="docked"
  assistantPlacement="end"
  …
/>

// Nav + canvas, chat tucked away behind a reveal rail on the right.
<ChatAppShell
  navMode="docked"
  assistantPlacement="end"
  defaultAssistantCollapsed
  persistAssistantCollapsedKey="host.assistantCollapsed"
  …
/>
```

---

## Backward compatibility

With **none** of `navMode`, `navDockedWidth`, `assistantPlacement`,
`assistantCollapsed`, `defaultAssistantCollapsed`,
`onAssistantCollapsedChange`, or `persistAssistantCollapsedKey` supplied, the
shell renders exactly as the C1 shell does today: assistant left, canvas right,
hamburger overlay nav — byte-for-byte identical DOM and behavior.

- **No engine / API / binary change.** This is confined to
  `packages/chat-component`. `agent/`, `server/`, the `/chat/*` HTTP contract,
  `canvas_tabs`, `open_nav`, and the `chat-service` binary are untouched. A host
  can adopt these props while keeping its deployed `chat-service` (v0.2.x)
  exactly as-is.
- **`ChatPanel` unchanged for existing callers.** C8a/C8b are implemented on the
  docked layout; drawer variant and all current docked callers that omit the new
  props are unaffected.

### Release

- Component: minor bump `0.2.2` → **`0.3.0`** (`hideSingleTab` additive;
  removing the default `"AI Assistant"` title is a behavior change).
- Consumers on `^0.2.0` will not auto-pick `0.3.0`; bump the pin (or widen the
  range) and refresh the lockfile. Hosts that want the old title bar must pass
  `title` explicitly; hosts that want a clean single-tab canvas pass
  `hideSingleTab`.

### Test plan (mirrors C1–C6 discipline)

- `ChatAppShell.test.tsx`: default render unchanged (no new props → overlay,
  assistant-left DOM); `navMode="docked"` renders the menu as a persistent
  column and mounts no `Drawer`; `toggleNav` toggles docked expand/collapse and
  drives `navOpened`.
- Placement: `assistantPlacement="end"` orders the assistant after the canvas;
  width/resize/persist behave identically to `'start'`.
- Collapse: controlled + uncontrolled toggle; reveal rail has the documented
  roles/labels; `onAssistantCollapsedChange` fires; `persistAssistantCollapsedKey`
  round-trips; `ChatPanel` is not unmounted while collapsed (chat state kept).
- A byte-for-byte "no new props" snapshot test guarding the default path (as in
  the C1–C6 backward-compat suite).
