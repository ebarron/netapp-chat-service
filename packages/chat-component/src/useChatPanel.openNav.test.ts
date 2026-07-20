import { renderHook, act, createMockChatAPI } from '../test-utils';
import { vi, describe, it, expect } from 'vitest';
import { useChatPanel } from './useChatPanel';

/** Build an SSE response that emits an open_nav event then done. */
function makeOpenNavSSEResponse(payload: Record<string, unknown>): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('event: open_nav\ndata: ' + JSON.stringify(payload) + '\n\n'));
      controller.enqueue(encoder.encode('event: done\ndata: {"session_id":"s1"}\n\n'));
      controller.close();
    },
  });
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
}

describe('useChatPanel open-nav SSE handling (C6)', () => {
  it('parses open_nav and surfaces the destination to the host handler', async () => {
    const onOpenNav = vi.fn();
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeOpenNavSSEResponse({ destination: 'settings/general' })),
    });
    const { result } = renderHook(() => useChatPanel({ onOpenNav }), { api });

    await act(async () => {
      await result.current.sendMessage('open general settings');
    });

    expect(onOpenNav).toHaveBeenCalledTimes(1);
    expect(onOpenNav).toHaveBeenCalledWith('settings/general');
    // open_nav is a side-channel signal — it must NOT create a canvas tab.
    expect(result.current.canvasTabs).toHaveLength(0);
  });

  it('is a safe no-op when no handler is registered', async () => {
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeOpenNavSSEResponse({ destination: 'settings/general' })),
    });
    const { result } = renderHook(() => useChatPanel(), { api });

    await act(async () => {
      await result.current.sendMessage('open general settings');
    });

    // No crash, no state change.
    expect(result.current.canvasTabs).toHaveLength(0);
    expect(result.current.streaming).toBe(false);
  });

  it('ignores an open_nav event with an empty/missing destination', async () => {
    const onOpenNav = vi.fn();
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(makeOpenNavSSEResponse({ destination: '' })),
    });
    const { result } = renderHook(() => useChatPanel({ onOpenNav }), { api });

    await act(async () => {
      await result.current.sendMessage('open nothing');
    });

    expect(onOpenNav).not.toHaveBeenCalled();
  });
});
