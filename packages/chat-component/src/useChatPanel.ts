import { useCallback, useEffect, useRef, useState } from 'react';
import { useChatAPI } from './ChatAPIContext';
import {
  normalizeCanvasContent,
  type PayloadValidationIssue,
} from './charts/chartTypes';

/** A message in the chat conversation. */
export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  toolName?: string;
  toolStatus?: 'executing' | 'completed' | 'failed';
  toolParams?: unknown;
  toolResult?: string;
  capability?: string;
}

/** A capability definition returned by the backend. */
export interface Capability {
  id: string;
  name: string;
  description: string;
  state: 'off' | 'ask' | 'ask-on-write' | 'allow';
  available: boolean;
  tools_count: number;
  /** Tools annotated as read-only (or in the per-server allowlist). */
  read_only_tools_count: number;
}

/** Tool budget summary returned by /chat/capabilities. */
export interface ToolBudget {
  used: number;
  max: number;
  mode: ChatMode;
}

/** Per-mode tool budget previews returned by /chat/capabilities. */
export interface ToolBudgets {
  read_only: { used: number; max: number };
  read_write: { used: number; max: number };
}

/** A pending tool approval event from the SSE stream. */
export interface PendingApproval {
  approvalId: string;
  capability: string;
  tool: string;
  params?: unknown;
  description: string;
}

/** Chat mode: read-only or read-write. */
export type ChatMode = 'read-only' | 'read-write';

/** State of the opt-in one-slot interactive action queue. */
export type InteractiveSubmissionState = 'idle' | 'queued' | 'continuing';

/**
 * A per-tab context summary the host attaches so the LLM stays aware of what
 * is shown in a canvas tab (C5). It mirrors the engine's `CanvasTabSummary`
 * and is forwarded verbatim in the `canvas_tabs` field of `/chat/message`.
 *
 * All fields are optional: absent/empty fields are omitted from the wire so
 * there is no empty/`null` noise. `digest` is a free-text description for
 * pages that don't decompose neatly into `key_properties`. Secrets must be
 * excluded by the host before attaching.
 */
export interface CanvasTabSummary {
  kind?: string;
  name?: string;
  qualifier?: string;
  status?: string;
  key_properties?: Record<string, string>;
  digest?: string;
  /**
   * First-class structured option sets (C1 grounding): the AVAILABLE choices
   * for the on-screen controls (dropdowns, picklists, segmented controls, mode
   * toggles). Advertising the real choice set lets the assistant answer "what
   * options are there?" from fact instead of inventing plausible-but-wrong
   * values. Absent/empty ⇒ today's behavior (nothing added to the wire).
   * Secrets must never be placed in a choice set.
   */
  options?: CanvasControlOptions[];
}

/**
 * The available choices for a single on-screen control. `label` is the
 * human-readable control label (e.g. "Provider", "Preferred ONTAP API");
 * `choices` is the list of selectable option labels. Empty choices are dropped
 * before being sent.
 */
export interface CanvasControlOptions {
  label: string;
  choices: string[];
}

/** A canvas tab holding pinned content (object-detail or dashboard). */
export interface CanvasTab {
  tabId: string;
  title: string;
  kind: string;
  qualifier: string;
  content: Record<string, unknown>;
  /**
   * True when this tab's body is host-provided React content rendered via a
   * portal target the component exposes (C2–C4), rather than declarative
   * engine-driven JSON. The component never imports host pages — it renders an
   * empty mount node and the host portals its own tree in.
   */
  host?: boolean;
  /**
   * When `false`, this tab is exempt from the max-tab FIFO auto-eviction (the
   * reserved "nav" tab, §8). It can still be closed manually. Defaults to
   * evictable (undefined/true) so existing engine tabs behave as today.
   */
  evictable?: boolean;
  /** Host-attached context summary forwarded to the LLM (C5). */
  summary?: CanvasTabSummary;
}

/**
 * Input for opening/updating a host-content canvas tab (C2–C4). The body is
 * rendered by the host via a portal into the mount node the component exposes
 * (see `onHostTabPortal`), so no React node is passed here.
 */
export interface HostCanvasTabInput {
  tabId: string;
  title: string;
  kind?: string;
  qualifier?: string;
  /** false = exempt from max-tab FIFO eviction (the reserved nav tab). */
  evictable?: boolean;
  /** C5 context summary attached to this tab. */
  summary?: CanvasTabSummary;
}

