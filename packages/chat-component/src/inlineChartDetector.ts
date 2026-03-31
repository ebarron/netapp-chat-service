/**
 * Detects bare chart/dashboard JSON in assistant messages and wraps them in
 * fenced code blocks so ReactMarkdown + the existing code-block handlers
 * can render them as visual panels.
 *
 * LLMs sometimes emit the JSON inline (no code fence) even when instructed
 * to use ```chart / ```dashboard fences. This module provides a robust
 * fallback for that case.
 */

import { inferChartType } from './charts/chartTypes';

const CHART_TYPES = new Set([
  'area',
  'bar',
  'gauge',
  'sparkline',
  'status-grid',
  'stat',
  'alert-summary',
  'resource-table',
  'alert-list',
  'callout',
  'proposal',
  'action-button',
]);

/**
 * Strip JS-style comments and trailing commas — the most common JSON errors from LLMs.
 */
export function sanitizeJson(text: string): string {
  // Remove single-line // comments (but not inside strings — good-enough heuristic).
  const noComments = text.replace(/^(\s*)\/\/.*$/gm, '$1');
  // Remove trailing commas before ] or }.
  return noComments.replace(/,\s*([\]}])/g, '$1');
}

/**
 * Extract a balanced JSON object starting at `start` (which must be a '{').
 * Returns the substring including the braces, or null if braces never balance.
 */
function extractJsonObject(text: string, start: number): string | null {
  let depth = 0;
  let inString = false;
  let escaped = false;

  for (let i = start; i < text.length; i++) {
    const ch = text[i];

    if (escaped) {
      escaped = false;
      continue;
    }
    if (ch === '\\' && inString) {
      escaped = true;
      continue;
    }
    if (ch === '"') {
      inString = !inString;
      continue;
    }
    if (inString) continue;

    if (ch === '{') depth++;
    else if (ch === '}') {
      depth--;
      if (depth === 0) {
        return text.slice(start, i + 1);
      }
    }
  }
  return null;
}

/**
 * Classify a parsed JSON object into a rendering category.
 * Returns a code-fence language tag, or null for non-structured data.
 * If the object lacks a `type` field, attempts shape-based inference.
 * Falls back to 'json' for any structured object so the code handler
 * can render it via AutoJsonBlock instead of showing raw text.
 */
function classify(
  obj: unknown,
): 'chart' | 'dashboard' | 'object-detail' | 'json' | null {
  if (typeof obj !== 'object' || obj === null) return null;
  const rec = obj as Record<string, unknown>;

  // Dashboard: has panels array
  if (Array.isArray(rec.panels)) return 'dashboard';

  // Object-detail: has kind + name + sections, or type === 'object-detail'
  if (rec.type === 'object-detail') return 'object-detail';
  if (typeof rec.kind === 'string' && typeof rec.name === 'string' && Array.isArray(rec.sections)) {
    return 'object-detail';
  }

  // Chart: has a known type field
  if (typeof rec.type === 'string' && CHART_TYPES.has(rec.type)) return 'chart';

  // No type — try shape-based inference (handles LLMs omitting the discriminator)
  const inferred = inferChartType(rec);
  if (inferred && inferred !== 'object-detail') {
    rec.type = inferred;
    return 'chart';
  }

  // Generic structured JSON — wrap so the code handler can auto-render it.
  // Require at least 3 keys or a nested array of objects to avoid wrapping
  // trivial objects like {"ok": true}.
  const keys = Object.keys(rec);
  const hasNestedArray = Object.values(rec).some(
    (v) => Array.isArray(v) && v.length > 0 && typeof v[0] === 'object',
  );
  if (keys.length >= 3 || hasNestedArray) return 'json';

  return null;
}

/**
 * Pre-process assistant message content: find bare chart/dashboard JSON
 * objects that are NOT already inside fenced code blocks and wrap them.
 */
export function wrapInlineChartJson(content: string): string {
  // Split around existing fenced code blocks to avoid double-wrapping.
  // This regex captures ``` followed by any language tag, content, and closing ```.
  const fencePattern = /```[\s\S]*?```/g;
  let lastEnd = 0;
  const segments: Array<{ text: string; fenced: boolean }> = [];

  for (const match of content.matchAll(fencePattern)) {
    const idx = match.index!;
    if (idx > lastEnd) {
      segments.push({ text: content.slice(lastEnd, idx), fenced: false });
    }
    segments.push({ text: match[0], fenced: true });
    lastEnd = idx + match[0].length;
  }
  if (lastEnd < content.length) {
    segments.push({ text: content.slice(lastEnd), fenced: false });
  }

  // Process non-fenced segments for bare JSON, and re-tag ```json fences
  // that contain chart/dashboard data.
  return segments
    .map((seg) => (seg.fenced ? retagJsonFence(seg.text) : processSegment(seg.text)))
    .join('');
}

