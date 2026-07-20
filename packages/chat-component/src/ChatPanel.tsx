import {
  Drawer,
  Text,
  Group,
  ActionIcon,
  Textarea,
  Button,
  Stack,
  Badge,
  Paper,
  ScrollArea,
  Alert,
  Loader,
  Tooltip,
  Divider,
} from '@mantine/core';
import {
  IconSend,
  IconTrash,
  IconPlayerStop,
  IconRobot,
  IconBolt,
  IconMessageChatbot,
} from '@tabler/icons-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  useChatPanel,
  ChatMessage,
  type ChatMode,
  type CanvasEventInfo,
  type CanvasTabSummary,
  type HostCanvasTabInput,
} from './useChatPanel';
import { ModeToggle } from './ModeToggle';
import { CapabilityControls } from './CapabilityControls';
import { BookmarkPrompts } from './BookmarkPrompts';
import type { BookmarkPrompt } from './BookmarkPrompts';
import { ActionConfirmation } from './ActionConfirmation';
import { ToolStatusCard } from './ToolStatusCard';
import { CanvasPanel } from './CanvasPanel';
import { ChartBlock, DashboardBlock, ObjectDetailBlock, AutoJsonBlock } from './charts';
import { wrapInlineChartJson, hideIncompleteChartJson, sanitizeJson } from './inlineChartDetector';
import { parseChart, parseObjectDetail } from './charts/chartTypes';
import classes from './ChatPanel.module.css';

interface ChatPanelProps {
  opened: boolean;
  onClose: () => void;
  /** Title displayed in the drawer header. Defaults to "AI Assistant". */
  title?: string;
  /** Subtitle badge text. Defaults to none. */
  subtitle?: string;
  /** Suggested prompts shown when the conversation is empty. */
  suggestedPrompts?: string[];
  /** Bookmark prompts grouped by MCP, shown via a book icon in the header. */
  bookmarkPrompts?: BookmarkPrompt[];
  /** When true, renders as a full-page layout instead of a Drawer. */
  fullPage?: boolean;
  /**
   * Initial chat mode ('read-only' or 'read-write'). Defaults to
   * 'read-write'. The user can still toggle mode at runtime; this only
   * sets the initial value.
   */
  defaultMode?: ChatMode;
  /**
   * A prompt to auto-send once. When this value changes to a non-empty
   * string while the panel is `opened` and not streaming, the panel sends
   * it as a user message exactly once, then calls `onPromptConsumed`.
   * The host should clear its own state in `onPromptConsumed` so the same
   * prompt isn't resent on a later re-render.
   */
  pendingPrompt?: string;
  /** Called after `pendingPrompt` has been submitted. */
  onPromptConsumed?: () => void;
  /**
   * Notifies the host when the assistant's busy state changes, so the host
   * can disable a trigger control while a turn is streaming.
   */
  onBusyChange?: (busy: boolean) => void;
  /**
   * Called when a canvas tab opens/updates or closes. Lets the host react to
   * a specific canvas (e.g. refresh a page when a matching canvas changes).
   */
  onCanvasEvent?: (info: CanvasEventInfo) => void;
  /**
   * Layout variant (C1). `"drawer"` (default) is today's slide-over Drawer,
   * byte-for-byte unchanged. `"docked"` renders a persistent, full-height
   * panel that fills its parent (assistant on the left, canvas filling the
   * remaining width) — intended for a full-width-header shell. In docked mode
   * the panel is always present (not gated by `opened`).
   */
  variant?: 'drawer' | 'docked';
  /**
   * Called when the engine emits an `open_nav` SSE event (C6), typically from
   * a host-registered `open_nav_view` tool. Receives the opaque destination
   * string. Absence is a safe no-op.
   */
  onOpenNav?: (destination: string) => void;
  /**
   * Portal mount callback for host-content canvas tabs (C2–C4). Called with
   * the mount node when a host tab mounts (`el`) and `null` when it unmounts.
   * The host renders its own page into `el` via `ReactDOM.createPortal`.
   */
  onHostTabPortal?: (tabId: string, el: HTMLElement | null) => void;
}

