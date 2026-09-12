package agent

import (
	"log/slog"
	"strings"
)

// canvasFenceInterceptor buffers streamed text tokens and detects
// canvas-object-detail / canvas-dashboard code fences. When a complete
// canvas fence is found, it emits an EventCanvasOpen event and suppresses
// the fence text from the regular EventText stream.
//
// The interceptor wraps the emit function: pass each EventText through
// HandleToken. Non-text events should be forwarded directly.
type canvasFenceInterceptor struct {
	emit      func(Event) // downstream emit
	buffer    strings.Builder
	inside    bool // true when we're inside a canvas fence
	lastValid map[string][]byte
}

// canvasFencePrefixes are the code fence openings we intercept.
var canvasFencePrefixes = []string{
	"```canvas-object-detail",
	"```canvas-dashboard",
}

// newCanvasFenceInterceptor creates an interceptor that wraps emit.
func newCanvasFenceInterceptor(emit func(Event)) *canvasFenceInterceptor {
	return &canvasFenceInterceptor{
		emit:      emit,
		lastValid: make(map[string][]byte),
	}
}

// HandleToken processes a text token. It buffers tokens that might be
// part of a canvas fence and emits them downstream once resolved.
func (c *canvasFenceInterceptor) HandleToken(text string) {
	c.buffer.WriteString(text)
	buf := c.buffer.String()

	slog.Debug("canvas interceptor", "tokenLen", len(text), "bufLen", len(buf), "inside", c.inside)

	if c.inside {
		// We're inside a canvas fence — look for the closing ```.
		closingIdx := strings.Index(buf, "\n```")
		if closingIdx < 0 {
			// Also check for ``` at end without trailing newline.
			if strings.HasSuffix(buf, "\n```") {
				closingIdx = len(buf) - 4
			}
		}
		if closingIdx >= 0 {
			// Extract the JSON content (everything before the closing fence).
			jsonContent := strings.TrimSpace(buf[:closingIdx])
			c.emitCanvasEvent(jsonContent)
			// Keep anything after the closing fence marker.
			remaining := buf[closingIdx:]
			// Skip past the closing ``` and optional trailing newline.
			if idx := strings.Index(remaining, "```"); idx >= 0 {
				remaining = remaining[idx+3:]
				remaining = strings.TrimPrefix(remaining, "\n")
			}
			c.buffer.Reset()
			c.inside = false
			if remaining != "" {
				c.HandleToken(remaining)
			}
			return
		}
		// Still accumulating — don't emit yet.
		return
	}

	// Not inside a fence — check if we're starting one.
	for _, prefix := range canvasFencePrefixes {
		if idx := strings.Index(buf, prefix); idx >= 0 {
			slog.Debug("canvas interceptor: FENCE MATCH", "prefix", prefix, "idx", idx)
			// Emit any text before the fence.
			before := buf[:idx]
			if before != "" {
				c.emit(Event{Type: EventText, Text: before})
			}
			// Enter fence mode with the content after the opening line.
			afterPrefix := buf[idx+len(prefix):]
			// Skip the rest of the opening line (e.g., trailing newline).
			if nlIdx := strings.IndexByte(afterPrefix, '\n'); nlIdx >= 0 {
				afterPrefix = afterPrefix[nlIdx+1:]
			} else {
				afterPrefix = ""
			}
			c.buffer.Reset()
			c.buffer.WriteString(afterPrefix)
			c.inside = true
			// Re-check in case the fence is already complete.
			c.HandleToken("")
			return
		}
	}

	// Check if the buffer ends with a partial match of ANY canvas prefix.
	// Only hold the buffer if the suffix is still a viable start of a canvas
	// fence. This avoids holding regular ```dashboard or ```object-detail
	// fences that share a common ``` prefix with canvas fences.
	if canvasPartialMatch(buf) {
		slog.Debug("canvas interceptor: PARTIAL MATCH, holding buffer", "bufLen", len(buf))
		return
	}

	// No fence detected and no partial match — emit the buffered text.
	slog.Debug("canvas interceptor: NO MATCH, emitting", "bufLen", len(buf))
	c.emit(Event{Type: EventText, Text: buf})
	c.buffer.Reset()
}

