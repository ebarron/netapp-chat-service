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
