import { renderHook, act, createMockChatAPI } from '../test-utils';
import { describe, it, expect } from 'vitest';
import { useChatPanel } from './useChatPanel';

describe('useChatPanel defaultMode option', () => {
  it('defaults to read-write when defaultMode is omitted', () => {
    const api = createMockChatAPI();
    const { result } = renderHook(() => useChatPanel(), { api });
    expect(result.current.mode).toBe('read-write');
  });

  it('uses defaultMode="read-only" when explicitly provided', () => {
    const api = createMockChatAPI();
    const { result } = renderHook(
      () => useChatPanel({ defaultMode: 'read-only' }),
      { api },
    );
    expect(result.current.mode).toBe('read-only');
  });

  it('uses defaultMode="read-write" when explicitly provided', () => {
    const api = createMockChatAPI();
    const { result } = renderHook(
      () => useChatPanel({ defaultMode: 'read-write' }),
      { api },
    );
    expect(result.current.mode).toBe('read-write');
  });

  it('runtime setMode still works after defaultMode is set', () => {
    const api = createMockChatAPI();
    const { result } = renderHook(
      () => useChatPanel({ defaultMode: 'read-only' }),
      { api },
    );
    expect(result.current.mode).toBe('read-only');

    act(() => {
      result.current.setMode('read-write');
    });
    expect(result.current.mode).toBe('read-write');

    act(() => {
      result.current.setMode('read-only');
    });
    expect(result.current.mode).toBe('read-only');
  });
});
