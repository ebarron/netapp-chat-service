# Mobile layout (responsive `ChatAppShell`)

> **Status:** Implemented in `@edjbarron/netapp-chat-component` `0.4.0`
> (tag `chat-component-v0.4.0`). MVP-first; see Future enhancements for deferred
> work. **Audience:** Engineers working on `@edjbarron/netapp-chat-component` and the
> two host products that embed it (NABox, RTB-Platform).
> **Scope:** Component-only (`packages/chat-component`). No engine, no
> `/chat/*` HTTP surface, no `chat-service` binary changes. Additive and opt-in
> — absent the new props, desktop behavior is **byte-for-byte unchanged**.
>
> Builds on [`agentic-forward-seams.md`](./agentic-forward-seams.md) (C1–C6) and
> [`agentic-forward-layout-modes.md`](./agentic-forward-layout-modes.md) (C7–C8).
> Baseline reviewed: `@edjbarron/netapp-chat-component@0.3.1`
> (tag `chat-component-v0.3.1`).

## Why this exists

The component has **no responsive/mobile support**. On a narrow viewport the
canvas is simply removed, and there is no reactive breakpoint, no
single-column mode, and no prop to change any of it. That is fine for a
slide-over `Drawer` assistant, but it breaks the C1–C8 "AI-forward app shell"
use case, where the host renders its **own application pages inside the canvas
host-portal** (C2–C4). When the canvas disappears on mobile, those pages
disappear with it — the app is gone, leaving only the assistant.

Concretely, three things in the current source combine into the bug:

1. **A hard-coded, non-reactive width gate.** In
   `packages/chat-component/src/ChatPanel.tsx` (~383–384) the canvas is gated by
   a one-shot read of `window.innerWidth`:

```380:387:packages/chat-component/src/ChatPanel.tsx
  const dragging = useRef(false);

  // Canvas active when tabs exist and viewport is wide enough.
  const isNarrow = typeof window !== 'undefined' && window.innerWidth < 1024;
  const hasCanvas = canvasTabs.length > 0 && !isNarrow;
  const effectiveWidth = hasCanvas
    ? Math.max(drawerWidth * 2.5, typeof window !== 'undefined' ? window.innerWidth * 0.8 : 1200)
    : drawerWidth;
```

   This is read once at render with no `resize`/`matchMedia` subscription, the
   `1024` threshold is a magic number with no override, and when it trips
   `hasCanvas` becomes `false` for **all** tabs — engine tabs *and* host-portal
   tabs alike.

2. **Host pages live in the canvas region.** `ChatAppShell` portals the host's
   page for the active destination into a mount node that `CanvasPanel` owns
   (`ChatAppShell.tsx` ~439–445 → `CanvasPanel.tsx` `HostTabMount` ~117–131 via
   `onHostTabPortal`). So "no canvas" literally means "no host page":

```439:445:packages/chat-component/src/ChatAppShell.tsx
  const portalNode =
    portalEl && activeDest
      ? createPortal(
          renderDestination({ destination: activeDest, publishSummary, openNav }),
          portalEl,
        )
      : null;
```

3. **The narrow fallback only covers *engine* canvases.** `useChatPanel.ts`
   (~763–778) re-renders engine-emitted canvas content **inline in the chat
   transcript** when narrow — but a host-portal tab has no engine content to
   inline; its content is a React subtree the host owns:

```763:778:packages/chat-component/src/useChatPanel.ts
            // On narrow viewports, render canvas content inline in chat
            // instead of opening a canvas tab (canvas panel is hidden).
            const narrow = typeof window !== 'undefined' && window.innerWidth < 1024;
            if (narrow) {
              const fenceType = tab.content && 'panels' in tab.content ? 'dashboard' : 'object-detail';
              const json = JSON.stringify(tab.content);
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, content: m.content + '\n```' + fenceType + '\n' + json + '\n```\n' }
                    : m
                )
              );
            } else {
              addOrFocusCanvasTab(tab);
            }
```

There is no prop, CSS custom property, or className override to change any of
this. A consumer that portals its app into the canvas therefore **cannot** ship
the component as a full app shell on phones/tablets. Today both known consumers
work around it by mounting a *separate, hand-rolled* shell below 1024px (see
below) — duplicated layout code the component should make unnecessary.

