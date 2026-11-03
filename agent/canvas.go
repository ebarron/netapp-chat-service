package agent

import (
	"encoding/json"
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

var inlineRenderFencePrefixes = []string{
	"```object-detail",
	"```dashboard",
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
				// Fence separators are transport formatting, not assistant
				// prose. EmitResult renderers commonly append two newlines;
				// discard all immediate line breaks so a successful canvas
				// does not create an empty chat message after canvas_open.
				remaining = strings.TrimLeft(remaining, "\r\n")
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

// ParseTerminalCanvasResult parses a successful tool result that is exactly
// one complete, valid, actionable canvas open/update fence. Informational
// views and close directives are excluded: they generally require explanation
// or confirmation and must continue through the provider.
//
// Hosts with a deterministic pre-provider render path may use this helper to
// enforce the same validation contract as Agent.Run.
func ParseTerminalCanvasResult(result string) (*CanvasPayload, bool) {
	parsed, ok := parseCompleteRenderResult(
		result,
		canvasFencePrefixes,
		TerminalRenderPresentationCanvas,
	)
	if !ok {
		return nil, false
	}
	if parsed.metadata != nil &&
		parsed.metadata.Presentation != TerminalRenderPresentationCanvas {
		return nil, false
	}
	payload := parsed.payload
	if !hasInteractiveCanvasControl(payload.Content) {
		return nil, false
	}
	return payload, true
}

// ParseTerminalCanvasResultWithHandoff validates a complete Canvas result
// against renderer-owned handoff metadata. Forms must contain an actionable
// control; results must contain a nonempty, non-error destination. The
// validated metadata is attached only to the emitted Canvas event and is not
// persisted as assistant prose.
func ParseTerminalCanvasResultWithHandoff(
	result string,
) (*CanvasPayload, bool) {
	parsed, ok := parseCompleteRenderResult(
		result,
		canvasFencePrefixes,
		TerminalRenderPresentationCanvas,
	)
	if !ok || parsed.metadata == nil ||
		parsed.metadata.Presentation != TerminalRenderPresentationCanvas {
		return nil, false
	}
	normalizedHandoff, ok := normalizedCanvasHandoff(parsed.metadata.Handoff)
	if !ok {
		return nil, false
	}
	payload := parsed.payload
	switch normalizedHandoff.Kind {
	case CanvasHandoffForm:
		ok = hasInteractiveCanvasControl(payload.Content)
	case CanvasHandoffResults:
		ok = hasResultCanvasContent(payload.Content)
	default:
		ok = false
	}
	if !ok {
		return nil, false
	}
	payload.Handoff = normalizedHandoff
	return payload, true
}

// TerminalRenderResult is a validated self-contained renderer result. Result
// is the clean user-visible fence with protocol metadata removed. Canvas is
// populated only for the separate Canvas presentation; inline results remain
// transcript text and never carry handoff UI.
type TerminalRenderResult struct {
	Presentation TerminalRenderPresentation
	Result       string
	Canvas       *CanvasPayload
}

// ParseTerminalRenderResult validates explicit renderer-owned terminal
// presentation metadata. Inline results must be nonempty, non-error structured
// output without a handoff. Canvas results require a validated form/results
// handoff whose kind matches the destination content.
func ParseTerminalRenderResult(result string) (*TerminalRenderResult, bool) {
	if parsed, ok := parseCompleteRenderResult(
		result,
		canvasFencePrefixes,
		TerminalRenderPresentationCanvas,
	); ok && parsed.metadata != nil &&
		parsed.metadata.Presentation == TerminalRenderPresentationCanvas {
		handoff, valid := normalizedCanvasHandoff(parsed.metadata.Handoff)
		if !valid {
			return nil, false
		}
		switch handoff.Kind {
		case CanvasHandoffForm:
			valid = hasInteractiveCanvasControl(parsed.payload.Content)
		case CanvasHandoffResults:
			valid = hasResultCanvasContent(parsed.payload.Content)
		default:
			valid = false
		}
		if !valid {
			return nil, false
		}
		parsed.payload.Handoff = handoff
		return &TerminalRenderResult{
			Presentation: TerminalRenderPresentationCanvas,
			Result:       parsed.cleanResult,
			Canvas:       parsed.payload,
		}, true
	}

	if parsed, ok := parseCompleteRenderResult(
		result,
		inlineRenderFencePrefixes,
		TerminalRenderPresentationInline,
	); ok && parsed.metadata != nil &&
		parsed.metadata.Presentation == TerminalRenderPresentationInline &&
		parsed.metadata.Handoff == nil &&
		hasResultCanvasContent(parsed.payload.Content) {
		return &TerminalRenderResult{
			Presentation: TerminalRenderPresentationInline,
			Result:       parsed.cleanResult,
		}, true
	}

	return nil, false
}

type parsedRenderResult struct {
	payload     *CanvasPayload
	metadata    *TerminalRenderMetadata
	cleanResult string
}

func parseCompleteRenderResult(
	result string,
	prefixes []string,
	presentation TerminalRenderPresentation,
) (*parsedRenderResult, bool) {
	trimmed := strings.TrimSpace(result)
	for _, prefix := range prefixes {
		opening := prefix + "\n"
		if !strings.HasPrefix(trimmed, opening) || !strings.HasSuffix(trimmed, "\n```") {
			continue
		}
		raw := strings.TrimSpace(trimmed[len(opening) : len(trimmed)-len("\n```")])
		normalized, meta, err := normalizeCanvasPayload(raw)
		if err != nil {
			return nil, false
		}
		var envelope struct {
			Close          bool                    `json:"close"`
			TerminalRender *TerminalRenderMetadata `json:"_terminal_render,omitempty"`
		}
		if json.Unmarshal(normalized, &envelope) != nil || envelope.Close {
			return nil, false
		}
		if envelope.TerminalRender != nil {
			var content map[string]json.RawMessage
			if json.Unmarshal(normalized, &content) != nil {
				return nil, false
			}
			delete(content, terminalRenderMetadataKey)
			cleaned, err := json.Marshal(content)
			if err != nil {
				return nil, false
			}
			normalized, meta, err = normalizeCanvasPayload(string(cleaned))
			if err != nil {
				return nil, false
			}
		}
		payload := &CanvasPayload{
			TabID:     meta.TabID,
			Title:     meta.Title,
			Kind:      meta.Kind,
			Qualifier: meta.Qualifier,
			Content:   normalized,
		}
		if envelope.TerminalRender != nil &&
			envelope.TerminalRender.Presentation != presentation {
			return nil, false
		}
		return &parsedRenderResult{
			payload:     payload,
			metadata:    envelope.TerminalRender,
			cleanResult: opening + string(normalized) + "\n```",
		}, true
	}
	return nil, false
}

// hasResultCanvasContent rejects structurally empty and explicit error
// destinations while accepting valid informational dashboards/details. It
// intentionally does not infer form-vs-results from the top-level envelope.
func hasResultCanvasContent(normalized json.RawMessage) bool {
	var root map[string]any
	if json.Unmarshal(normalized, &root) != nil || canvasContainsError(root) {
		return false
	}
	for _, key := range []string{"panels", "sections", "items", "rows", "data"} {
		switch value := root[key].(type) {
		case []any:
			if len(value) > 0 {
				return true
			}
		case map[string]any:
			if len(value) > 0 {
				return true
			}
		}
	}
	if extension, ok := root["extension"].(map[string]any); ok {
		return len(extension) > 0
	}
	return false
}

func canvasContainsError(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if canvasContainsError(item) {
				return true
			}
		}
	case map[string]any:
		if errValue, ok := typed["error"]; ok && errValue != nil {
			if text, stringValue := errValue.(string); !stringValue || strings.TrimSpace(text) != "" {
				return true
			}
		}
		if status, ok := typed["status"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(status)) {
			case "error", "failed", "failure":
				return true
			}
		}
		if valueType, ok := typed["type"].(string); ok && strings.EqualFold(valueType, "error") {
			return true
		}
		for _, child := range typed {
			if canvasContainsError(child) {
				return true
			}
		}
	}
	return false
}

// hasInteractiveCanvasControl distinguishes a self-contained interactive view
// from a valid informational/error Canvas. Built-in forms/buttons are detected
// structurally. Host extensions opt in per result with
// `extension.interactive: true`, allowing a renderer to keep its error/empty
// states nonterminal while marking its actionable state.
func hasInteractiveCanvasControl(normalized json.RawMessage) bool {
	var root any
	if json.Unmarshal(normalized, &root) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(value any) bool {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if visit(item) {
					return true
				}
			}
		case map[string]any:
			panelType, _ := typed["type"].(string)
			if panelType == "action-form" || panelType == "action-button" {
				return true
			}
			if extension, ok := typed["extension"].(map[string]any); ok {
				if interactive, _ := extension["interactive"].(bool); interactive {
					return true
				}
			}
			if toggle, ok := typed["toggle"].(map[string]any); ok {
				if message, _ := toggle["message"].(string); message != "" {
					return true
				}
			}
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(root)
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
