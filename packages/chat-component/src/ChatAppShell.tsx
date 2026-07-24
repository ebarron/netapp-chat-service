import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';
import { Box, Drawer } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { ChatAPIProvider } from './ChatAPIContext';
import type { ChatAPI } from './ChatAPI';
import { ChatPanel, type ChatPanelHandle } from './ChatPanel';
import type { BookmarkPrompt } from './BookmarkPrompts';
import type {
  CanvasEventInfo,
  CanvasTabSummary,
  ChatMode,
} from './useChatPanel';

/**
 * A navigation destination the shell can open in the reserved nav canvas tab.
 * `id` is a stable identifier; `label` is the tab title; `route` is an opaque,
 * host-defined string (usually a URL path) used as the tab qualifier and as the
 * value the shell resolves an `open_nav` event / hamburger selection against.
 * The shell never interprets `route` beyond matching — routing stays host-side.
 */
export interface ChatAppShellDestination {
  id: string;
  label: string;
  route: string;
}

/** API handed to the host's header slot so its hamburger/trigger can drive nav. */
export interface ChatAppShellHeaderApi {
  /** Whether the nav overlay is currently open. */
  navOpened: boolean;
  /** Toggle the nav overlay (wire this to the host's hamburger). */
  toggleNav: () => void;
  /** Close the nav overlay. */
  closeNav: () => void;
  /** Open a destination by id or route in the reserved nav tab + sync the URL. */
  openNav: (idOrRoute: string) => void;
}

/** API handed to the host's nav-menu slot (rendered inside the overlay). */
export interface ChatAppShellNavApi {
  /** Open a destination by id or route (also closes the overlay). */
  openNav: (idOrRoute: string) => void;
  /** Close the nav overlay. */
  closeNav: () => void;
  /** The id of the destination currently shown in the nav tab, or null. */
  activeDestinationId: string | null;
  /** The full destination catalog passed to the shell. */
  destinations: ChatAppShellDestination[];
}

/** API handed to `renderDestination` so a host page can drive nav + publish C5. */
export interface ChatAppShellDestinationApi {
  /** The destination whose page should be rendered into the canvas hole. */
  destination: ChatAppShellDestination;
  /**
   * Publish (or clear→identity) the C5 context summary for the current nav tab
   * so the assistant can answer questions about what's on screen. Pass null to
   * reset to the `buildSummary` identity summary. Secrets MUST be excluded.
   */
  publishSummary: (summary: CanvasTabSummary | null) => void;
  /** Open another destination by id or route. */
  openNav: (idOrRoute: string) => void;
}

export interface ChatAppShellProps {
  // --- Chat wiring (forwarded to the docked ChatPanel) ---
  /**
   * The ChatAPI. When provided, the shell wraps its subtree in a
   * ChatAPIProvider. Omit it if the host already provides one higher up.
   */
  chatAPI?: ChatAPI;
  /** Optional assistant title (ChatPanel). No default — omit for no header bar. */
  title?: string;
  /** Optional assistant subtitle badge (ChatPanel). */
  subtitle?: string;
  /** Initial chat mode. */
  defaultMode?: ChatMode;
  /** Suggested prompts for the empty state. */
  suggestedPrompts?: string[];
  /** Bookmark prompts grouped by MCP. */
  bookmarkPrompts?: BookmarkPrompt[];
  /** Host-driven prompt injection (auto-send once). */
  pendingPrompt?: string;
  /** Called after `pendingPrompt` is submitted. */
  onPromptConsumed?: () => void;
  /** Busy-state notifier for the host. */
  onBusyChange?: (busy: boolean) => void;
  /**
   * Canvas open/close notifier. The shell handles the reserved nav tab's manual
   * close internally, and forwards every event here so the host can compose its
   * own reactions (e.g. refresh a page when a matching canvas changes).
   */
  onCanvasEvent?: (info: CanvasEventInfo) => void;

