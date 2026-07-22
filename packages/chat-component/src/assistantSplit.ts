export const DEFAULT_ASSISTANT_WIDTH = '40%';
export const DEFAULT_ASSISTANT_MIN_WIDTH = 320;
export const ASSISTANT_WIDTH_PCT_MIN = 15;
export const ASSISTANT_WIDTH_PCT_MAX = 60;

export interface AssistantSizingProps {
  assistantWidth?: number | string;
  defaultAssistantWidth?: number | string;
  resizableAssistant?: boolean;
  assistantMinWidth?: number;
  assistantMaxWidth?: number;
}

/** True when any opt-in assistant-width prop is used (triggers sized layout mode). */
export function isAssistantSizingActive({
  assistantWidth,
  defaultAssistantWidth,
  resizableAssistant,
  assistantMinWidth = DEFAULT_ASSISTANT_MIN_WIDTH,
  assistantMaxWidth,
}: AssistantSizingProps): boolean {
  return (
    assistantWidth !== undefined ||
    defaultAssistantWidth !== undefined ||
    resizableAssistant === true ||
    assistantMinWidth !== DEFAULT_ASSISTANT_MIN_WIDTH ||
    assistantMaxWidth !== undefined
  );
}

/** Format a host-provided width for use in CSS `flex-basis`. */
export function formatAssistantWidth(value: number | string): string {
  return typeof value === 'number' ? `${value}px` : value;
}

/** Parse a pixel width string such as `"480px"`. Returns null for non-px values. */
export function parseAssistantPixels(width: string): number | null {
  const match = width.trim().match(/^([\d.]+)px$/);
  return match ? Number(match[1]) : null;
}

export function getAssistantWidthBounds(
  containerWidth: number,
  assistantMinWidth = DEFAULT_ASSISTANT_MIN_WIDTH,
  assistantMaxWidth?: number,
): { minPx: number; maxPx: number } {
  const pctMin = containerWidth * (ASSISTANT_WIDTH_PCT_MIN / 100);
  const pctMax = containerWidth * (ASSISTANT_WIDTH_PCT_MAX / 100);
  let minPx = Math.max(assistantMinWidth, pctMin);
  let maxPx = Math.min(pctMax, containerWidth);
  if (assistantMaxWidth !== undefined) {
    maxPx = Math.min(maxPx, assistantMaxWidth);
  }
  if (minPx > maxPx) {
    minPx = maxPx;
  }
  return { minPx: Math.round(minPx), maxPx: Math.round(maxPx) };
}

/** Clamp an assistant column width in pixels to min/max and percentage bounds. */
export function clampAssistantWidthPx(
  px: number,
  containerWidth: number,
  assistantMinWidth = DEFAULT_ASSISTANT_MIN_WIDTH,
  assistantMaxWidth?: number,
): number {
  const { minPx, maxPx } = getAssistantWidthBounds(
    containerWidth,
    assistantMinWidth,
    assistantMaxWidth,
  );
  return Math.round(Math.max(minPx, Math.min(maxPx, px)));
}

/** Resolve the default reset width (double-click / keyboard reset target). */
export function resolveDefaultAssistantWidth(
  defaultAssistantWidth?: number | string,
): string {
  if (defaultAssistantWidth !== undefined) {
    return formatAssistantWidth(defaultAssistantWidth);
  }
  return DEFAULT_ASSISTANT_WIDTH;
}

/** Convert a configured width to pixels when possible for callbacks and ARIA. */
export function resolveAssistantWidthPx(
  width: number | string | undefined,
  containerWidth: number,
  assistantMinWidth = DEFAULT_ASSISTANT_MIN_WIDTH,
  assistantMaxWidth?: number,
): number {
  if (width === undefined) {
    return clampAssistantWidthPx(
      containerWidth * 0.4,
      containerWidth,
      assistantMinWidth,
      assistantMaxWidth,
    );
  }
  if (typeof width === 'number') {
    return clampAssistantWidthPx(width, containerWidth, assistantMinWidth, assistantMaxWidth);
  }
  const px = parseAssistantPixels(width);
  if (px !== null) {
    return clampAssistantWidthPx(px, containerWidth, assistantMinWidth, assistantMaxWidth);
  }
  const pctMatch = width.trim().match(/^([\d.]+)%$/);
  if (pctMatch) {
    return clampAssistantWidthPx(
      (containerWidth * Number(pctMatch[1])) / 100,
      containerWidth,
      assistantMinWidth,
      assistantMaxWidth,
    );
  }
  return clampAssistantWidthPx(
    containerWidth * 0.4,
    containerWidth,
    assistantMinWidth,
    assistantMaxWidth,
  );
}
