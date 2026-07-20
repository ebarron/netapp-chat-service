import { renderHook, act, createMockChatAPI } from '../test-utils';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import type { CanvasTab } from './useChatPanel';
import { useChatPanel } from './useChatPanel';

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

const evictable = (id: string): CanvasTab => ({
  tabId: id,
  title: id,
  kind: 'volume',
  qualifier: '',
  content: { type: 'object-detail', kind: 'volume', name: id, sections: [] },
});

describe('useChatPanel host-content canvas tabs (C2–C4)', () => {
  beforeEach(() => vi.clearAllMocks());

  it('openHostCanvasTab adds a host tab and focuses it', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() =>
      result.current.openHostCanvasTab({ tabId: 'nav', title: 'Alerting', kind: 'nav-view', qualifier: '/alerting' }),
    );
    expect(result.current.canvasTabs).toHaveLength(1);
    const tab = result.current.canvasTabs[0];
    expect(tab.tabId).toBe('nav');
    expect(tab.host).toBe(true);
    expect(tab.title).toBe('Alerting');
    expect(result.current.activeCanvasTab).toBe('nav');
  });

  it('openHostCanvasTab replaces content of an existing tab (single reused nav tab)', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Alerting' }));
    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Network' }));
    expect(result.current.canvasTabs).toHaveLength(1);
    expect(result.current.canvasTabs[0].title).toBe('Network');
    expect(result.current.activeCanvasTab).toBe('nav');
  });

  it('updateHostCanvasTab updates in place without changing focus', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Alerting' }));
    act(() => result.current.addOrFocusCanvasTab(evictable('vol::v1')));
    expect(result.current.activeCanvasTab).toBe('vol::v1');

    act(() => result.current.updateHostCanvasTab('nav', { title: 'Alerting (updated)' }));
    expect(result.current.canvasTabs.find((t) => t.tabId === 'nav')?.title).toBe('Alerting (updated)');
    // Focus unchanged.
    expect(result.current.activeCanvasTab).toBe('vol::v1');
  });

  it('nav tab (evictable:false) is exempt from max-tab FIFO eviction but user-closable', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Nav', evictable: false }));
    // Fill well beyond the max with evictable tabs.
    for (let i = 1; i <= 8; i++) {
      act(() => result.current.addOrFocusCanvasTab(evictable(`t${i}`)));
    }
    // Capped at 5, and the nav tab survived every eviction.
    expect(result.current.canvasTabs).toHaveLength(5);
    expect(result.current.canvasTabs.some((t) => t.tabId === 'nav')).toBe(true);

    // The user can still close it manually.
    act(() => result.current.closeCanvasTab('nav'));
    expect(result.current.canvasTabs.some((t) => t.tabId === 'nav')).toBe(false);
  });

  it('canvas is hidden at zero tabs (state collapses to empty/null)', () => {
    const { result } = renderHook(() => useChatPanel());
    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Nav', evictable: false }));
    act(() => result.current.closeCanvasTab('nav'));
    expect(result.current.canvasTabs).toHaveLength(0);
    expect(result.current.activeCanvasTab).toBeNull();
  });
});

describe('useChatPanel canvas context provider (C5)', () => {
  it('forwards an attached summary (with digest + key_properties) into canvas_tabs', async () => {
    let body: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_p: string, b: unknown) => {
        body = b as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    act(() =>
      result.current.openHostCanvasTab({
        tabId: 'nav',
        title: 'Alerting',
        kind: 'nav-view',
        qualifier: '/alerting',
        evictable: false,
        summary: {
          kind: 'nav-view',
          name: 'Alerting',
          qualifier: '/alerting',
          status: 'warning',
          key_properties: { enabled: '3', disabled: '1' },
          digest: 'Latency rule firing on cls1.',
        },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('what is on screen?');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs).toHaveLength(1);
    expect(tabs[0]).toEqual({
      tab_id: 'nav',
      kind: 'nav-view',
      name: 'Alerting',
      qualifier: '/alerting',
      status: 'warning',
      key_properties: { enabled: '3', disabled: '1' },
      digest: 'Latency rule firing on cls1.',
    });
  });

  it('omits empty summary fields cleanly (no null/empty noise on the wire)', async () => {
    let body: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_p: string, b: unknown) => {
        body = b as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    // Host tab with only identity in its summary — no status/key_properties/digest.
    act(() =>
      result.current.openHostCanvasTab({
        tabId: 'nav',
        title: 'Home',
        kind: 'nav-view',
        qualifier: '/home',
        summary: { kind: 'nav-view', name: 'Home', qualifier: '/home', digest: '   ' },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('hi');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs[0]).toEqual({
      tab_id: 'nav',
      kind: 'nav-view',
      name: 'Home',
      qualifier: '/home',
    });
    expect('digest' in tabs[0]).toBe(false);
    expect('status' in tabs[0]).toBe(false);
    expect('key_properties' in tabs[0]).toBe(false);
  });

  it('setCanvasTabSummary attaches a summary to an already-open tab', async () => {
    let body: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_p: string, b: unknown) => {
        body = b as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    act(() => result.current.openHostCanvasTab({ tabId: 'nav', title: 'Home', kind: 'nav-view' }));
    act(() => result.current.setCanvasTabSummary('nav', { digest: 'Capacity 72%.' }));

    await act(async () => {
      await result.current.sendMessage('hi');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs[0].digest).toBe('Capacity 72%.');
  });

  it('legacy declarative tabs keep the exact prior wire shape (backward compat)', async () => {
    let body: Record<string, unknown> | null = null;
    const api = createMockChatAPI({
      stream: vi.fn().mockImplementation(async (_p: string, b: unknown) => {
        body = b as Record<string, unknown>;
        return makeSSEResponse();
      }),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    act(() =>
      result.current.addOrFocusCanvasTab({
        tabId: 'volume::vol_prod_01::on SVM svm1',
        title: 'vol_prod_01',
        kind: 'volume',
        qualifier: 'on SVM svm1',
        content: { type: 'object-detail', kind: 'volume', name: 'vol_prod_01', status: 'warning', sections: [] },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('what about that volume?');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs[0]).toEqual({
      tab_id: 'volume::vol_prod_01::on SVM svm1',
      kind: 'volume',
      name: 'vol_prod_01',
      qualifier: 'on SVM svm1',
      status: 'warning',
    });
  });
});
