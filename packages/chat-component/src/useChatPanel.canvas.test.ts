import { renderHook, act, createMockChatAPI } from '../test-utils';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import type { CanvasTab } from './useChatPanel';
import { useChatPanel } from './useChatPanel';

const makeTab = (id: string, title?: string): CanvasTab => ({
  tabId: id,
  title: title ?? id,
  kind: 'volume',
  qualifier: '',
  content: { type: 'object-detail', kind: 'volume', name: title ?? id, sections: [] },
});

/** Build a minimal SSE-streaming Response for stream() mocks. */
function makeSSEResponse(): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('event: done\ndata: {"session_id":"s1"}\n\n'));
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

describe('useChatPanel canvas state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('starts with empty canvas tabs', () => {
    const { result } = renderHook(() => useChatPanel());
    expect(result.current.canvasTabs).toHaveLength(0);
    expect(result.current.activeCanvasTab).toBeNull();
  });

  it('addOrFocusCanvasTab adds a tab and sets it active', () => {
    const { result } = renderHook(() => useChatPanel());
    const tab = makeTab('vol::v1::');
    act(() => result.current.addOrFocusCanvasTab(tab));
    expect(result.current.canvasTabs).toHaveLength(1);
    expect(result.current.canvasTabs[0].tabId).toBe('vol::v1::');
    expect(result.current.activeCanvasTab).toBe('vol::v1::');
  });

  it('deduplicates tabs by tabId (replaces content, focuses)', () => {
    const { result } = renderHook(() => useChatPanel());
    const tab1 = makeTab('vol::v1::');
    const tab2 = makeTab('vol::v2::');
    const tab1Updated = { ...tab1, title: 'v1-updated' };

    act(() => {
      result.current.addOrFocusCanvasTab(tab1);
      result.current.addOrFocusCanvasTab(tab2);
    });
    expect(result.current.canvasTabs).toHaveLength(2);
    expect(result.current.activeCanvasTab).toBe('vol::v2::');

    act(() => result.current.addOrFocusCanvasTab(tab1Updated));
    expect(result.current.canvasTabs).toHaveLength(2);
    expect(result.current.canvasTabs[0].title).toBe('v1-updated');
    expect(result.current.activeCanvasTab).toBe('vol::v1::');
  });

  it('closes a tab and focuses the previous', () => {
    const { result } = renderHook(() => useChatPanel());
    const tab1 = makeTab('t1');
    const tab2 = makeTab('t2');

    act(() => {
      result.current.addOrFocusCanvasTab(tab1);
      result.current.addOrFocusCanvasTab(tab2);
    });
    expect(result.current.activeCanvasTab).toBe('t2');

    act(() => result.current.closeCanvasTab('t2'));
    expect(result.current.canvasTabs).toHaveLength(1);
    expect(result.current.activeCanvasTab).toBe('t1');
  });

  it('closing the last tab sets activeCanvasTab to null', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => result.current.addOrFocusCanvasTab(makeTab('only')));
    act(() => result.current.closeCanvasTab('only'));
    expect(result.current.canvasTabs).toHaveLength(0);
    expect(result.current.activeCanvasTab).toBeNull();
  });

  it('max 5 tabs — 6th evicts the oldest', () => {
    const { result } = renderHook(() => useChatPanel());
    for (let i = 1; i <= 6; i++) {
      act(() => result.current.addOrFocusCanvasTab(makeTab(`t${i}`)));
    }
    expect(result.current.canvasTabs).toHaveLength(5);
    // Oldest (t1) should be gone, newest (t6) should be present.
    expect(result.current.canvasTabs.map((t) => t.tabId)).toEqual([
      't2', 't3', 't4', 't5', 't6',
    ]);
    expect(result.current.activeCanvasTab).toBe('t6');
  });

  it('closing a non-active tab does not change active tab', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => {
      result.current.addOrFocusCanvasTab(makeTab('t1'));
      result.current.addOrFocusCanvasTab(makeTab('t2'));
      result.current.addOrFocusCanvasTab(makeTab('t3'));
    });
    // t3 is active
    act(() => result.current.closeCanvasTab('t1'));
    expect(result.current.activeCanvasTab).toBe('t3');
    expect(result.current.canvasTabs).toHaveLength(2);
  });
});

