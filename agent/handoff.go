package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxHandoffHeadingRunes = 160
	maxHandoffActionRunes  = 80
)

// WithCanvasHandoff wraps a renderer so its successful complete Canvas result
// carries renderer-owned transcript metadata. Invalid metadata, errors, prose,
// and non-Canvas results pass through unchanged. The agent extracts and
// validates this envelope before terminal completion; default tools are inert.
func WithCanvasHandoff(
	handler InternalToolHandler,
	handoff CanvasHandoff,
) InternalToolHandler {
	normalized, valid := normalizedCanvasHandoff(&handoff)
	return func(ctx context.Context, input json.RawMessage) (string, error) {
		result, err := handler(ctx, input)
		if err != nil || !valid {
			return result, err
		}
		return embedCanvasHandoff(result, *normalized), nil
	}
}

func embedCanvasHandoff(result string, handoff CanvasHandoff) string {
	trimmed := strings.TrimSpace(result)
	for _, prefix := range canvasFencePrefixes {
		opening := prefix + "\n"
		if !strings.HasPrefix(trimmed, opening) || !strings.HasSuffix(trimmed, "\n```") {
			continue
		}
		raw := strings.TrimSpace(trimmed[len(opening) : len(trimmed)-len("\n```")])
		var payload map[string]any
		if json.Unmarshal([]byte(raw), &payload) != nil {
			return result
		}
		payload["handoff"] = handoff
		encoded, err := json.Marshal(payload)
		if err != nil {
			return result
		}
		return opening + string(encoded) + "\n```"
	}
	return result
}

// ValidateCanvasHandoff validates renderer-owned transcript copy. The
// component renders these fields as plain text; controls and line breaks are
// rejected so metadata cannot become markup or an unbounded transcript.
func ValidateCanvasHandoff(handoff CanvasHandoff) error {
	if handoff.Kind != CanvasHandoffForm && handoff.Kind != CanvasHandoffResults {
		return errors.New("handoff kind must be form or results")
	}
	if err := validateHandoffText(handoff.Heading, maxHandoffHeadingRunes); err != nil {
		return errors.New("invalid handoff heading: " + err.Error())
	}
	if err := validateHandoffText(handoff.ActionLabel, maxHandoffActionRunes); err != nil {
		return errors.New("invalid handoff action label: " + err.Error())
	}
	return nil
}

func normalizedCanvasHandoff(handoff *CanvasHandoff) (*CanvasHandoff, bool) {
	if handoff == nil {
		return nil, false
	}
	normalized := &CanvasHandoff{
		Kind:        handoff.Kind,
		Heading:     strings.TrimSpace(handoff.Heading),
		ActionLabel: strings.TrimSpace(handoff.ActionLabel),
	}
	if ValidateCanvasHandoff(*normalized) != nil {
		return nil, false
	}
	return normalized, true
}

func validateHandoffText(value string, maxRunes int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("must not be empty")
	}
	if !utf8.ValidString(value) {
		return errors.New("must be valid UTF-8")
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return errors.New("is too long")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("must be single-line plain text")
		}
	}
	return nil
}