  // --- Header slot ---
  /**
   * Renders the host's top banner (logo, actions) INCLUDING the hamburger
   * trigger. Wire the trigger to `api.toggleNav`. The shell owns the layout row
   * but not its contents, so the shell stays app-agnostic.
   */
  renderHeader: (api: ChatAppShellHeaderApi) => ReactNode;
  /** Height (px) reserved for the header row. Defaults to 60. */
  headerHeight?: number;

  // --- Destination catalog + rendering ---
  /** The destination catalog (data, not components). */
  destinations: ChatAppShellDestination[];
  /**
   * Renders the host's own page for a destination into the canvas "hole" (via a
   * portal the shell manages). The shell never imports host pages — it hands
   * back the destination + a `publishSummary`/`openNav` API and portals whatever
   * the host returns.
   */
  renderDestination: (api: ChatAppShellDestinationApi) => ReactNode;
  /**
   * Builds the per-destination C5 identity summary attached when the nav tab
   * opens/replaces. Optional; when omitted the tab carries only its identity.
   */
  buildSummary?: (destination: ChatAppShellDestination) => CanvasTabSummary | undefined;

  // --- Nav-menu slot ---
  /**
   * Renders the host's own navigation menu tree inside the hamburger overlay.
   * The host supplies its (possibly rich, role-scoped) menu; the shell owns only
   * the overlay chrome. Wire menu items to `api.openNav`.
   */
  renderNavMenu: (api: ChatAppShellNavApi) => ReactNode;
  /** Title of the nav overlay drawer. Defaults to "Navigation". */
  navOverlayTitle?: string;
  /** Width (px) of the nav overlay drawer. Defaults to 260. */
  navOverlayWidth?: number;
  /**
   * How the nav menu is presented (C7). `'overlay'` (default) is today's
   * hamburger-driven `Drawer`. `'docked'` renders `renderNavMenu` as a
   * persistent left column and mounts no `Drawer`; the header hamburger's
   * `toggleNav` collapses/expands the column and `navOpened` reflects its
   * expanded (`true`) / collapsed (`false`) state.
   */
  navMode?: 'overlay' | 'docked';
  /** Width (px) of the docked nav column. Defaults to `navOverlayWidth` (260). */
  navDockedWidth?: number;

  // --- Route-sync contract (routing stays host-side) ---
  /**
   * The id of the destination currently reflected by the host's URL, or null
   * for "no destination" (e.g. a landing/greeting route). When this changes the
   * shell opens/replaces the reserved nav tab to match (host-URL-changed → set
   * active). The host owns the URL→id mapping and any policy (e.g. keep the
   * greeting on the default route by passing null).
   */
  activeDestinationId?: string | null;
  /**
   * Called when the shell activates a destination (via the hamburger menu or an
   * `open_nav` event) so the host can update its URL (active-destination-changed
   * → host updates URL). The URL change then flows back via `activeDestinationId`.
   */
  onActiveDestinationChange?: (destination: ChatAppShellDestination) => void;
  /**
   * Resolves the opaque string from an `open_nav` event (or a hamburger call)
   * to a destination. Defaults to matching by `id` then exact `route`. Provide
   * this for custom matching (e.g. route-prefix matching for parameterized
   * routes).
   */
  resolveDestination?: (idOrRoute: string) => ChatAppShellDestination | undefined;

  // --- Reserved nav tab identity ---
  /** Reserved nav tab id. Defaults to "nav". */
  navTabId?: string;
  /** Reserved nav tab kind (used as the C5 identity kind). Defaults to "nav-view". */
  navTabKind?: string;

  /** Optional style overrides merged onto the shell root (default height 100vh). */
  style?: CSSProperties;

  /**
   * Assistant column width in the docked split (pixels or CSS length). Controlled
   * counterpart to drag-resize; default `"40%"` when omitted.
   */
  assistantWidth?: number | string;
  /** Uncontrolled initial assistant width (pixels or CSS length). */
  defaultAssistantWidth?: number | string;
  /** Minimum assistant column width in pixels. Defaults to `320`. */
  assistantMinWidth?: number;
  /** Optional maximum assistant column width in pixels. */
  assistantMaxWidth?: number;
  /** Render a draggable divider between assistant and canvas. Defaults to `false`. */
  resizableAssistant?: boolean;
  /** Fired with the clamped assistant width in pixels while resizing and on release. */
  onAssistantWidthChange?: (width: number) => void;
  /** Persist user-resized assistant width (px) under this localStorage key. */
  persistAssistantWidthKey?: string;

