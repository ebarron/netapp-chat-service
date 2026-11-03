import { act, createMockChatAPI, renderHook, waitFor } from '../test-utils';
import { describe, expect, it, vi } from 'vitest';
import { useChatPanel } from './useChatPanel';

function doneResponse(sessionId = 'session-1'): Response {
  const encoder = new TextEncoder();
  return new Response(new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(
        `event: done\ndata: {"session_id":"${sessionId}"}\n\n`,
      ));
      controller.close();
    },
  }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
}

function controlledResponse() {
  const encoder = new TextEncoder();
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const response = new Response(new ReadableStream<Uint8Array>({
    start(value) {
      controller = value;
    },
  }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
  return {
    response,
    done(sessionId = 'session-1') {
      controller.enqueue(encoder.encode(
        `event: done\ndata: {"session_id":"${sessionId}"}\n\n`,
      ));
    },
    close() {
      controller.close();
    },
  };
}

describe('useChatPanel interactive submission queue', () => {
  it('dispatches one immutable payload only after done is committed and the prior stream closes', async () => {
    const first = controlledResponse();
    const stream = vi.fn()
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(doneResponse('session-1'));
    const api = createMockChatAPI({ stream });
    const { result } = renderHook(
      () => useChatPanel({ queueInteractiveSubmissions: true }),
      { api },
    );

    act(() => {
      void result.current.sendMessage('first turn');
    });
    await waitFor(() => expect(result.current.streaming).toBe(true));

    let payload = 'Run render_placement with choice=one  ';
    act(() => {
      expect(result.current.sendInteractiveMessage(payload)).toBe(true);
      payload = 'mutated after click';
    });
    expect(result.current.interactiveSubmissionState).toBe('queued');
    expect(stream).toHaveBeenCalledTimes(1);

    act(() => first.done());
    await Promise.resolve();
    expect(stream).toHaveBeenCalledTimes(1);

    act(() => first.close());
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(2));
    const queuedBody = stream.mock.calls[1][1] as Record<string, unknown>;
    expect(queuedBody.message).toBe('Run render_placement with choice=one  ');
    expect(queuedBody.session_id).toBe('session-1');
    await waitFor(() => {
      expect(result.current.interactiveSubmissionState).toBe('idle');
      expect(result.current.streaming).toBe(false);
    });
  });

  it('rejects duplicate clicks while the one slot is occupied', async () => {
    const first = controlledResponse();
    const stream = vi.fn()
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(doneResponse());
    const { result } = renderHook(
      () => useChatPanel({ queueInteractiveSubmissions: true }),
      { api: createMockChatAPI({ stream }) },
    );

    act(() => {
      void result.current.sendMessage('first');
    });
    await waitFor(() => expect(result.current.streaming).toBe(true));
    act(() => {
      expect(result.current.sendInteractiveMessage('queued once')).toBe(true);
      expect(result.current.sendInteractiveMessage('duplicate')).toBe(false);
    });
    act(() => {
      first.done();
      first.close();
    });
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(2));
    expect((stream.mock.calls[1][1] as Record<string, unknown>).message).toBe('queued once');
  });

  it('rejects a second immediate action before React publishes streaming state', async () => {
    const first = controlledResponse();
    const stream = vi.fn().mockResolvedValueOnce(first.response);
    const { result } = renderHook(
      () => useChatPanel({ queueInteractiveSubmissions: true }),
      { api: createMockChatAPI({ stream }) },
    );

    act(() => {
      expect(result.current.sendInteractiveMessage('first action')).toBe(true);
      expect(result.current.sendInteractiveMessage('same-click duplicate')).toBe(false);
    });
    expect(stream).toHaveBeenCalledTimes(1);
    expect((stream.mock.calls[0][1] as Record<string, unknown>).message).toBe('first action');
    await waitFor(() => {
      expect(result.current.streaming).toBe(true);
      expect(result.current.interactiveSubmissionState).toBe('continuing');
    });

    act(() => {
      first.done();
      first.close();
    });
    await waitFor(() => {
      expect(result.current.streaming).toBe(false);
      expect(result.current.interactiveSubmissionState).toBe('idle');
    });
  });

  it('drops the queued slot on unmount and session clear', async () => {
    for (const scenario of ['unmount', 'clear'] as const) {
      const first = controlledResponse();
      const stream = vi.fn().mockResolvedValueOnce(first.response);
      const hook = renderHook(
        () => useChatPanel({ queueInteractiveSubmissions: true }),
        { api: createMockChatAPI({ stream }) },
      );
      act(() => {
        void hook.result.current.sendMessage('first');
      });
      await waitFor(() => expect(hook.result.current.streaming).toBe(true));
      act(() => {
        hook.result.current.sendInteractiveMessage('must not dispatch');
      });

      if (scenario === 'unmount') {
        hook.unmount();
      } else {
        await act(async () => {
          await hook.result.current.clear();
        });
      }
      act(() => {
        first.done();
        first.close();
      });
      await Promise.resolve();
      await Promise.resolve();
      expect(stream).toHaveBeenCalledTimes(1);
      if (scenario === 'clear') {
        expect(hook.result.current.interactiveSubmissionState).toBe('idle');
      }
      if (scenario !== 'unmount') hook.unmount();
    }
  });

  it('continues the queued action after the prior request fails', async () => {
    let resolveFirst!: (response: Response) => void;
    const firstResponse = new Promise<Response>((resolve) => {
      resolveFirst = resolve;
    });
    const stream = vi.fn()
      .mockReturnValueOnce(firstResponse)
      .mockResolvedValueOnce(doneResponse());
    const { result } = renderHook(
      () => useChatPanel({ queueInteractiveSubmissions: true }),
      { api: createMockChatAPI({ stream }) },
    );

    act(() => {
      void result.current.sendMessage('first');
    });
    await waitFor(() => expect(result.current.streaming).toBe(true));
    act(() => {
      result.current.sendInteractiveMessage('continue after failure');
      resolveFirst(new Response('failed', { status: 500 }));
    });
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(2));
    expect((stream.mock.calls[1][1] as Record<string, unknown>).message)
      .toBe('continue after failure');
  });

  it('preserves legacy drop-while-streaming behavior unless opted in', async () => {
    const first = controlledResponse();
    const stream = vi.fn().mockResolvedValueOnce(first.response);
    const { result } = renderHook(() => useChatPanel(), {
      api: createMockChatAPI({ stream }),
    });

    act(() => {
      void result.current.sendMessage('first');
    });
    await waitFor(() => expect(result.current.streaming).toBe(true));
    act(() => {
      expect(result.current.sendInteractiveMessage('legacy drop')).toBe(false);
    });
    expect(result.current.interactiveSubmissionState).toBe('idle');
    act(() => {
      first.done();
      first.close();
    });
    await waitFor(() => expect(result.current.streaming).toBe(false));
    expect(stream).toHaveBeenCalledTimes(1);
  });
});
