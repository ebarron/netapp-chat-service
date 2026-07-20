import { useCallback, useEffect, useRef, useState } from 'react';
import { useChatAPI } from './ChatAPIContext';

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
  const sendMessage = useCallback(
    async (text: string) => {
      if (!text.trim() || streaming) return;

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

        if (!response.ok || !response.body) {
          const err = await response.text();
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: `Error: ${err || response.statusText}` }
                : m
            )
          );
          setStreaming(false);
          return;
        }

        // Process SSE stream.
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
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
              handleSSEEvent(eventType, parsed, assistantId);
            } catch {
              // Ignore malformed events.
            }
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
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
        setStreaming(false);
        abortRef.current = null;
      }
    },
    [sessionId, streaming, mode, setMode, canvasTabs, api]
  );

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
          // Clear any stale approvals from the finished stream.
          setApprovalQueue([]);
          break;

        case 'canvas_open': {
          // Open (or focus) a canvas tab with the received content.
          const d = data as unknown as Record<string, unknown>;
          const tab: CanvasTab = {
            tabId: d.tab_id as string || '',
            title: d.title as string || '',
            kind: d.kind as string || '',
            qualifier: d.qualifier as string || '',
            content: (d.content as Record<string, unknown>) || {},
          };
          if (tab.tabId) {
            // A canvas whose content carries `close: true` is a directive to
            // CLOSE the matching tab (e.g. after the underlying object was
            // deleted) rather than open one. Either way the host is notified
            // via onCanvasEvent so it can react to the specific canvas.
            const isClose = !!(tab.content as Record<string, unknown>).close;
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

  /** Stop the current stream. */
  const stop = useCallback(async () => {
    if (abortRef.current) {
      abortRef.current.abort();
    }
    // Also notify the backend.
    if (sessionId) {
      try {
        await api.post('/chat/stop', { session_id: sessionId });
      } catch {
        // best-effort
      }
    }
  }, [sessionId]);

  /** Clear conversation. */
  const clear = useCallback(async () => {
    if (sessionId) {
      try {
        await api.delete('/chat/session', { session_id: sessionId });
      } catch {
        // Best-effort clear.
      }
    }
    setMessages([]);
    setSessionId(undefined);
  }, [sessionId]);

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