  /**
   * Which side of the canvas the assistant column sits on (C8a). `'start'`
   * (default) is left; `'end'` is right. Order-only.
   */
  assistantPlacement?: 'start' | 'end';
  /** Controlled collapsed state for the assistant column (C8b). */
  assistantCollapsed?: boolean;
  /** Uncontrolled initial collapsed state (C8b). */
  defaultAssistantCollapsed?: boolean;
  /** Fired whenever the collapsed state toggles (C8b). */
  onAssistantCollapsedChange?: (collapsed: boolean) => void;
  /** Persist collapsed state under this localStorage key (C8b, SSR-safe restore). */
  persistAssistantCollapsedKey?: string;
  /**
   * When true, hide the canvas tab strip while exactly one tab is open.
   * Forwarded to the docked `ChatPanel` / `CanvasPanel`. Defaults to `false`.
   */
  hideSingleTab?: boolean;
}

/**
 * ChatAppShell is a GENERIC, opt-in "AI-forward" workspace shell (C1–C6):
 * a host-provided full-width header on top, a persistent docked assistant on
 * the left, a tabbed canvas on the right, and a hamburger-driven nav overlay.
 * It owns the single reserved nav-tab open-or-replace lifecycle (never
 * accumulate), reacts to `open_nav` events, and drives the overlay chrome — but
 * remains app-agnostic: the host plugs in ONLY via slots/callbacks
 * (`renderHeader`, `destinations`, `renderDestination`, `buildSummary`,
 * `renderNavMenu`, and the two route-sync callbacks). Routing stays host-side.
 *
 * It is additive: the existing `ChatPanel` (drawer + docked) is unchanged.
 */