/**
 * Imperative handle for driving host-content canvas tabs (C2–C4/C5). Obtain it
 * via a ref on `<ChatPanel>`:
 *
 * ```tsx
 * const ref = useRef<ChatPanelHandle>(null);
 * <ChatPanel ref={ref} variant="docked" onHostTabPortal={...} />
 * ref.current?.openHostCanvasTab({ tabId: 'nav', title: 'Alerting', evictable: false, summary });
 * ```
 */
export interface ChatPanelHandle {
  /** Open (or focus/replace) a host-content canvas tab. */
  openHostCanvasTab(input: HostCanvasTabInput): void;
  /** Update an open host tab in place (title/summary/…) without refocusing. */
  updateHostCanvasTab(tabId: string, patch: Partial<Omit<HostCanvasTabInput, 'tabId'>>): void;
  /** Attach/replace the C5 context summary for any open canvas tab. */
  setCanvasTabSummary(tabId: string, summary: CanvasTabSummary): void;
  /** Close any canvas tab by id (also works for the reserved nav tab). */
  closeCanvasTab(tabId: string): void;
  /** Focus an open canvas tab by id. */
  focusCanvasTab(tabId: string): void;
}

const DEFAULT_SUGGESTED_PROMPTS = [
  "What's the health of my fleet?",
  'Show volumes over 80% capacity',
  'Show me my Grafana dashboards',
];

/**
 * ChatPanel is the main AI assistant side panel.
 * Design ref: docs/chatbot-design-spec.md §6.1
 */