---

## Goals / Non-goals

### Goals (MVP)

- Add a **reactive** (resize/`matchMedia`-aware) breakpoint **within the new
  `mobileLayout` mode**, with a **configurable** threshold (prop; default
  `1024`). The legacy one-shot `window.innerWidth` gate is left unchanged for
  non-adopters (strictly opt-in — [Resolved decisions](#resolved-decisions) #3).
- **Keep the host-portal region mounted at all viewport widths** so consumers
  that portal pages into the canvas never lose their app on mobile. This is the
  core fix.
- Provide a built-in **single-column mobile layout** auto-applied below the
  breakpoint: nav collapses to a burger/drawer, and the user switches between
  the **page/canvas** view and the **assistant** via a 2-item **"Page ·
  Assistant" bottom-tab bar** (one region visible at a time).
- **Persist the active mobile region across reloads** via an opt-in
  `persistMobileRegionKey`, mirroring `persistAssistantCollapsedKey`.
- **A minimal responsive pass on the assistant internals** so the assistant is
  genuinely phone-usable standalone (denser/wrapping header controls,
  collapsible bookmarks) — just enough, since NABox deletes its classic shell on
  landing ([Resolved decisions](#resolved-decisions) #5) and the mobile UX must
  stand alone.
- Be **opt-in and backward compatible**: with none of the new props supplied,
  the shell renders exactly as `0.3.1` does today (including today's
  remove-the-canvas narrow behavior).
- Keep it **generic** — no host routes/pages/app knowledge in the component;
  reusable by any consumer, framed as a possible **4th layout mode** alongside
  the host presets consumers already compose from C7/C8.

### Non-goals (MVP — see Future enhancements)

- No touch-gesture navigation (swipe between page/assistant), no bottom-sheet
  physics, no animated transitions beyond what Mantine gives for free.
- **No re-homing of engine-emitted canvas content.** Engine canvas tabs keep
  falling back to **inline-in-chat when narrow, exactly as `0.3.1` does**
  (ephemeral, scrolls away with the transcript — `useChatPanel.ts` ~763–778 is
  unchanged). The persistent canvas region is reserved for **host-portal
  pages** only; the sole mobile change to the canvas region is keeping those
  host pages mounted (§ item 3 above).
- **No *deeper* responsive tuning** of the assistant's internals beyond the
  minimal MVP pass (see Goals) — richer small-screen layouts are a Future
  enhancement.
- No change to the `variant="drawer"` `ChatPanel` (already a full-height
  slide-over that works on narrow screens).
- No engine/API/binary changes of any kind.

---

## How the two consumers use it today

Both consumers hit the **same** structural problem — they portal their routed
app (`<Outlet/>`) into the canvas host-portal — and both currently solve it by
**not** using the component below 1024px, mounting a bespoke narrow shell
instead. The MVP's job is to make that bespoke narrow shell unnecessary.

### 1. NABox — `naboxd/admin-ui/src/aiForward/`

- `AgenticForwardShell.tsx` is a thin adapter over `ChatAppShell`. Its
  `renderDestination` wraps a react-router `<Outlet/>` — i.e. **every NABox page
  renders inside the canvas host-portal**. Lose the canvas ⇒ lose the whole app.
- `useLayoutPreset.ts` defines 3 presets (`assistant-canvas`, `three-column`,
  `focus`) that map to C7/C8 props; `LayoutPicker.tsx` is the header control.
- `useAiForward.ts` gates the whole forward experience on
  `useMediaQuery('(min-width: 1024px)')`: **below 1024px NABox drops out of the
  forward shell entirely** and renders a *separate classic Mantine `AppShell`*
  (`App.tsx` `classicShell`). That classic shell has **real** mobile support —
  `navbar={{ width: 200, breakpoint: 'sm', collapsed: { mobile: !opened } }}`
  and a `<Burger hiddenFrom="sm" />` — with the assistant as a slide-over
  `ChatPanel` (`variant="drawer"`). **That burger + collapsible-nav UX is the
  bar the built-in mobile mode must meet.** Per
  [Resolved decisions](#resolved-decisions) #5, the adoption plan is **full
  retirement on landing**: once this mode ships, NABox flips `useAiForward` to
  the single `ChatAppShell` at all widths and **deletes the classic Mantine
  `AppShell` immediately** — no transition / dual-shell period. So the MVP
  mobile UX is the outright replacement for classic and must stand alone.

### 2. RTB-Platform — `web/src/aiForward/`

Located at `/Users/ebarron/VSC Projects/RTB-Platform/` (the task's
"RTP-platform" = **RTB-Platform**). Confirmed a live consumer of
`@edjbarron/netapp-chat-component`.

- `RTBAgenticShell.tsx` uses `ChatAppShell` with a `renderDestination` that also
  wraps `<Outlet/>` — **RTB likewise portals its routed pages into the canvas
  host-portal**, so it has the same core exposure as NABox.
- Same 3-preset model as NABox (`web/src/aiForward/useLayoutPreset.ts`,
  per-user persisted).
- **RTB already hand-rolls its own narrow story and does *not* mount
  `ChatAppShell` below 1024px.** `useAiReadiness.ts` returns
  `reason === 'narrow'` under `(min-width: 1024px)`, which routes RTB to a
  bespoke `DegradedFrame` (Tailwind, not the component): a full-width
  `<main><Outlet/></main>` (pages rendered **directly**, not via the canvas
  portal), a `RTBNavOverlay` drawer, and a floating `NarrowAssistantDock` FAB
  that opens a full-width `ChatPanel` (`variant="drawer"`) slide-over. So RTB's
  mobile works today precisely *because* it side-steps the component on narrow.

**Implication for the design.** The MVP must not break RTB's current usage:
RTB can keep its `DegradedFrame` (opt-out by simply not passing the new props),
or later adopt the built-in mobile mode to delete that bespoke code. NABox is
the motivated **first adopter** and will retire its classic shell outright the
moment this lands ([Resolved decisions](#resolved-decisions) #5), so the
built-in mobile UX has to be a complete standalone experience, not a stopgap.
It should resemble the union of what both consumers hand-rolled:
**burger→drawer nav + one-region-at-a-time page/assistant switching (a "Page ·
Assistant" bottom-tab bar) + full-width assistant**, with **pages always
mounted**.

---

## Proposed MVP design

Frame it exactly as the C7/C8 doc frames layout: a small set of generic,
orthogonal, opt-in props on `ChatAppShell` (and the underlying width signal on
`ChatPanel`). Consumers compose them; the component ships behavior, not a
preset vocabulary. Conceptually this is the **4th layout mode** ("mobile"),
except it is **auto-applied by viewport width** rather than user-selected.

### Behavior

Below the configured breakpoint, `ChatAppShell` switches to a **single-column**
arrangement:

- **Nav** becomes a burger-driven overlay `Drawer` regardless of the desktop
  `navMode` (a docked nav column can't fit). The existing header hamburger
  (`ChatAppShellHeaderApi.toggleNav`) already drives this — **no host header
  change required** across the boundary, matching how C7 preserved the header
  API.
- **One region visible at a time**, switched by a built-in 2-item **"Page ·
  Assistant" bottom-tab bar** ([Resolved decisions](#resolved-decisions) #1):
  either
  - the **page/canvas** region (the host-portal + any canvas tabs), or
  - the **assistant**.

  This is the **mobile expression of "view 1" (`assistant-canvas`)**: the
  assistant + canvas panels that sit side-by-side on desktop can't fit on a
  phone, so they become two tappable tabs.
- **The host-portal mount node stays mounted at all widths.** This is the
  central change: instead of unmounting the canvas when narrow, the mobile
  layout keeps the portal target in the tree and hides the *non-active* region
  with CSS (`display:none`/visibility), never by unmounting. That preserves the
  host's page state, the conversation, mode, and any in-flight SSE stream across
  region switches — the same "hide, don't unmount" discipline C8b uses for the
  collapsible assistant.
- **Assistant goes full-width** when it is the visible region (no split, no
  resizer). Resizing/placement/collapse props (C1/C8) are wide-viewport-only
  and are simply inert below the breakpoint.
- **Canvas tabs still work** within the page/canvas region (the tab strip
  renders as today, subject to `hideSingleTab`). The reserved nav tab / host
  page renders in the same region.

Default region on first entering mobile: the **page/canvas** region
([Resolved decisions](#resolved-decisions) #4), so a consumer whose app lives in
the portal shows the app, not an empty assistant; the assistant is opened **on
demand** by tapping its bottom tab. Switching to a destination via nav or
`open_nav` should surface the page/canvas region; tapping the Assistant tab
brings the assistant forward. When `persistMobileRegionKey` is set, the last
active region is restored on reload (SSR-safe, in an effect — same convention as
`persistAssistantCollapsedKey`).

### Responsive assistant internals (minimal MVP pass)

Because NABox retires its classic shell on landing
([Resolved decisions](#resolved-decisions) #5), the assistant must be usable
standalone on a phone — not merely stretched full-width. The MVP includes a
**minimal** pass: assistant-header controls (mode toggle, bookmarks, settings,
clear) wrap/condense instead of overflowing, and the bookmark list is
collapsible so it doesn't crowd the transcript. This is deliberately just
enough; deeper small-screen tuning (message-bubble density, richer layouts) is a
Future enhancement.

### Reactive breakpoint

When `mobileLayout` is on, replace the one-shot read at `ChatPanel.tsx` ~383
with a resize-aware signal (a small internal
`useMediaQuery(`(min-width: ${breakpoint}px)`)`-style hook, mirroring the pattern
already used consumer-side in NABox `useAiForward.ts` and RTB
`useAiReadiness.ts`). The reactive signal is **strictly opt-in behind
`mobileLayout`** ([Resolved decisions](#resolved-decisions) #3): with
`mobileLayout` off, the legacy one-shot gate is left exactly as `0.3.1` ships it
(no change for non-adopters). When on, the signal must:

- subscribe to `matchMedia` `change` (fall back to a `resize` listener), so
  rotating a device or resizing a window flips the layout live;
- be **SSR-safe** — treat only an *explicit* `matches === false` as narrow, and
  default to wide when `matchMedia`/`window` is unavailable (again mirroring the
  consumers), so first paint under SSR/jsdom is unchanged.

### Minimal new API surface (design level)

Additive props on `ChatAppShell` (threaded to `ChatPanel`/`CanvasPanel` as
needed). Names are illustrative; the shape is what matters.

```ts
// ChatAppShell (and mirrored on ChatPanel where relevant)

/**
 * Enables the built-in single-column mobile layout below `mobileBreakpoint`.
 * Default: false — with this off, today's 0.3.1 narrow behavior (remove the
 * canvas; engine canvases inline in chat) is preserved byte-for-byte.
 */
mobileLayout?: boolean;

/**
 * Viewport width (px) at/below which the mobile layout applies. Default: 1024.
 * Only consulted when mobileLayout is on; the legacy narrow gate is untouched
 * for non-adopters (see Resolved decisions #3).
 */
mobileBreakpoint?: number;

/**
 * Which region is shown first when entering mobile: 'canvas' (host page /
 * canvas — default) or 'assistant'. Default: 'canvas' (Resolved decisions #4).
 */
mobileDefaultRegion?: 'canvas' | 'assistant';

/**
 * Style of the page↔assistant switch. The MVP ships and defaults to
 * 'bottom-tab' (a 2-item "Page · Assistant" bar — the mobile expression of the
 * assistant-canvas view). 'toggle' (a single header button) remains an
 * available alternative. Default: 'bottom-tab' (Resolved decisions #1).
 */
mobileRegionSwitch?: 'bottom-tab' | 'toggle';

/** Optional controlled/uncontrolled + notifier for the active mobile region. */
mobileRegion?: 'canvas' | 'assistant';
defaultMobileRegion?: 'canvas' | 'assistant';
onMobileRegionChange?: (region: 'canvas' | 'assistant') => void;

/**
 * Persist the active mobile region under this localStorage key so it survives
 * reloads (opt-in; mirrors persistAssistantCollapsedKey). Omit for ephemeral,
 * default-region-on-load behavior. SSR-safe restore in an effect.
 */
persistMobileRegionKey?: string;
```

Notes:

- **Only `mobileLayout` changes anything.** With it `false` (default), the
  component behaves as `0.3.1` in every respect — including the legacy one-shot
  narrow gate, which is **left unchanged** for non-adopters
  ([Resolved decisions](#resolved-decisions) #3). The reactive breakpoint and
  every other new behavior are reachable **only** by turning on `mobileLayout`.
- No new engine/wire types. `canvas_tabs`, `open_nav`, `CanvasTabSummary`, etc.
  are untouched.
- The controlled/uncontrolled `mobileRegion` trio follows the exact convention
  already established for `assistantCollapsed`/`assistantWidth`, so consumers
  wire it the same way; `persistMobileRegionKey` provides built-in reload
  persistence for the uncontrolled case (mirroring
  `persistAssistantCollapsedKey`).

### Where it lives in the code (design intent, not implementation)

- `ChatPanel.tsx` ~383: **only when `mobileLayout` is on**, swap the one-shot
  `isNarrow` for the reactive hook fed by `mobileBreakpoint` and **stop** letting
  `isNarrow` zero out `hasCanvas` for host-portal tabs — keep the portal region
  mounted and toggle visibility by the active region instead. With `mobileLayout`
  off, this line is left exactly as-is (the legacy non-reactive gate).
- `ChatAppShell.tsx`: when mobile, force nav to overlay, render the **"Page ·
  Assistant" bottom-tab bar**, and CSS-hide the inactive region while keeping
  both in the tree (the `portalNode` from ~439–445 — the **host-portal page** —
  must remain rendered at all widths).
- `useChatPanel.ts` ~763–778: **unchanged in the MVP.** Engine-emitted canvas
  tabs still fall back to **inline-in-chat when narrow, exactly as `0.3.1`
  does** — that content stays ephemeral in the transcript and is *not* re-homed
  into the persistent canvas region (which is host-portal pages only).
  Re-homing engine canvases is a deferred Future enhancement.

> **Implementer invariant — keep `HostTabMount` MOUNTED, don't just keep its
> state.** The persistent-region requirement is specifically that the
> host-portal mount node (`HostTabMount` in `CanvasPanel.tsx`) stays **mounted**
> at all widths — *not* merely that `canvasTabs[].summary` state is retained.
> Unmounting the portal while keeping the state would still tear down the host
> page's **mount-scoped context-publishing effects** (e.g. the consumer's
> `useNavSummary`/`publishSummary`), silently stopping live per-page context
> updates to the chatbot even though a stale/identity summary might still be
> sent. So: keep `HostTabMount` mounted and toggle visibility via **CSS only** —
> never gate its mount on `hasCanvas`/narrow. Payoff: because the
> `/chat/message` `canvas_tabs` payload is assembled from hook state (not the
> DOM) and isn't visibility-gated, a mounted-but-hidden portal keeps **fresh**
> published context flowing — fixing today's narrow behavior where unmounting
> the portal stops live publishes.

---

## Backward compatibility

- **Default off.** With none of `mobileLayout` / `mobileBreakpoint` /
  `mobileDefaultRegion` / `mobileRegionSwitch` / `mobileRegion*` /
  `persistMobileRegionKey` supplied, the DOM and behavior match `0.3.1` exactly,
  including today's "remove canvas when narrow" and the engine inline-canvas
  fallback. `persistMobileRegionKey` is itself opt-in: absent the key, the region
  is ephemeral (resets to `mobileDefaultRegion` on load). This should be guarded
  by a "no new props" snapshot test, matching the C1–C8 backward-compat
  discipline.
- **Component-only.** Confined to `packages/chat-component`. `agent/`,
  `server/`, the `/chat/*` contract, `canvas_tabs`, `open_nav`, and the
  `chat-service` binary are untouched; a host can adopt this while keeping its
  deployed engine exactly as-is.
- **`ChatPanel` drawer variant unchanged.** The slide-over assistant is already
  narrow-friendly and is not touched.
- **RTB is unaffected until it opts in.** RTB keeps its `DegradedFrame` on
  narrow simply by not passing the new props; if/when it adopts `mobileLayout`
  it can delete that bespoke frame.
- **Release.** Purely additive props ⇒ a minor bump (e.g. `0.3.1` → `0.4.0`).
  Because the legacy narrow gate is untouched for non-adopters
  ([Resolved decisions](#resolved-decisions) #3), there is no behavior-change
  wrinkle to flag — the changelog simply documents the new opt-in
  `mobileLayout` mode.

---

## Resolved decisions

The owner has locked in the following. They are reflected inline throughout the
design above; captured here for traceability.

1. **Region switch = `bottom-tab` (MVP).** The page↔assistant switch is a 2-item
   **"Page · Assistant" bottom-tab bar**, not a single header toggle. Rationale:
   it is the **mobile expression of "view 1" (`assistant-canvas`)** — the
   assistant + canvas panels that sit side-by-side on desktop can't fit on a
   phone, so they become two tappable tabs. `mobileRegionSwitch` defaults to
   `'bottom-tab'`; `'toggle'` remains an available alternative but is no longer
   the default. (Swipe gestures between tabs stay deferred — see Future
   enhancements.)
2. **Breakpoint default = `1024px`.** `mobileBreakpoint` defaults to `1024`,
   matching both consumers' current threshold.
3. **Legacy narrow gate is strictly opt-in behind `mobileLayout`.** The MVP does
   **not** touch the current non-reactive `window.innerWidth` gate for
   non-adopters; with `mobileLayout` off, behavior is `0.3.1` byte-for-byte. The
   reactive breakpoint only applies when `mobileLayout` is on. (Fixing the legacy
   gate globally is listed under Future enhancements.)
4. **Default region on activation = `canvas`/page.** `mobileDefaultRegion`
   defaults to the **page/canvas** region so the host's app is visible first; the
   assistant is opened on demand by tapping its bottom tab.
5. **NABox classic-shell retirement = full retirement on landing.** Once this
   mobile mode ships in the component, NABox flips `useAiForward` to the single
   `ChatAppShell` and **deletes the classic Mantine `AppShell` immediately** —
   there is no transition / dual-shell period. Consequently the MVP mobile UX is
   the **outright replacement** for classic and must be good enough to stand
   alone (burger→drawer nav, "Page · Assistant" bottom-tab switch, full-width
   assistant with the minimal responsive-internals pass). See
   [How the two consumers use it today](#how-the-two-consumers-use-it-today).

No open questions remain.

---

## Future enhancements (post-MVP)

- **Re-home engine canvases on mobile:** *deferred.* In the MVP, engine-emitted
  canvas content keeps rendering **inline in the chat transcript when narrow,
  exactly as `0.3.1` does** — no change from today. A later enhancement could
  route engine canvases into the persistent canvas region (auto-switching to the
  canvas tab) instead of inlining JSON fences (`useChatPanel.ts` ~763–778). Not
  in the MVP, whose only canvas-region change is keeping **host-portal pages**
  mounted.
- **Swipe gestures** between the Page and Assistant tabs (the MVP ships the
  tap-only `bottom-tab` bar).
- **Fix the legacy narrow gate globally** — make the non-reactive
  `window.innerWidth` gate at `ChatPanel.tsx` ~383 reactive (and
  `mobileBreakpoint`-driven) for *all* consumers, not just `mobileLayout`
  adopters. Deferred out of the MVP so non-adopters see zero change
  ([Resolved decisions](#resolved-decisions) #3).
- **Assistant as bottom sheet / partial-height overlay** over the page, instead
  of a full-region swap, for quick "ask about what I'm looking at" flows.
- **Container queries** instead of a viewport media query, so the layout reacts
  to the shell's own width when embedded in a narrower container.
- **Further responsive assistant tuning beyond the minimal MVP pass** — e.g.
  message-bubble density, richer small-screen layouts (the MVP already condenses
  header controls and makes bookmarks collapsible).
- **Multiple named mobile presets** if consumers diverge (kept in the host, per
  the C7/C8 philosophy).