describe('useChatPanel narrow viewport canvas behavior', () => {
  it('addOrFocusCanvasTab still adds tabs regardless of viewport', () => {
    // The hook's addOrFocusCanvasTab is viewport-agnostic — it always
    // adds. The canvas_open SSE handler checks viewport before calling it.
    // This test documents that narrow viewport gating happens at the
    // event handler level, not at the state management level.
    const { result } = renderHook(() => useChatPanel());
    const tab = makeTab('vol::v1::');
    act(() => result.current.addOrFocusCanvasTab(tab));
    expect(result.current.canvasTabs).toHaveLength(1);
    expect(result.current.activeCanvasTab).toBe('vol::v1::');
  });
});

describe('useChatPanel canvas tab summary extraction', () => {
  it('sendMessage includes canvas_tabs with correct shape', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_path: string, body: unknown) => {
        capturedBody = body as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });

    const { result } = renderHook(() => useChatPanel(), { api });

    // Add a canvas tab with content that has a status field.
    const tab: CanvasTab = {
      tabId: 'volume::vol_prod_01::on SVM svm1',
      title: 'vol_prod_01',
      kind: 'volume',
      qualifier: 'on SVM svm1',
      content: {
        type: 'object-detail',
        kind: 'volume',
        name: 'vol_prod_01',
        status: 'warning',
        sections: [],
      },
    };
    act(() => result.current.addOrFocusCanvasTab(tab));

    // Send a message — this should include canvas_tabs in the request body.
    await act(async () => {
      await result.current.sendMessage('what about that volume?');
    });

    expect(capturedBody).not.toBeNull();
    const tabs = capturedBody!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs).toHaveLength(1);
    expect(tabs[0]).toEqual({
      tab_id: 'volume::vol_prod_01::on SVM svm1',
      kind: 'volume',
      name: 'vol_prod_01',
      qualifier: 'on SVM svm1',
      status: 'warning',
    });
  });

  it('sendMessage omits canvas_tabs when no tabs are open', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_path: string, body: unknown) => {
        capturedBody = body as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });

    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('hello');
    });

    expect(capturedBody).not.toBeNull();
    expect(capturedBody!.canvas_tabs).toBeUndefined();
  });
});

/** Build an SSE response that emits a single canvas_open event then done. */
function makeCanvasSSEResponse(canvas: Record<string, unknown>): Response {
  return makeCanvasSequenceSSEResponse([canvas]);
}

/** Build an SSE response containing canvas events in arrival order. */
function makeCanvasSequenceSSEResponse(canvases: Record<string, unknown>[]): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      for (const canvas of canvases) {
        controller.enqueue(
          encoder.encode('event: canvas_open\ndata: ' + JSON.stringify(canvas) + '\n\n')
        );
      }
      controller.enqueue(encoder.encode('event: done\ndata: {"session_id":"s1"}\n\n'));
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