/** Maximum number of canvas tabs before oldest auto-closes. */
const MAX_CANVAS_TABS = 5;

/** SSE event data as sent by the backend. */
interface SSEData {
  type: string;
  content?: string;
  tool?: string;
  params?: unknown;
  result?: string;
  error?: string;
  message?: string;
  session_id?: string;
  capability?: string;
  approval_id?: string;
  description?: string;
  destination?: string;
}

function reportRejectedCanvasPayload(issues: PayloadValidationIssue[]): void {
  // Paths and reason codes are safe for diagnostics; never log payload values,
  // titles, tab IDs, tool parameters, or other model-generated content.
  console.warn('[netapp-chat-component] Rejected invalid canvas payload', {
    code: 'invalid_canvas_payload',
    issues: issues.map(({ code, path }) => ({ code, path })),
  });
}

function reportMalformedSSEFrame(): void {
  console.warn('[netapp-chat-component] Rejected malformed SSE frame', {
    code: 'malformed_sse_frame',
  });
}

let msgCounter = 0;
const nextId = () => `msg-${++msgCounter}`;

/** Auto-disable timer duration in ms (10 minutes). */
const MODE_TIMEOUT_MS = 10 * 60 * 1000;

/**
 * useChatPanel manages chat state including messages, streaming, sessions,
 * mode toggle, capabilities, and approval flow.
 * Design ref: docs/chatbot-design-spec.md §5.1, §5.2, §6, §7
 */
export interface UseChatPanelOptions {
  /**
   * Initial mode for the chat panel. Defaults to 'read-write' so that
   * deployments backed by MCP servers without ToolAnnotations still
   * expose their tools to the LLM. Users can toggle at runtime via the
   * existing ModeToggle UI; this only sets the initial value.
   */
  defaultMode?: ChatMode;
  /**
   * Called when a canvas tab is opened/updated or closed. Lets the host
   * react to a specific canvas (e.g. refresh a related page when a matching
   * canvas changes). A canvas whose content carries `close: true` closes
   * the matching tab instead of opening it, and is reported with
   * `action: 'close'`.
   */
  onCanvasEvent?: (info: CanvasEventInfo) => void;
  /**
   * Called when the engine emits an `open_nav` SSE event (C6) — typically from
   * a host-registered `open_nav_view` tool. The host acts on the opaque
   * `destination` (e.g. routes to a screen / opens the nav canvas tab). When
   * omitted, the event is a safe no-op.
   */
  onOpenNav?: (destination: string) => void;
  /**
   * When true, Canvas and panel-extension actions submitted during an active
   * stream occupy a one-slot queue and run once after that stream closes.
   * Ordinary sendMessage behavior is unchanged, and the default is false.
   */
  queueInteractiveSubmissions?: boolean;
}

/** Describes a canvas open/update or close, for `onCanvasEvent`. */
export interface CanvasEventInfo {
  action: 'open' | 'close';
  tabId: string;
  title: string;
  kind: string;
}