/**
 * If a fenced block uses a non-chart language tag (```json, ```, ```text,
 * etc.) but contains chart/dashboard data, re-tag it so ReactMarkdown
 * routes it to the correct renderer.
 */
function retagJsonFence(text: string): string {
  // Match fences that are NOT already tagged as chart, dashboard, or object-detail.
  // Captures an optional language tag and the body.
  const match = text.match(/^```(?!(chart|dashboard|object-detail)\b)([\w-]*)\s*\n([\s\S]*)\n```$/);
  if (!match) return text;

  const body = match[3];
  try {
    const parsed = JSON.parse(sanitizeJson(body));
    const kind = classify(parsed);
    if (kind) {
      return '```' + kind + '\n' + body + '\n```';
    }
  } catch { /* not valid JSON */ }

  return text;
}

/**
 * Scan a text segment (known to be outside fenced code blocks) for JSON
 * objects that look like chart or dashboard data and wrap them in fences.
 */
function processSegment(text: string): string {
  const parts: string[] = [];
  let i = 0;

  while (i < text.length) {
    const braceIdx = text.indexOf('{', i);
    if (braceIdx === -1) {
      parts.push(text.slice(i));
      break;
    }

    // Push text before the brace.
    parts.push(text.slice(i, braceIdx));

    // Try to extract a balanced JSON object.
    const jsonStr = extractJsonObject(text, braceIdx);
    if (jsonStr) {
      try {
        const parsed = JSON.parse(sanitizeJson(jsonStr));
        const kind = classify(parsed);
        if (kind) {
          // Strip single backticks that LLMs sometimes wrap around inline JSON.
          // A leading ` before { and trailing ` after } would cause ReactMarkdown
          // to treat the entire fenced block as inline <code>, defeating rendering.
          const lastPart = parts[parts.length - 1];
          if (lastPart && lastPart.endsWith('`')) {
            parts[parts.length - 1] = lastPart.slice(0, -1);
          }
          let endIdx = braceIdx + jsonStr.length;
          if (text[endIdx] === '`') {
            endIdx += 1; // skip trailing backtick
          }
          parts.push(`\n\n\`\`\`${kind}\n${jsonStr}\n\`\`\`\n\n`);
          i = endIdx;
          continue;
        }
      } catch {
        // Not valid JSON — fall through.
      }
    }

    // Not a chart JSON — keep the brace literal and advance past it.
    parts.push('{');
    i = braceIdx + 1;
  }

  return parts.join('');
}

/** Sentinel used by hideIncompleteChartJson to replace partial JSON blocks. */
const STREAMING_PLACEHOLDER = '\n\n*Building dashboard…*\n\n';

/**
 * Patterns that strongly indicate an incomplete JSON blob is chart/dashboard
 * data (partial keys that appear early in the streamed JSON).
 */
const CHART_HINTS = /["'](?:panels|sections|kind|type["']\s*:\s*["'](?:area|bar|gauge|sparkline|status-grid|stat|alert-summary|resource-table|alert-list|callout|proposal|action-button|object-detail))/;

/**
 * During streaming, incomplete JSON that looks like a chart/dashboard is
 * replaced with a user-friendly placeholder so the user doesn't see a
 * wall of raw JSON tokens. Call this INSTEAD of wrapInlineChartJson while
 * the message is still being streamed.
 */
export function hideIncompleteChartJson(content: string): string {
  // First, process any already-complete blocks normally.
  const processed = wrapInlineChartJson(content);

  // Check for an unclosed ``` fence at the tail.
  // Odd total count of ``` means the last one is unclosed.
  const fences = processed.match(/```/g);
  if (fences && fences.length % 2 === 1) {
    const lastFence = processed.lastIndexOf('```');
    const fenceBody = processed.slice(lastFence + 3);
    if (CHART_HINTS.test(fenceBody)) {
      return processed.slice(0, lastFence) + STREAMING_PLACEHOLDER;
    }
  }

  // Check for bare incomplete JSON — scan forward for the first
  // unbalanced '{' whose trailing text contains chart/dashboard patterns.
  let pos = 0;
  while (pos < processed.length) {
    const braceIdx = processed.indexOf('{', pos);
    if (braceIdx === -1) break;

    const extracted = extractJsonObject(processed, braceIdx);
    if (extracted === null) {
      const tail = processed.slice(braceIdx);
      if (CHART_HINTS.test(tail)) {
        return processed.slice(0, braceIdx) + STREAMING_PLACEHOLDER;
      }
      break; // remaining text is all inside this unbalanced tail
    }
    pos = braceIdx + extracted.length;
  }

  return processed;
}