describe('useChatPanel canvas events (onCanvasEvent + close)', () => {
  // jsdom reports innerWidth 1024 by default in this harness, so the wide-view
  // path (open a tab) is taken. Guard in case a prior test changed it.
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
  });

  it('opens a tab and notifies onCanvasEvent with action "open"', async () => {
    const onCanvasEvent = vi.fn();
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(
        makeCanvasSSEResponse({
          tab_id: 'dashboard::Item Detail::',
          title: 'Item Detail',
          kind: 'dashboard',
          qualifier: '',
          content: { type: 'dashboard', title: 'Item Detail', panels: [] },
        })
      ),
    });
    const { result } = renderHook(() => useChatPanel({ onCanvasEvent }), { api });

    await act(async () => {
      await result.current.sendMessage('show item detail');
    });

    expect(result.current.canvasTabs).toHaveLength(1);
    expect(onCanvasEvent).toHaveBeenCalledWith({
      action: 'open',
      tabId: 'dashboard::Item Detail::',
      title: 'Item Detail',
      kind: 'dashboard',
    });
  });

  it('rejects a malformed duplicate and preserves the last valid dashboard', async () => {
    const onCanvasEvent = vi.fn();
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const valid = {
      tab_id: 'dashboard::Alert Rule::',
      title: 'Alert Rule',
      kind: 'dashboard',
      qualifier: '',
      content: {
        type: 'dashboard',
        title: 'Alert Rule',
        panels: [{
          type: 'action-button',
          buttons: [{ label: 'Delete', action: 'message', message: 'delete it' }],
        }],
      },
    };
    const malformed = {
      ...valid,
      content: {
        type: 'dashboard',
        title: 'Alert Rule',
        panels: [{ type: 'action-button' }],
      },
    };
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeCanvasSequenceSSEResponse([valid, malformed])),
    });
    const { result } = renderHook(() => useChatPanel({ onCanvasEvent }), { api });

    await act(async () => {
      await result.current.sendMessage('clone the rule');
    });

    expect(result.current.canvasTabs).toHaveLength(1);
    const panels = result.current.canvasTabs[0].content.panels as Array<Record<string, unknown>>;
    expect(panels[0].buttons).toEqual([
      { label: 'Delete', action: 'message', message: 'delete it' },
    ]);
    expect(onCanvasEvent).toHaveBeenCalledTimes(1);
    expect(warn).toHaveBeenCalledWith(
      '[netapp-chat-component] Rejected invalid canvas payload',
      expect.objectContaining({ code: 'invalid_canvas_payload' }),
    );
    const diagnostic = warn.mock.calls[0][1] as Record<string, unknown>;
    expect(JSON.stringify(diagnostic)).not.toContain('Delete');
    expect(JSON.stringify(diagnostic)).not.toContain('Alert Rule');
    warn.mockRestore();
  });

  it('does not put a first malformed dashboard into canvas state', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(
        makeCanvasSSEResponse({
          tab_id: 'dashboard::Broken::',
          title: 'Broken',
          kind: 'dashboard',
          qualifier: '',
          content: {
            type: 'dashboard',
            title: 'Broken',
            panels: [{ type: 'action-button' }],
          },
        })
      ),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('show it');
    });

    expect(result.current.canvasTabs).toHaveLength(0);
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it('rejects a partial SSE frame without changing canvas state', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const encoder = new TextEncoder();
    const response = new Response(new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(
          'event: canvas_open\ndata: {"tab_id":"dashboard::Partial::","content":{"type":"dashboard"'
        ));
        controller.close();
      },
    }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
    const api = createMockChatAPI({ stream: vi.fn().mockResolvedValue(response) });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('show it');
    });

    expect(result.current.canvasTabs).toHaveLength(0);
    expect(warn).toHaveBeenCalledWith(
      '[netapp-chat-component] Rejected malformed SSE frame',
      { code: 'malformed_sse_frame' },
    );
    warn.mockRestore();
  });

  it('closes the matching tab when content.close is set, and notifies "close"', async () => {
    const onCanvasEvent = vi.fn();
    // First open the tab, then a second turn delivers a close directive.
    const stream = vi
      .fn()
      .mockResolvedValueOnce(
        makeCanvasSSEResponse({
          tab_id: 'dashboard::Item Detail::',
          title: 'Item Detail',
          kind: 'dashboard',
          qualifier: '',
          content: { type: 'dashboard', title: 'Item Detail', panels: [] },
        })
      )
      .mockResolvedValueOnce(
        makeCanvasSSEResponse({
          tab_id: 'dashboard::Item Detail::',
          title: 'Item Detail',
          kind: 'dashboard',
          qualifier: '',
          content: { type: 'dashboard', title: 'Item Detail', close: true },
        })
      );
    const api = createMockChatAPI({ stream });
    const { result } = renderHook(() => useChatPanel({ onCanvasEvent }), { api });

    await act(async () => {
      await result.current.sendMessage('show item detail');
    });
    expect(result.current.canvasTabs).toHaveLength(1);

    await act(async () => {
      await result.current.sendMessage('remove the item');
    });

    // Tab removed by the close directive.
    expect(result.current.canvasTabs).toHaveLength(0);
    expect(onCanvasEvent).toHaveBeenLastCalledWith({
      action: 'close',
      tabId: 'dashboard::Item Detail::',
      title: 'Item Detail',
      kind: 'dashboard',
    });
  });
});