export const ChatPanel = forwardRef<ChatPanelHandle, ChatPanelProps>(function ChatPanel({
  opened,
  onClose,
  title = 'AI Assistant',
  subtitle,
  suggestedPrompts = DEFAULT_SUGGESTED_PROMPTS,
  bookmarkPrompts,
  fullPage = false,
  defaultMode,
  pendingPrompt,
  onPromptConsumed,
  onBusyChange,
  onCanvasEvent,
  variant = 'drawer',
  onOpenNav,
  onHostTabPortal,
}: ChatPanelProps, ref) {
  const {
    messages,
    streaming,
    configured,
    mode,
    setMode,
    modeTimeLeft,
    capabilities,
    toolBudgets,
    capabilityError,
    clearCapabilityError,
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
    closeCanvasTab,
    openHostCanvasTab,
    updateHostCanvasTab,
    setCanvasTabSummary,
  } = useChatPanel({ defaultMode, onCanvasEvent, onOpenNav });

  const docked = variant === 'docked';

  // Expose the host-content canvas tab API (C2–C4/C5) imperatively.
  useImperativeHandle(
    ref,
    () => ({
      openHostCanvasTab,
      updateHostCanvasTab,
      setCanvasTabSummary,
      closeCanvasTab,
      focusCanvasTab: setActiveCanvasTab,
    }),
    [openHostCanvasTab, updateHostCanvasTab, setCanvasTabSummary, closeCanvasTab, setActiveCanvasTab],
  );

  const [input, setInput] = useState('');
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // Check config and fetch capabilities on open. In docked mode the panel is
  // persistent (not gated by `opened`), so treat it as always active.
  const active = docked || opened;
  useEffect(() => {
    if (active) {
      checkConfigured();
      fetchCapabilities();
    }
  }, [active, checkConfigured, fetchCapabilities]);

  // Host-driven prompt injection: auto-send `pendingPrompt` once when the
  // panel is open and idle. Guards against double-send on unrelated
  // re-renders via lastSentRef. See docs/host-prompt-injection.md §4.1.
  const lastSentRef = useRef<string | null>(null);
  useEffect(() => {
    const p = pendingPrompt?.trim();
    // When the host clears the prompt (to ''), reset the dedup guard so an
    // intentional re-send of identical text later is honored.
    if (!p) {
      lastSentRef.current = null;
      return;
    }
    if (!active || streaming) return;
    if (lastSentRef.current === p) return;
    lastSentRef.current = p;
    sendMessage(p);
    onPromptConsumed?.();
  }, [active, pendingPrompt, streaming, sendMessage, onPromptConsumed]);

  // Surface busy state to the host so it can disable its trigger control
  // while a turn is streaming. See docs/host-prompt-injection.md §4.2.
  useEffect(() => {
    onBusyChange?.(streaming);
  }, [streaming, onBusyChange]);

  // Auto-scroll on new messages.
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTo({
        top: scrollRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  }, [messages]);

  const handleSend = useCallback(() => {
    if (!input.trim() || streaming) return;
    sendMessage(input.trim());
    setInput('');
    inputRef.current?.focus();
  }, [input, streaming, sendMessage]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  const handleSuggestedPrompt = useCallback(
    (prompt: string) => {
      sendMessage(prompt);
    },
    [sendMessage]
  );

  const STORAGE_KEY = 'chat-component-width';
  const MIN_WIDTH = 360;
  const MAX_WIDTH = typeof window !== 'undefined' ? window.innerWidth * 0.8 : 1200;
  const DEFAULT_WIDTH = 480;

  const [drawerWidth, setDrawerWidth] = useState(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    const parsed = stored ? Number(stored) : NaN;
    return Number.isFinite(parsed) ? Math.max(MIN_WIDTH, Math.min(parsed, MAX_WIDTH)) : DEFAULT_WIDTH;
  });

  const dragging = useRef(false);

  // Canvas active when tabs exist and viewport is wide enough.
  const isNarrow = typeof window !== 'undefined' && window.innerWidth < 1024;
  const hasCanvas = canvasTabs.length > 0 && !isNarrow;
  const effectiveWidth = hasCanvas
    ? Math.max(drawerWidth * 2.5, typeof window !== 'undefined' ? window.innerWidth * 0.8 : 1200)
    : drawerWidth;

  const onResizeStart = useCallback((e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    dragging.current = true;
    const startX = e.clientX;
    const startWidth = drawerWidth;

    const onMove = (ev: globalThis.PointerEvent) => {
      const delta = ev.clientX - startX; // moving right = increase (left-side drawer)
      const newWidth = Math.max(MIN_WIDTH, Math.min(startWidth + delta, MAX_WIDTH));
      setDrawerWidth(newWidth);
    };
    const onUp = () => {
      dragging.current = false;
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      setDrawerWidth((w: number) => {
        localStorage.setItem(STORAGE_KEY, String(w));
        return w;
      });
    };
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
  }, [drawerWidth]);

  const headerBar = (
    <Group gap="xs" className={classes.fullPageHeader}>
      <IconMessageChatbot size={20} />
      <Group align="baseline" gap="xs">
        <Text fw={600}>{title}</Text>
        {subtitle && <Text size="xs" c="red" fw={500}>{subtitle}</Text>}
      </Group>
    </Group>
  );

  const chatContent = (
    <>
      <div className={hasCanvas ? classes.drawerBody : classes.panelWrapper}>
        <div className={classes.panel}>
        {/* Mode toggle + Capability controls */}
        {configured && (
          <>
            <Group justify="space-between" px="xs">
              <ModeToggle
                mode={mode}
                onChange={setMode}
                timeLeft={modeTimeLeft}
                disabled={!configured}
              />
              <Group gap={4}>
                {bookmarkPrompts && bookmarkPrompts.length > 0 && (
                  <BookmarkPrompts
                    bookmarks={bookmarkPrompts}
                    capabilities={capabilities}
                    onSelect={sendMessage}
                    disabled={!configured}
                  />
                )}
                <CapabilityControls
                  capabilities={capabilities}
                  onUpdate={updateCapability}
                  disabled={!configured}
                  showTraces={showTraces}
                  onShowTracesChange={setShowTraces}
                  mode={mode}
                  toolBudgets={toolBudgets}
                  capabilityError={capabilityError}
                  onClearCapabilityError={clearCapabilityError}
                />
              </Group>
            </Group>
            {capabilityError && (
              <Alert
                color="red"
                variant="light"
                mx="xs"
                mt="xs"
                withCloseButton
                onClose={clearCapabilityError}
              >
                <Text size="xs">{capabilityError}</Text>
              </Alert>
            )}
            <Divider />
          </>
        )}
        {/* Not configured banner */}
        {!configured && (
          <Alert color="yellow" title="AI Not Configured" mb="sm">
            Configure an AI provider in Settings → AI to enable the assistant.
          </Alert>
        )}

        {/* Messages area */}
        <ScrollArea
          viewportRef={scrollRef}
          className={classes.messages}
          style={{ flex: 1 }}
        >
          {messages.length === 0 && configured && (
            <Stack gap="xs" mt="md">
              <Paper
                p="sm"
                radius="sm"
                bg="#fff8b0"
                style={{ border: '1px solid #e6d96e' }}
              >
                <Text fz="sm" c="black" ta="center">
                  You are interacting with a chat bot supported by artificial
                  intelligence. Please check responses for accuracy.
                </Text>
              </Paper>
              <Text fz="sm" c="dimmed" ta="center" mt="sm">
                Suggested prompts:
              </Text>
              {suggestedPrompts.map((prompt) => (
                <Paper
                  key={prompt}
                  className={classes.suggestedPrompt}
                  p="xs"
                  radius="sm"
                  withBorder
                  onClick={() => handleSuggestedPrompt(prompt)}
                >
                  <Text fz="sm">{prompt}</Text>
                </Paper>
              ))}
            </Stack>
          )}

          {(() => {
            // Find the last assistant message index so we can mark it as
            // streaming. Tool messages may be appended after it, so
            // checking idx === messages.length - 1 doesn't work.
            const lastAssistantIdx = streaming
              ? messages.findLastIndex((m: ChatMessage) => m.role === 'assistant')
              : -1;
            const visible = showTraces
              ? messages
              : messages.filter((m: ChatMessage) => m.role !== 'tool');
            return visible.map((msg: ChatMessage) => (
              <MessageBubble
                key={msg.id}
                message={msg}
                onAction={sendMessage}
                readOnly={mode === 'read-only'}
                isStreaming={messages.indexOf(msg) === lastAssistantIdx}
              />
            ));
          })()}

          {streaming && (
            <Group gap="xs" mt="xs">
              <Loader size="xs" />
              <Text fz="xs" c="dimmed">
                Thinking...
              </Text>
            </Group>
          )}
        </ScrollArea>

        {/* Input area */}
        <div className={classes.inputArea}>
          <Group gap="xs" align="flex-end">
            <Textarea
              ref={inputRef}
              placeholder={!configured ? 'AI not configured' : 'Type a message...'}
              value={input}
              onChange={(e) => setInput(e.currentTarget.value)}
              onKeyDown={handleKeyDown}
              disabled={!configured || streaming}
              autosize
              minRows={1}
              maxRows={4}
              style={{ flex: 1 }}
            />
            {streaming ? (
              <Tooltip label="Stop">
                <ActionIcon
                  color="red"
                  variant="filled"
                  size="lg"
                  onClick={stop}
                  aria-label="Stop"
                >
                  <IconPlayerStop size={18} />
                </ActionIcon>
              </Tooltip>
            ) : (
              <Tooltip label="Send">
                <ActionIcon
                  color="blue"
                  variant="filled"
                  size="lg"
                  onClick={handleSend}
                  disabled={!input.trim() || !configured}
                  aria-label="Send"
                >
                  <IconSend size={18} />
                </ActionIcon>
              </Tooltip>
            )}
            <Tooltip label="Clear conversation">
              <ActionIcon
                variant="subtle"
                size="lg"
                onClick={clear}
                disabled={messages.length === 0}
                aria-label="Clear"
              >
                <IconTrash size={18} />
              </ActionIcon>
            </Tooltip>
          </Group>
        </div>
        </div>
        {/* Canvas region — only rendered when tabs are present */}
        {hasCanvas && (
          <CanvasPanel
            tabs={canvasTabs}
            activeTab={activeCanvasTab}
            onTabChange={setActiveCanvasTab}
            onTabClose={closeCanvasTab}
            onAction={sendMessage}
            readOnly={mode === 'read-only'}
            onHostTabPortal={onHostTabPortal}
          />
        )}
      </div>

      {/* Action confirmation dialog */}
      <ActionConfirmation
        approval={pendingApproval}
        onApprove={approveAction}
        onDeny={denyAction}
      />
    </>
  );

  if (docked) {
    // Persistent, full-height panel that fills its parent. The host provides
    // the surrounding full-width-header shell (C1). Always rendered.
    return (
      <div className={classes.docked}>
        {headerBar}
        <div className={classes.dockedBody}>
          {chatContent}
        </div>
      </div>
    );
  }

  if (fullPage) {
    if (!opened) return null;
    return (
      <div className={classes.fullPage}>
        {headerBar}
        <div className={classes.fullPageBody}>
          {chatContent}
        </div>
      </div>
    );
  }

  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      position="left"
      size={effectiveWidth}
      title={
        <Group gap="xs">
          <IconMessageChatbot size={20} />
          <Group align="baseline" gap="xs">
            <Text fw={600}>{title}</Text>
            {subtitle && <Text size="xs" c="red" fw={500}>{subtitle}</Text>}
          </Group>
        </Group>
      }
      styles={{
        body: { height: 'calc(100% - 60px)', display: 'flex', flexDirection: 'column' },
        content: { overflow: 'visible' },
      }}
    >
      {/* Resize handle on the right edge */}
      <div
        className={classes.resizeHandle}
        onPointerDown={onResizeStart}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize chat panel"
      />
      {chatContent}
    </Drawer>
  );
});