export function ChatAppShell({
  chatAPI,
  title,
  subtitle,
  defaultMode,
  suggestedPrompts,
  bookmarkPrompts,
  pendingPrompt,
  onPromptConsumed,
  onBusyChange,
  onCanvasEvent,
  renderHeader,
  headerHeight = 60,
  destinations,
  renderDestination,
  buildSummary,
  renderNavMenu,
  navOverlayTitle = 'Navigation',
  navOverlayWidth = 260,
  navMode = 'overlay',
  navDockedWidth,
  activeDestinationId = null,
  onActiveDestinationChange,
  resolveDestination,
  navTabId = 'nav',
  navTabKind = 'nav-view',
  style,
  assistantWidth,
  defaultAssistantWidth,
  assistantMinWidth,
  assistantMaxWidth,
  resizableAssistant,
  onAssistantWidthChange,
  persistAssistantWidthKey,
  assistantPlacement,
  assistantCollapsed,
  defaultAssistantCollapsed,
  onAssistantCollapsedChange,
  persistAssistantCollapsedKey,
  hideSingleTab,
}: ChatAppShellProps) {
  const chatRef = useRef<ChatPanelHandle>(null);
  // In docked nav mode the column starts expanded (navOpened=true); in overlay
  // mode the Drawer starts closed (navOpened=false) exactly as before.
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure(navMode === 'docked');
  const [portalEl, setPortalEl] = useState<HTMLElement | null>(null);
  const navDockedWidthResolved = navDockedWidth ?? navOverlayWidth;

  // The id currently shown in the reserved nav tab (null = closed/greeting).
  // Kept as BOTH state (so the portal re-renders the right page) and a ref (so
  // the URL-sync effect can guard without depending on it).
  const [openedDestId, setOpenedDestIdState] = useState<string | null>(null);
  const openedDestIdRef = useRef<string | null>(null);
  const setOpenedDestId = useCallback((id: string | null) => {
    openedDestIdRef.current = id;
    setOpenedDestIdState(id);
  }, []);

  // Keep callback/data props in refs so the URL-sync effect can depend ONLY on
  // `activeDestinationId` — an inline `buildSummary`/`onActiveDestinationChange`
  // must not retrigger it (which would fight a manual close).
  const buildSummaryRef = useRef(buildSummary);
  buildSummaryRef.current = buildSummary;
  const onActiveChangeRef = useRef(onActiveDestinationChange);
  onActiveChangeRef.current = onActiveDestinationChange;
  const onCanvasEventRef = useRef(onCanvasEvent);
  onCanvasEventRef.current = onCanvasEvent;
  const destinationsRef = useRef(destinations);
  destinationsRef.current = destinations;
  const resolveRef = useRef(resolveDestination);
  resolveRef.current = resolveDestination;

  const findById = useCallback(
    (id: string | null) => (id ? destinationsRef.current.find((d) => d.id === id) : undefined),
    [],
  );

  const resolve = useCallback((idOrRoute: string): ChatAppShellDestination | undefined => {
    if (resolveRef.current) return resolveRef.current(idOrRoute);
    const list = destinationsRef.current;
    return list.find((d) => d.id === idOrRoute) ?? list.find((d) => d.route === idOrRoute);
  }, []);

  // Open (or replace) the reserved nav tab for a destination. Uses refs so this
  // callback stays stable (only `navTabId`/`navTabKind` are inputs).
  const openDestinationTab = useCallback(
    (dest: ChatAppShellDestination) => {
      chatRef.current?.openHostCanvasTab({
        tabId: navTabId,
        title: dest.label,
        kind: navTabKind,
        qualifier: dest.route,
        evictable: false,
        summary: buildSummaryRef.current?.(dest),
      });
      setOpenedDestId(dest.id);
    },
    [navTabId, navTabKind, setOpenedDestId],
  );

  // openNav is shared by hamburger clicks and the engine's open_nav event so
  // both take the exact same path: open/replace the tab, ask the host to sync
  // the URL, and close the overlay.
  const openNav = useCallback(
    (idOrRoute: string) => {
      const dest = resolve(idOrRoute);
      if (!dest) return;
      openDestinationTab(dest);
      onActiveChangeRef.current?.(dest);
      // Overlay mode closes the Drawer after a selection (as before). In docked
      // mode the persistent column stays put — there is nothing to close.
      if (navMode === 'overlay') closeNav();
    },
    [resolve, openDestinationTab, closeNav, navMode],
  );

  const publishSummary = useCallback(
    (summary: CanvasTabSummary | null) => {
      if (summary) {
        chatRef.current?.setCanvasTabSummary(navTabId, summary);
        return;
      }
      // Reset to the identity summary for whatever destination is showing.
      const dest = findById(openedDestIdRef.current);
      const identity = dest ? buildSummaryRef.current?.(dest) : undefined;
      if (identity) chatRef.current?.setCanvasTabSummary(navTabId, identity);
    },
    [navTabId, findById],
  );

  const onHostTabPortal = useCallback(
    (tabId: string, el: HTMLElement | null) => {
      if (tabId === navTabId) setPortalEl(el);
    },
    [navTabId],
  );

  const handleCanvasEvent = useCallback(
    (info: CanvasEventInfo) => {
      // A manual close of the reserved nav tab returns to the greeting until the
      // user navigates again — mirror that by forgetting the opened id.
      if (info.tabId === navTabId && info.action === 'close') {
        setOpenedDestId(null);
      }
      onCanvasEventRef.current?.(info);
    },
    [navTabId, setOpenedDestId],
  );

  const onOpenNav = useCallback((destination: string) => openNav(destination), [openNav]);

  // URL sync (host-URL-changed → set active). Depends ONLY on
  // `activeDestinationId`, so a manual tab close (which doesn't change it) does
  // not retrigger an auto-reopen — the same guarantee the reference consumer had.
  useEffect(() => {
    if (!activeDestinationId) return;
    if (openedDestIdRef.current === activeDestinationId) return;
    const dest = findById(activeDestinationId);
    if (dest) openDestinationTab(dest);
  }, [activeDestinationId, openDestinationTab, findById]);

  const headerApi: ChatAppShellHeaderApi = { navOpened, toggleNav, closeNav, openNav };
  const navApi: ChatAppShellNavApi = {
    openNav,
    closeNav,
    activeDestinationId: openedDestId,
    destinations,
  };

  const activeDest = findById(openedDestId);

  // Shared assistant/canvas panel + reserved-nav-tab portal, placed into either
  // the overlay-nav body (unchanged) or the docked-nav body.
  const chatPanelEl = (
    <ChatPanel
      ref={chatRef}
      variant="docked"
      opened
      onClose={() => {}}
      title={title}
      subtitle={subtitle}
      defaultMode={defaultMode}
      suggestedPrompts={suggestedPrompts}
      bookmarkPrompts={bookmarkPrompts}
      pendingPrompt={pendingPrompt}
      onPromptConsumed={onPromptConsumed}
      onBusyChange={onBusyChange}
      onOpenNav={onOpenNav}
      onHostTabPortal={onHostTabPortal}
      onCanvasEvent={handleCanvasEvent}
      assistantWidth={assistantWidth}
      defaultAssistantWidth={defaultAssistantWidth}
      assistantMinWidth={assistantMinWidth}
      assistantMaxWidth={assistantMaxWidth}
      resizableAssistant={resizableAssistant}
      onAssistantWidthChange={onAssistantWidthChange}
      persistAssistantWidthKey={persistAssistantWidthKey}
      assistantPlacement={assistantPlacement}
      assistantCollapsed={assistantCollapsed}
      defaultAssistantCollapsed={defaultAssistantCollapsed}
      onAssistantCollapsedChange={onAssistantCollapsedChange}
      persistAssistantCollapsedKey={persistAssistantCollapsedKey}
      hideSingleTab={hideSingleTab}
    />
  );

  const portalNode =
    portalEl && activeDest
      ? createPortal(
          renderDestination({ destination: activeDest, publishSummary, openNav }),
          portalEl,
        )
      : null;

  const body =
    navMode === 'docked' ? (
      // C7 — persistent nav column (left) · assistant/canvas area. No Drawer.
      <Box
        style={{ flex: 1, minHeight: 0, position: 'relative', display: 'flex', flexDirection: 'row' }}
      >
        {navOpened && (
          <Box
            component="nav"
            data-testid="nav-docked-column"
            style={{
              flex: `0 0 ${navDockedWidthResolved}px`,
              height: '100%',
              minHeight: 0,
              overflow: 'auto',
              borderRight: '1px solid var(--mantine-color-default-border)',
            }}
          >
            {renderNavMenu(navApi)}
          </Box>
        )}
        <Box style={{ flex: '1 1 auto', minWidth: 0, minHeight: 0, position: 'relative' }}>
          {chatPanelEl}
          {portalNode}
        </Box>
      </Box>
    ) : (
      // Body: docked assistant (left) + canvas (right), nav overlay on top.
      <Box style={{ flex: 1, minHeight: 0, position: 'relative' }}>
        {chatPanelEl}

        {/* Live host page rendered into the reserved nav canvas tab. */}
        {portalNode}

        {/* Hamburger-driven navigation overlay (host supplies the menu tree). */}
        <Drawer
          opened={navOpened}
          onClose={closeNav}
          position="left"
          size={navOverlayWidth}
          withCloseButton
          title={navOverlayTitle}
          styles={{ body: { padding: 0, height: `calc(100% - ${headerHeight}px)` } }}
        >
          {renderNavMenu(navApi)}
        </Drawer>
      </Box>
    );

  const shell = (
    <Box style={{ display: 'flex', flexDirection: 'column', height: '100vh', ...style }}>
      {/* Full-width app header (host-provided contents). */}
      <Box
        component="header"
        style={{
          flex: `0 0 ${headerHeight}px`,
          height: headerHeight,
          borderBottom: '1px solid var(--mantine-color-default-border)',
        }}
      >
        {renderHeader(headerApi)}
      </Box>

      {body}
    </Box>
  );

  if (chatAPI) {
    return <ChatAPIProvider value={chatAPI}>{shell}</ChatAPIProvider>;
  }
  return shell;
}