describe('useChatPanel deterministic canvas handoff metadata', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
  });

  const canvas = (
    handoff?: Record<string, unknown>,
    overrides: Record<string, unknown> = {},
  ) => ({
    tab_id: 'dashboard::destination::',
    title: 'Destination',
    kind: 'dashboard',
    qualifier: '',
    terminal_interactive: true,
    content: {
      type: 'dashboard',
      title: 'Destination',
      panels: [{ type: 'callout', title: 'Ready', body: 'Ready.' }],
    },
    ...(handoff ? { handoff } : {}),
    ...overrides,
  });

  it.each([
    ['form', 'Provision volume is ready.', 'Continue in the form.'],
    ['results', 'Fleet health is ready.', 'View results.'],
  ])('renders renderer-owned %s handoff without another request', async (
    kind,
    heading,
    actionLabel,
  ) => {
    const stream = vi.fn().mockResolvedValue(makeCanvasSSEResponse(canvas({
      kind,
      heading,
      action_label: actionLabel,
    })));
    const api = createMockChatAPI({ stream });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('show destination');
    });

    expect(stream).toHaveBeenCalledTimes(1);
    expect(result.current.messages.filter((message) => message.role === 'handoff')).toEqual([
      expect.objectContaining({
        canvasTabId: 'dashboard::destination::',
        handoffKind: kind,
        handoffHeading: heading,
        handoffActionLabel: actionLabel,
      }),
    ]);
  });

  it('preserves default behavior for unclassified, nonterminal, and invalid metadata', async () => {
    const events = [
      canvas(undefined),
      canvas({ kind: 'form', heading: 'Wrong', action_label: 'Wrong' }, {
        terminal_interactive: false,
      }),
      canvas({ kind: 'form', heading: 'Line\nbreak', action_label: 'Continue.' }),
    ];
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeCanvasSequenceSSEResponse(events)),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('show destination');
    });

    expect(result.current.messages.some((message) => message.role === 'handoff')).toBe(false);
  });

  it('deduplicates replayed updates by destination and keeps the latest copy', async () => {
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeCanvasSequenceSSEResponse([
        canvas({
          kind: 'results',
          heading: 'Fleet health is ready.',
          action_label: 'View results.',
        }),
        canvas({
          kind: 'results',
          heading: 'Updated fleet health is ready.',
          action_label: 'View updated results.',
        }),
      ])),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('show destination');
    });

    const handoffs = result.current.messages.filter((message) => message.role === 'handoff');
    expect(handoffs).toHaveLength(1);
    expect(handoffs[0]).toEqual(expect.objectContaining({
      handoffHeading: 'Updated fleet health is ready.',
      handoffActionLabel: 'View updated results.',
    }));
  });

  it('removes a prior handoff when the destination closes', async () => {
    const stream = vi
      .fn()
      .mockResolvedValueOnce(makeCanvasSSEResponse(canvas({
        kind: 'form',
        heading: 'Alert rule is ready.',
        action_label: 'Continue in the form.',
      })))
      .mockResolvedValueOnce(makeCanvasSSEResponse(canvas(undefined, {
        terminal_interactive: false,
        content: { type: 'dashboard', title: 'Destination', close: true },
      })));
    const api = createMockChatAPI({ stream });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('open');
    });
    expect(result.current.messages.some((message) => message.role === 'handoff')).toBe(true);
    await act(async () => {
      await result.current.sendMessage('close');
    });
    expect(result.current.messages.some((message) => message.role === 'handoff')).toBe(false);
  });
});