interface MessageBubbleProps {
  message: ChatMessage;
  onAction?: (message: string) => void;
  readOnly?: boolean;
  isStreaming?: boolean;
}

/** Renders a single message in the conversation. */
function MessageBubble({ message, onAction, readOnly, isStreaming }: MessageBubbleProps) {
  if (message.role === 'tool') {
    return <ToolStatusCard message={message} />;
  }

  if (message.role === 'user') {
    return (
      <div className={classes.userMessage}>
        <Text fz="sm">{message.content}</Text>
      </div>
    );
  }

  // Assistant message — render markdown with chart/dashboard code-block handlers.
  return (
    <div className={classes.assistantMessage}>
      <ReactMarkdown
        key={readOnly ? 'ro' : 'rw'}
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ children, href, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
            <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
              {children}
            </a>
          ),
          code: ({ className, children, ...props }: React.HTMLAttributes<HTMLElement>) => {
            const content = String(children).replace(/\n$/, '');
            if (className === 'language-dashboard') {
              return <DashboardBlock json={content} onAction={onAction} readOnly={readOnly} />;
            }
            if (className === 'language-object-detail') {
              return <ObjectDetailBlock json={content} onAction={onAction} readOnly={readOnly} />;
            }
            if (className === 'language-chart') {
              return <ChartBlock json={content} onAction={onAction} readOnly={readOnly} />;
            }
            // LLMs emit chart/dashboard JSON inside ```json, ```alert-list, or other fences.
            // Try JSON parsing for any unrecognized language tag or no tag at all.
            // (dashboard/object-detail/chart already returned above.)
            try {
              const parsed = JSON.parse(sanitizeJson(content));
              if (Array.isArray(parsed?.panels)) {
                return <DashboardBlock json={content} onAction={onAction} readOnly={readOnly} />;
              }
              if (parseObjectDetail(JSON.stringify(parsed))) {
                return <ObjectDetailBlock json={JSON.stringify(parsed)} onAction={onAction} readOnly={readOnly} />;
              }
              if (parseChart(JSON.stringify(parsed))) {
                return <ChartBlock json={JSON.stringify(parsed)} onAction={onAction} readOnly={readOnly} />;
              }
              if (typeof parsed === 'object' && parsed !== null) {
                return <AutoJsonBlock json={content} />;
              }
            } catch {
              // Not valid JSON — fall through to raw code rendering.
            }
            return <code className={className} {...props}>{children}</code>;
          },
          pre: ({ children }: React.HTMLAttributes<HTMLPreElement>) => {
            // Strip the <pre> wrapper for dashboard/chart blocks so they render edge-to-edge.
            return <>{children}</>;
          },
        }}
      >
        {isStreaming
          ? hideIncompleteChartJson(message.content || '...')
          : wrapInlineChartJson(message.content || '...')}
      </ReactMarkdown>
    </div>
  );
}