// Flush emits any remaining buffered text. Call when the stream ends.
func (c *canvasFenceInterceptor) Flush() {
	buf := c.buffer.String()
	if buf == "" {
		if c.inside {
			slog.Warn("canvas payload rejected",
				"reason", "incomplete_stream",
				"path", "$",
				"content_len", 0,
			)
			c.inside = false
		}
		return
	}
	slog.Debug("canvas interceptor: FLUSH", "inside", c.inside, "bufLen", len(buf))
	if c.inside {
		// A partial structured payload must not leak into normal text or UI
		// state. Report only shape metadata, never model-generated content.
		slog.Warn("canvas payload rejected",
			"reason", "incomplete_stream",
			"path", "$",
			"content_len", len(buf),
		)
	} else {
		c.emit(Event{Type: EventText, Text: buf})
	}
	c.buffer.Reset()
	c.inside = false
}

// emitCanvasEvent validates and normalizes the JSON content before emitting an
// EventCanvasOpen. Invalid payloads are rejected, and exact valid duplicates
// are suppressed. Updated valid payloads with the same tab identity still emit.
func (c *canvasFenceInterceptor) emitCanvasEvent(jsonContent string) {
	slog.Debug("canvas interceptor: EMITTING CANVAS EVENT", "contentLen", len(jsonContent))
	normalized, meta, err := normalizeCanvasPayload(jsonContent)
	if err != nil {
		issue, ok := err.(*canvasPayloadIssue)
		reason, path := "invalid_canvas_payload", "$"
		if ok {
			reason, path = issue.Code, issue.Path
		}
		_, duplicate := c.lastValid[meta.TabID]
		slog.Warn("canvas payload rejected",
			"reason", reason,
			"path", path,
			"payload_type", meta.PayloadType,
			"duplicate", duplicate,
			"content_len", len(jsonContent),
		)
		return
	}

	if previous, ok := c.lastValid[meta.TabID]; ok && sameCanvasPayload(previous, normalized) {
		slog.Debug("canvas payload duplicate suppressed",
			"payload_type", meta.PayloadType,
			"content_len", len(normalized),
		)
		return
	}
	c.lastValid[meta.TabID] = append([]byte(nil), normalized...)

	c.emit(Event{
		Type: EventCanvasOpen,
		Canvas: &CanvasPayload{
			TabID:     meta.TabID,
			Title:     meta.Title,
			Kind:      meta.Kind,
			Qualifier: meta.Qualifier,
			Content:   normalized,
		},
	})
}

// canvasPartialMatch returns true if the end of s could still become a
// canvas fence opening. All canvas fence openings start with "```canvas-".
//
// We hold the buffer in two situations:
//  1. The suffix matches ≥4 characters of a canvas prefix (e.g. "```c",
//     "```canvas", "```canvas-d"). At that point we're past the ambiguous
//     "```" shared with regular fences like ```dashboard.
//  2. The buffer ends with 1–3 backticks that sit at a fence-start position
//     (beginning of string or preceded by a newline). This covers the case
//     where the LLM tokenizes the backticks separately from "canvas-...".
//     Mid-content backticks (e.g. inline code) are NOT held.
func canvasPartialMatch(s string) bool {
	// Case 1: suffix matches 4+ chars of a canvas prefix.
	maxLen := 0
	for _, p := range canvasFencePrefixes {
		if len(p) > maxLen {
			maxLen = len(p)
		}
	}
	limit := maxLen
	if limit > len(s) {
		limit = len(s)
	}
	for _, prefix := range canvasFencePrefixes {
		for i := 4; i <= limit && i <= len(prefix); i++ {
			if s[len(s)-i:] == prefix[:i] {
				return true
			}
		}
	}

	// Case 2: trailing backticks at a fence-start position.
	// Count trailing backticks.
	ticks := 0
	for i := len(s) - 1; i >= 0 && s[i] == '`'; i-- {
		ticks++
	}
	if ticks >= 1 && ticks <= 3 {
		pos := len(s) - ticks
		if pos == 0 || s[pos-1] == '\n' {
			return true
		}
	}
	return false
}
