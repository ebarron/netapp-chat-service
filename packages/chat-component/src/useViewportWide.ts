import { useEffect, useState } from 'react';

/**
 * Reactive "viewport is at least `breakpoint` px wide" signal.
 *
 * SSR-safe: defaults to `true` (wide) when `window`/`matchMedia` is unavailable,
 * and treats only an explicit `matches === false` as narrow — matching the
 * consumer-side pattern in NABox `useAiForward` / RTB `useAiReadiness`.
 *
 * When `enabled` is false the hook stays idle and returns `true` (wide), so
 * callers can leave it mounted without affecting the legacy narrow path.
 */
export function useViewportWide(breakpoint: number, enabled: boolean): boolean {
  const [wide, setWide] = useState(() => {
    if (!enabled || typeof window === 'undefined') return true;
    if (typeof window.matchMedia === 'function') {
      return window.matchMedia(`(min-width: ${breakpoint}px)`).matches;
    }
    return window.innerWidth >= breakpoint;
  });

  useEffect(() => {
    if (!enabled || typeof window === 'undefined') {
      setWide(true);
      return;
    }

    const query = `(min-width: ${breakpoint}px)`;

    if (typeof window.matchMedia === 'function') {
      const mql = window.matchMedia(query);
      const update = () => setWide(mql.matches);
      update();
      // Prefer the modern API; fall back for older environments.
      if (typeof mql.addEventListener === 'function') {
        mql.addEventListener('change', update);
        return () => mql.removeEventListener('change', update);
      }
      mql.addListener(update);
      return () => mql.removeListener(update);
    }

    const update = () => setWide(window.innerWidth >= breakpoint);
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, [breakpoint, enabled]);

  return enabled ? wide : true;
}
