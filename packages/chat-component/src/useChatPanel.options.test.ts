import { renderHook, act, createMockChatAPI } from '../test-utils';
import { vi, describe, it, expect } from 'vitest';
import { useChatPanel } from './useChatPanel';

/** Minimal SSE-streaming Response for stream() mocks. */
function makeSSEResponse(): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('event: done\ndata: {"session_id":"s1"}\n\n'));
      controller.close();
    },
  });
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
}

describe('useChatPanel structured options relay (C1)', () => {
  it('forwards a summary\'s options into the canvas_tabs wire payload', async () => {
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
        title: 'AI Settings',
        kind: 'nav-view',
        qualifier: '/settings/ai',
        summary: {
          kind: 'nav-view',
          name: 'AI Settings',
          qualifier: '/settings/ai',
          options: [
            { label: 'Provider', choices: ['OpenAI', 'Anthropic', 'LLM Proxy'] },
            { label: 'Model', choices: ['gpt-4.1', 'claude-sonnet-4'] },
          ],
        },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('what providers can I pick?');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs[0].options).toEqual([
      { label: 'Provider', choices: ['OpenAI', 'Anthropic', 'LLM Proxy'] },
      { label: 'Model', choices: ['gpt-4.1', 'claude-sonnet-4'] },
    ]);
  });

  it('drops empty/blank choices and controls with no choices', async () => {
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
        title: 'X',
        summary: {
          options: [
            { label: 'Empty', choices: ['', '  '] },
            { label: 'Real', choices: [' a ', 'b', ''] },
          ],
        },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('hi');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect(tabs[0].options).toEqual([{ label: 'Real', choices: ['a', 'b'] }]);
  });

  it('omits options from the wire when none are supplied (backward compat)', async () => {
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
        title: 'Home',
        summary: { kind: 'nav-view', name: 'Home', qualifier: '/home' },
      }),
    );

    await act(async () => {
      await result.current.sendMessage('hi');
    });

    const tabs = body!.canvas_tabs as Array<Record<string, unknown>>;
    expect('options' in tabs[0]).toBe(false);
  });
});
