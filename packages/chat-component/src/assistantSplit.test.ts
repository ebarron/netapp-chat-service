import { describe, expect, it } from 'vitest';
import {
  ASSISTANT_WIDTH_PCT_MAX,
  ASSISTANT_WIDTH_PCT_MIN,
  clampAssistantWidthPx,
  formatAssistantWidth,
  getAssistantWidthBounds,
  isAssistantSizingActive,
  parseAssistantPixels,
  resolveAssistantWidthPx,
} from './assistantSplit';

describe('assistantSplit utilities', () => {
  it('formatAssistantWidth converts numbers to px and preserves strings', () => {
    expect(formatAssistantWidth(480)).toBe('480px');
    expect(formatAssistantWidth('24%')).toBe('24%');
  });

  it('parseAssistantPixels reads pixel strings only', () => {
    expect(parseAssistantPixels('480px')).toBe(480);
    expect(parseAssistantPixels('40%')).toBeNull();
  });

  it('isAssistantSizingActive stays false with legacy defaults', () => {
    expect(isAssistantSizingActive({})).toBe(false);
    expect(isAssistantSizingActive({ assistantMinWidth: 320 })).toBe(false);
  });

  it('isAssistantSizingActive detects opt-in props', () => {
    expect(isAssistantSizingActive({ assistantWidth: '30%' })).toBe(true);
    expect(isAssistantSizingActive({ resizableAssistant: true })).toBe(true);
    expect(isAssistantSizingActive({ assistantMinWidth: 400 })).toBe(true);
  });

  it('clampAssistantWidthPx enforces px, pct, and max bounds', () => {
    const containerWidth = 1000;
    expect(clampAssistantWidthPx(100, containerWidth, 320)).toBe(320);
    expect(clampAssistantWidthPx(900, containerWidth, 320)).toBe(
      containerWidth * (ASSISTANT_WIDTH_PCT_MAX / 100),
    );
    expect(clampAssistantWidthPx(500, containerWidth, 320, 450)).toBe(450);
    expect(clampAssistantWidthPx(200, containerWidth, 320)).toBe(320);
    expect(clampAssistantWidthPx(120, containerWidth, 100)).toBe(
      containerWidth * (ASSISTANT_WIDTH_PCT_MIN / 100),
    );
  });

  it('getAssistantWidthBounds aligns with clamp limits', () => {
    const bounds = getAssistantWidthBounds(1000, 320, 700);
    expect(bounds.minPx).toBe(320);
    expect(bounds.maxPx).toBe(600);
    expect(clampAssistantWidthPx(9999, 1000, 320, 700)).toBe(bounds.maxPx);
  });

  it('resolveAssistantWidthPx handles numbers, px strings, and percentages', () => {
    expect(resolveAssistantWidthPx(500, 1000, 320)).toBe(500);
    expect(resolveAssistantWidthPx('450px', 1000, 320)).toBe(450);
    expect(resolveAssistantWidthPx('40%', 1000, 320)).toBe(400);
    expect(resolveAssistantWidthPx(undefined, 1000, 320)).toBe(400);
  });
});