export function useChatPanel(options?: UseChatPanelOptions) {
  const api = useChatAPI();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [sessionId, setSessionId] = useState<string | undefined>();
  const [configured, setConfigured] = useState<boolean>(false);
  const abortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);
  const requestActiveRef = useRef(false);
  const requestEpochRef = useRef(0);
  const queueHandoffAllowedRef = useRef(true);
  const queueGenerationRef = useRef(0);
  const interactiveActiveRef = useRef(false);
  const queuedInteractiveRef = useRef<{ message: string; generation: number } | null>(null);
  const [interactiveSubmissionState, setInteractiveSubmissionState] =
    useState<InteractiveSubmissionState>('idle');

  // Phase 2: Mode toggle
  const [mode, setModeState] = useState<ChatMode>(options?.defaultMode ?? 'read-write');
  const modeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [modeTimeLeft, setModeTimeLeft] = useState<number | null>(null);
  const modeStartRef = useRef<number | null>(null);
  const modeIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Phase 2: Capabilities
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [toolBudgets, setToolBudgets] = useState<ToolBudgets | null>(null);
  /** Last error from a capability/mode change attempt (e.g. budget exceeded). */
  const [capabilityError, setCapabilityError] = useState<string | null>(null);

  // Phase 2: Pending approval queue (multiple ask-mode tools can arrive in parallel).
  const [approvalQueue, setApprovalQueue] = useState<PendingApproval[]>([]);
  const pendingApproval = approvalQueue[0] ?? null;

  // Canvas tabs state.
  const [canvasTabs, setCanvasTabs] = useState<CanvasTab[]>([]);
  const [activeCanvasTab, setActiveCanvasTab] = useState<string | null>(null);

  // Keep the latest onCanvasEvent in a ref so the SSE handler (deps: [])
  // always calls the current callback without being a dependency.
  const onCanvasEventRef = useRef(options?.onCanvasEvent);
  onCanvasEventRef.current = options?.onCanvasEvent;

  // Same pattern for the C6 open-nav handler.
  const onOpenNavRef = useRef(options?.onOpenNav);
  onOpenNavRef.current = options?.onOpenNav;

  useEffect(() => {
    // React StrictMode replays mount effects in development. Restore the
    // mounted marker during setup so the replayed cleanup does not leave the
    // live hook permanently unable to finish queued work.
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      requestEpochRef.current += 1;
      requestActiveRef.current = false;
      queueGenerationRef.current += 1;
      interactiveActiveRef.current = false;
      queuedInteractiveRef.current = null;
      abortRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (options?.queueInteractiveSubmissions !== false) return;
    queueGenerationRef.current += 1;
    interactiveActiveRef.current = false;
    queuedInteractiveRef.current = null;
    setInteractiveSubmissionState('idle');
  }, [options?.queueInteractiveSubmissions]);

  /** Clears the mode auto-disable timer. */
  const clearModeTimer = useCallback(() => {
    if (modeTimerRef.current) {
      clearTimeout(modeTimerRef.current);
      modeTimerRef.current = null;
    }
    if (modeIntervalRef.current) {
      clearInterval(modeIntervalRef.current);
      modeIntervalRef.current = null;
    }
    modeStartRef.current = null;
    setModeTimeLeft(null);
  }, []);

  /** Sets mode and manages auto-disable timer for read-write. */
  const setMode = useCallback(
    (newMode: ChatMode) => {
      // Block mode switch when it would exceed the LLM tool budget. The
      // server has authoritative budgets in `toolBudgets`; the UI uses
      // them as a pre-check so the user gets a clear blocker dialog
      // instead of a runtime error mid-message.
      if (newMode === 'read-write' && toolBudgets) {
        const rw = toolBudgets.read_write;
        if (rw.used > rw.max) {
          setCapabilityError(
            `Cannot switch to read-write: ${rw.used} tools would be sent (max ${rw.max}). ` +
              `Open Capabilities and disable an MCP to free up budget.`,
          );
          return;
        }
      }
      setCapabilityError(null);
      setModeState(newMode);
      clearModeTimer();

      if (newMode === 'read-write') {
        // Start auto-disable timer.
        modeStartRef.current = Date.now();
        setModeTimeLeft(MODE_TIMEOUT_MS);

        modeTimerRef.current = setTimeout(() => {
          setModeState('read-only');
          clearModeTimer();
        }, MODE_TIMEOUT_MS);

        // Update countdown every second.
        modeIntervalRef.current = setInterval(() => {
          if (modeStartRef.current) {
            const elapsed = Date.now() - modeStartRef.current;
            const remaining = Math.max(0, MODE_TIMEOUT_MS - elapsed);
            setModeTimeLeft(remaining);
          }
        }, 1000);
      }
    },
    [clearModeTimer, toolBudgets]
  );

  // Cleanup timer on unmount.
  useEffect(() => {
    return () => {
      clearModeTimer();
    };
  }, [clearModeTimer]);

  /** Check if AI is configured. */
  const checkConfigured = useCallback(async () => {
    try {
      const data = await api.get('/ai/config');
      setConfigured(data.configured ?? false);
      return data.configured ?? false;
    } catch {
      setConfigured(false);
      return false;
    }
  }, []);

  /** Fetch capabilities from backend. */
  const fetchCapabilities = useCallback(async () => {
    try {
      const data = await api.get('/chat/capabilities');
      setCapabilities(data.capabilities ?? []);
      setToolBudgets(data.tool_budgets ?? null);
    } catch {
      // Capabilities unavailable — leave empty.
    }
  }, []);

  /**
   * Update a capability state. Returns true on success. On failure (e.g. the
   * change would exceed the LLM's tool budget), the previous state is left
   * intact and `capabilityError` is populated with the server message so the
   * UI can show it.
   */
  const updateCapability = useCallback(async (id: string, state: string): Promise<boolean> => {
    setCapabilityError(null);
    const previous = capabilities.find((c) => c.id === id)?.state;
    try {
      const data = await api.post('/chat/capabilities', {
        capabilities: { [id]: state },
        mode,
      });
      setCapabilities((prev) =>
        prev.map((c) => (c.id === id ? { ...c, state: state as Capability['state'] } : c))
      );
      // Refresh budgets so the UI reflects the new totals.
      try {
        const fresh = await api.get('/chat/capabilities');
        setToolBudgets(fresh.tool_budgets ?? null);
      } catch {
        // best-effort
      }
      void data;
      return true;
    } catch (e) {
      // Try to surface the server's structured error message.
      let msg = 'Failed to update capability.';
      if (e instanceof Error && e.message) {
        try {
          const parsed = JSON.parse(e.message);
          msg = parsed.message ?? msg;
        } catch {
          msg = e.message;
        }
      }
      setCapabilityError(msg);
      // Re-fetch to ensure local state matches server.
      try {
        const data = await api.get('/chat/capabilities');
        setCapabilities(data.capabilities ?? []);
        setToolBudgets(data.tool_budgets ?? null);
      } catch {
        // best-effort
      }
      void previous;
      return false;
    }
  }, [capabilities, mode]);

  /** Approve a pending tool call. */
  const approveAction = useCallback(async () => {
    if (!pendingApproval) return;
    try {
      await api.post('/chat/approve', { approval_id: pendingApproval.approvalId });
    } catch {
      // best-effort
    } finally {
      setApprovalQueue((q) => q.slice(1));
    }
  }, [pendingApproval]);

  /** Deny a pending tool call. */
  const denyAction = useCallback(async () => {
    if (!pendingApproval) return;
    try {
      await api.post('/chat/deny', { approval_id: pendingApproval.approvalId });
    } catch {
      // best-effort
    } finally {
      setApprovalQueue((q) => q.slice(1));
    }
  }, [pendingApproval]);

  /** Add a canvas tab or focus an existing one (deduplication by tabId). */
  const addOrFocusCanvasTab = useCallback((tab: CanvasTab) => {
    setCanvasTabs((prev) => {
      const existing = prev.findIndex((t) => t.tabId === tab.tabId);
      if (existing >= 0) {
        const updated = [...prev];
        updated[existing] = tab;
        return updated;
      }
      // Evict the oldest EVICTABLE tab if at capacity. Tabs marked
      // `evictable: false` (the reserved nav tab, §8) are skipped so they are
      // never auto-evicted to make room — though the user can still close them
      // manually. If nothing is evictable we allow a temporary over-cap append.
      let base = prev;
      if (prev.length >= MAX_CANVAS_TABS) {
        const victim = prev.findIndex((t) => t.evictable !== false);
        if (victim >= 0) {
          base = [...prev.slice(0, victim), ...prev.slice(victim + 1)];
        }
      }
      return [...base, tab];
    });
    setActiveCanvasTab(tab.tabId);
  }, []);

  /**
   * Open (or focus/replace) a host-content canvas tab (C2–C4). The tab's body
   * is rendered by the host via a portal into the mount node the component
   * exposes; only identity + optional summary flow through here.
   */
  const openHostCanvasTab = useCallback((input: HostCanvasTabInput) => {
    addOrFocusCanvasTab({
      tabId: input.tabId,
      title: input.title,
      kind: input.kind ?? 'host',
      qualifier: input.qualifier ?? '',
      content: {},
      host: true,
      evictable: input.evictable,
      summary: input.summary,
    });
  }, [addOrFocusCanvasTab]);

  /**
   * Update an existing host-content tab in place (title/summary/etc.) without
   * changing which tab is focused. No-op if the tab isn't open.
   */
  const updateHostCanvasTab = useCallback(
    (tabId: string, patch: Partial<Omit<HostCanvasTabInput, 'tabId'>>) => {
      setCanvasTabs((prev) =>
        prev.map((t) =>
          t.tabId === tabId
            ? {
                ...t,
                ...(patch.title !== undefined ? { title: patch.title } : {}),
                ...(patch.kind !== undefined ? { kind: patch.kind } : {}),
                ...(patch.qualifier !== undefined ? { qualifier: patch.qualifier } : {}),
                ...(patch.evictable !== undefined ? { evictable: patch.evictable } : {}),
                ...(patch.summary !== undefined ? { summary: patch.summary } : {}),
              }
            : t,
        ),
      );
    },
    [],
  );

  /** Attach/replace the C5 context summary for any open canvas tab. */
  const setCanvasTabSummary = useCallback((tabId: string, summary: CanvasTabSummary) => {
    setCanvasTabs((prev) => prev.map((t) => (t.tabId === tabId ? { ...t, summary } : t)));
  }, []);

  /** Close a canvas tab. */
  const closeCanvasTab = useCallback((tabId: string) => {
    setCanvasTabs((prev) => {
      const filtered = prev.filter((t) => t.tabId !== tabId);
      setActiveCanvasTab((current) => {
        if (current === tabId) {
          return filtered.length > 0 ? filtered[filtered.length - 1].tabId : null;
        }
        return current;
      });
      return filtered;
    });
  }, []);

  /** Send a message and process the SSE stream. */
  const runMessage = useCallback(
    async (text: string, interactive = false) => {
      if (
        !text.trim()
        || streaming
        || requestActiveRef.current
        || (!interactive && interactiveActiveRef.current)
      ) return;

      // React state is not a mutex: two callbacks can run before `streaming`
      // is published. Claim the request synchronously so same-render sends
      // cannot overlap or overwrite abortRef.
      requestActiveRef.current = true;
      const requestEpoch = requestEpochRef.current;
      let allowQueuedHandoff = true;
      let sawDone = false;
      const isCurrentRequest = () => (
        mountedRef.current && requestEpoch === requestEpochRef.current
      );

      // Reset read-write timer on activity.
      if (mode === 'read-write') {
        setMode('read-write');
      }

      // Add user message.
      const userMsg: ChatMessage = { id: nextId(), role: 'user', content: text };
      setMessages((prev) => [...prev, userMsg]);
      setStreaming(true);

      // Prepare assistant placeholder.
      const assistantId = nextId();
      setMessages((prev) => [
        ...prev,
        { id: assistantId, role: 'assistant', content: '' },
      ]);

      const controller = new AbortController();
      abortRef.current = controller;

      try {
        // Build canvas tab summaries for LLM context.
        //
        // Tabs WITHOUT a host-attached summary keep the exact legacy shape
        // (`tab_id`/`kind`/`name`/`qualifier`/`status` derived from declarative
        // content) — byte-for-byte unchanged for existing consumers. Tabs WITH
        // an attached summary (C5) contribute their host-computed fields, with
        // optional `status`/`key_properties`/`digest` omitted cleanly when
        // empty so there is no null/empty noise on the wire.
        const canvasTabSummaries = canvasTabs.map((tab) => {
          if (tab.summary) {
            const s = tab.summary;
            const out: Record<string, unknown> = {
              tab_id: tab.tabId,
              kind: s.kind ?? tab.kind,
              name: s.name ?? tab.title,
              qualifier: s.qualifier ?? tab.qualifier,
            };
            if (s.status) out.status = s.status;
            if (s.key_properties && Object.keys(s.key_properties).length > 0) {
              out.key_properties = s.key_properties;
            }
            if (s.digest && s.digest.trim()) out.digest = s.digest.trim();
            // C1 structured options: keep only controls that carry at least one
            // non-empty choice, so absent/empty option sets stay off the wire
            // (byte-for-byte unchanged for summaries without options).
            if (s.options && s.options.length > 0) {
              const options = s.options
                .map((o) => ({
                  label: o.label,
                  choices: (o.choices ?? []).map((c) => c?.trim()).filter((c): c is string => !!c),
                }))
                .filter((o) => o.choices.length > 0);
              if (options.length > 0) out.options = options;
            }
            return out;
          }
          const c = tab.content;
          return {
            tab_id: tab.tabId,
            kind: tab.kind,
            name: tab.title,
            qualifier: tab.qualifier,
            status: (c as Record<string, unknown>).status as string | undefined,
          };
        });

        const response = await api.stream('/chat/message', {
          message: text,
          session_id: sessionId,
          mode,
          canvas_tabs: canvasTabSummaries.length > 0 ? canvasTabSummaries : undefined,
        }, controller.signal);

        if (!isCurrentRequest()) {
          await response.body?.cancel();
          return;
        }

        if (!response.ok || !response.body) {
          if (response.ok && !response.body) {
            allowQueuedHandoff = false;
          }
          const err = await response.text();
          if (isCurrentRequest()) {
            setMessages((prev) =>
              prev.map((m) =>
                m.id === assistantId
                  ? { ...m, content: `Error: ${err || response.statusText}` }
                  : m
              )
            );
          }
          return;
        }

        // Process SSE stream.
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (!isCurrentRequest()) {
            await reader.cancel();
            return;
          }
          if (done) break;

          buffer += decoder.decode(value, { stream: true });

          // Process complete SSE events (split on double newline).
          const parts = buffer.split('\n\n');
          buffer = parts.pop() || '';

          for (const part of parts) {
            if (!part.trim()) continue;

            let eventType = 'message';
            let data = '';

            for (const line of part.split('\n')) {
              if (line.startsWith('event: ')) {
                eventType = line.slice(7);
              } else if (line.startsWith('data: ')) {
                data = line.slice(6);
              }
            }

            if (!data) continue;

            try {
              const parsed: SSEData = JSON.parse(data);
              if (eventType === 'done') {
                sawDone = true;
              }
              handleSSEEvent(eventType, parsed, assistantId);
            } catch {
              reportMalformedSSEFrame();
            }
          }
        }
        if (buffer.trim()) {
          // The stream ended in the middle of an SSE frame. Never attempt to
          // parse or surface the partial data as structured UI content.
          reportMalformedSSEFrame();
        }
        // A clean SSE EOF without done has no client-visible guarantee that
        // the server committed session/history state. Do not hand queued work
        // into that ambiguous predecessor.
        if (!sawDone) {
          allowQueuedHandoff = false;
        }
      } catch (err) {
        if (!isCurrentRequest()) {
          // A stop, clear, or unmount invalidated this request. Its stale
          // transport completion must not mutate current chat state.
        } else if (err instanceof DOMException && err.name === 'AbortError') {
          // User stopped the stream.
        } else {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: m.content + '\n\n*Connection error.*' }
                : m
            )
          );
        }
      } finally {
        if (isCurrentRequest()) {
          queueHandoffAllowedRef.current = allowQueuedHandoff;
          requestActiveRef.current = false;
          setStreaming(false);
          if (abortRef.current === controller) {
            abortRef.current = null;
          }
        }
      }
    },
    [sessionId, streaming, mode, setMode, canvasTabs, api]
  );

  const sendMessage = useCallback(
    (text: string) => runMessage(text, false),
    [runMessage],
  );

  /**
   * Submit a Canvas/extension action. With queueing enabled, one exact message
   * snapshot can wait behind the active stream; additional clicks are rejected
   * until that queued turn finishes. With queueing disabled this delegates to
   * sendMessage and therefore preserves the legacy drop-while-streaming path.
   */
  const sendInteractiveMessage = useCallback(
    (text: string): boolean => {
      if (!text.trim()) return false;
      const message = text;
      const requestBusy = requestActiveRef.current || streaming;
      if (!options?.queueInteractiveSubmissions) {
        void sendMessage(message);
        return !requestBusy;
      }
      if (interactiveActiveRef.current) {
        return false;
      }
      if (!requestBusy) {
        const generation = queueGenerationRef.current;
        interactiveActiveRef.current = true;
        setInteractiveSubmissionState('continuing');
        void runMessage(message, true).finally(() => {
          if (
            mountedRef.current
            && generation === queueGenerationRef.current
          ) {
            interactiveActiveRef.current = false;
            setInteractiveSubmissionState('idle');
          }
        });
        return true;
      }
      interactiveActiveRef.current = true;
      queuedInteractiveRef.current = {
        message,
        generation: queueGenerationRef.current,
      };
      setInteractiveSubmissionState('queued');
      return true;
    },
    [
      interactiveSubmissionState,
      options?.queueInteractiveSubmissions,
      runMessage,
      sendMessage,
      streaming,
    ],
  );

  // Dispatch the queued immutable snapshot only after the prior SSE reader has
  // closed and sendMessage's finally block has returned the hook to idle.
  useEffect(() => {
    if (streaming || interactiveSubmissionState !== 'queued') return;
    const queued = queuedInteractiveRef.current;
    if (!queued) {
      interactiveActiveRef.current = false;
      setInteractiveSubmissionState('idle');
      return;
    }
    if (queued.generation !== queueGenerationRef.current) {
      interactiveActiveRef.current = false;
      queuedInteractiveRef.current = null;
      setInteractiveSubmissionState('idle');
      return;
    }
    if (!queueHandoffAllowedRef.current) {
      interactiveActiveRef.current = false;
      queuedInteractiveRef.current = null;
      queueHandoffAllowedRef.current = true;
      setInteractiveSubmissionState('idle');
      return;
    }

    queuedInteractiveRef.current = null;
    setInteractiveSubmissionState('continuing');
    void runMessage(queued.message, true).finally(() => {
      if (
        mountedRef.current
        && queued.generation === queueGenerationRef.current
      ) {
        interactiveActiveRef.current = false;
        setInteractiveSubmissionState('idle');
      }
    });
  }, [interactiveSubmissionState, runMessage, streaming]);

  /** Handle a single parsed SSE event. */
  const handleSSEEvent = useCallback(
    (eventType: string, data: SSEData, assistantId: string) => {
      switch (eventType) {
        case 'message':
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: m.content + (data.content || '') }
                : m
            )
          );
          break;

        case 'tool_call':
          setMessages((prev) => [
            ...prev,
            {
              id: nextId(),
              role: 'tool' as const,
              content: '',
              toolName: data.tool,
              toolStatus: 'executing' as const,
              toolParams: data.params,
              capability: data.capability,
            },
          ]);
          break;

        case 'tool_approval_required':
          // Phase 2: Enqueue approval (multiple ask-mode tools may arrive in parallel).
          setApprovalQueue((q) => [
            ...q,
            {
              approvalId: data.approval_id || '',
              capability: data.capability || '',
              tool: data.tool || '',
              params: data.params,
              description: data.description || '',
            },
          ]);
          break;

        case 'tool_result':
          // Update the most recent tool message with this tool name.
          setMessages((prev) => {
            const msgs = [...prev];
            for (let i = msgs.length - 1; i >= 0; i--) {
              if (msgs[i].role === 'tool' && msgs[i].toolName === data.tool) {
                msgs[i] = {
                  ...msgs[i],
                  toolStatus: data.type === 'tool_error' ? 'failed' : 'completed',
                  toolResult: data.result || data.error || '',
                };
                break;
              }
            }
            return msgs;
          });
          break;

        case 'text_clear':
          // Clear accumulated "thinking" text from a tool-call turn.
          // Claude models emit text alongside tool calls; this keeps
          // the final message clean while still showing feedback during streaming.
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId ? { ...m, content: '' } : m
            )
          );
          break;

        case 'error': {
          let msg = data.message || data.error || 'Unknown error';
          // Produce a friendlier message for common backend errors.
          if (msg.includes('429') || msg.includes('rate_limit')) {
            msg = 'The AI provider rate limit was exceeded. Please wait a moment and try again.';
          } else if (msg.includes('401') || msg.includes('Unauthorized')) {
            msg = 'AI provider authentication failed. Please check your API key in Settings.';
          }
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: m.content + `\n\n**Error:** ${msg}` }
                : m
            )
          );
          break;
        }

        case 'done':
          if (data.session_id) {
            setSessionId(data.session_id);
          }
          // Terminal Canvas turns intentionally have no assistant narration.
          // Remove their empty placeholder instead of leaving a permanent
          // ellipsis bubble after streaming finishes.
          setMessages((prev) => prev.filter(
            (message) => message.id !== assistantId || message.content !== '',
          ));
          // Clear any stale approvals from the finished stream.
          setApprovalQueue([]);
          break;

        case 'canvas_open': {
          // Validate and normalize untrusted model content before it can
          // replace a previously valid tab or enter any other UI state.
          const d = data as unknown as Record<string, unknown>;
          const tabId = typeof d.tab_id === 'string' ? d.tab_id : '';
          const normalized = normalizeCanvasContent(d.content);
          if (!tabId || !normalized.value) {
            reportRejectedCanvasPayload([
              ...(!tabId
                ? [{ code: 'missing_tab_id', path: '$.tab_id', severity: 'error' as const }]
                : []),
              ...normalized.issues,
            ]);
            break;
          }
          const tab: CanvasTab = {
            tabId,
            title: typeof d.title === 'string' ? d.title : '',
            kind: typeof d.kind === 'string' ? d.kind : '',
            qualifier: typeof d.qualifier === 'string' ? d.qualifier : '',
            content: normalized.value,
          };
          // A canvas whose content carries `close: true` is a directive to
          // CLOSE the matching tab (e.g. after the underlying object was
          // deleted) rather than open one. Either way the host is notified
          // via onCanvasEvent so it can react to the specific canvas.
          const isClose = tab.content.close === true;
          if (isClose) {
            closeCanvasTab(tab.tabId);
            onCanvasEventRef.current?.({
              action: 'close',
              tabId: tab.tabId,
              title: tab.title,
              kind: tab.kind,
            });
            break;
          }
          onCanvasEventRef.current?.({
            action: 'open',
            tabId: tab.tabId,
            title: tab.title,
            kind: tab.kind,
          });
          // On narrow viewports, render canvas content inline in chat
          // instead of opening a canvas tab (canvas panel is hidden).
          const narrow = typeof window !== 'undefined' && window.innerWidth < 1024;
          if (narrow) {
            const fenceType = 'panels' in tab.content ? 'dashboard' : 'object-detail';
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
          break;
        }

        case 'open_nav': {
          // C6: lightweight navigation signal. Surface the opaque destination
          // to the host-registered handler. With no handler it's a safe no-op.
          const dest = data.destination;
          if (typeof dest === 'string' && dest) {
            onOpenNavRef.current?.(dest);
          }
          break;
        }
      }
    },
    []
  );

  const cancelCurrentRequest = useCallback(() => {
    requestEpochRef.current += 1;
    requestActiveRef.current = false;
    queueHandoffAllowedRef.current = true;
    queueGenerationRef.current += 1;
    interactiveActiveRef.current = false;
    queuedInteractiveRef.current = null;
    const controller = abortRef.current;
    abortRef.current = null;
    controller?.abort();
    setStreaming(false);
    setInteractiveSubmissionState('idle');
    setApprovalQueue([]);
    setMessages((prev) => prev.filter(
      (message) => message.role !== 'assistant' || message.content !== '',
    ));
  }, []);

  /** Stop the current stream. */
  const stop = useCallback(async () => {
    const stoppedSessionId = sessionId;
    cancelCurrentRequest();
    // Also notify the backend.
    if (stoppedSessionId) {
      try {
        await api.post('/chat/stop', { session_id: stoppedSessionId });
      } catch {
        // best-effort
      }
    }
  }, [cancelCurrentRequest, sessionId]);

  /** Clear conversation. */
  const clear = useCallback(async () => {
    const clearedSessionId = sessionId;
    cancelCurrentRequest();
    // Clear local state before awaiting the network so a stale stream or a
    // slow delete response cannot restore or erase a newer conversation.
    setMessages([]);
    setSessionId(undefined);
    if (clearedSessionId) {
      try {
        await api.delete('/chat/session', { session_id: clearedSessionId });
      } catch {
        // Best-effort clear.
      }
    }
  }, [cancelCurrentRequest, sessionId]);

  // Show/hide tool traces — persisted in localStorage.
  const TRACES_KEY = 'chat-component-show-traces';
  const [showTraces, setShowTracesState] = useState(() => {
    const stored = localStorage.getItem(TRACES_KEY);
    return stored === null ? false : stored === 'true';
  });
  const setShowTraces = useCallback((v: boolean) => {
    setShowTracesState(v);
    localStorage.setItem(TRACES_KEY, String(v));
  }, []);

  return {
    messages,
    streaming,
    sessionId,
    configured,
    mode,
    setMode,
    modeTimeLeft,
    capabilities,
    toolBudgets,
    capabilityError,
    clearCapabilityError: () => setCapabilityError(null),
    fetchCapabilities,
    updateCapability,
    pendingApproval,
    approveAction,
    denyAction,
    sendMessage,
    sendInteractiveMessage,
    interactiveSubmissionState,
    stop,
    clear,
    checkConfigured,
    showTraces,
    setShowTraces,
    canvasTabs,
    activeCanvasTab,
    setActiveCanvasTab,
    addOrFocusCanvasTab,
    closeCanvasTab,
    openHostCanvasTab,
    updateHostCanvasTab,
    setCanvasTabSummary,
  };
}
