import { renderHook, act } from '../test-utils';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { useViewportWide } from './useViewportWide';

describe('useViewportWide', () => {
  let listeners: Array<(ev: MediaQueryListEvent) => void>;

  beforeEach(() => {
    listeners = [];
    window.matchMedia = vi.fn().mockImplementation((query: string) => {
      const min = Number(/min-width:\s*(\d+)px/.exec(query)?.[1] ?? 0);
      const mql = {
        get matches() {
          return window.innerWidth >= min;
        },
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn((_type: string, cb: (ev: MediaQueryListEvent) => void) => {
          listeners.push(cb);
        }),
        removeEventListener: vi.fn((_type: string, cb: (ev: MediaQueryListEvent) => void) => {
          listeners = listeners.filter((l) => l !== cb);
        }),
        dispatchEvent: vi.fn(),
      };
      return mql as unknown as MediaQueryList;
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 });
  });

  it('defaults to wide when disabled', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 400 });
    const { result } = renderHook(() => useViewportWide(1024, false));
    expect(result.current).toBe(true);
  });

  it('reads the initial match synchronously when enabled', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 });
    const { result } = renderHook(() => useViewportWide(1024, true));
    expect(result.current).toBe(false);
  });

  it('flips live when matchMedia fires a change event', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 });
    const { result } = renderHook(() => useViewportWide(1024, true));
    expect(result.current).toBe(false);

    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
    act(() => {
      for (const cb of listeners) {
        cb({ matches: true } as MediaQueryListEvent);
      }
    });
    expect(result.current).toBe(true);
  });

  it('defaults to wide when enabled is false even if the viewport is narrow', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 400 });
    const { result } = renderHook(() => useViewportWide(1024, false));
    expect(result.current).toBe(true);
  });
});
