package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ebarron/netapp-chat-service/llm"
)

// OpenNavToolName is the conventional name for the open-nav-view tool (C6).
// It is a generic, host-registered navigation seam: a navigation interest's
// body instructs the LLM to call open_nav_view(destination), and the host
// handles the resulting EventOpenNav by driving its own navigation machinery
// (mirroring how canvas_open is handled). The destination is an opaque,
// host-defined string — the engine never hardcodes or interprets destinations.
const OpenNavToolName = "open_nav_view"

// openNavInput is the argument schema for the open_nav_view tool.
type openNavInput struct {
	Destination string `json:"destination"`
}

// NewOpenNavTool returns a ready-to-register InternalTool implementing the
// generic open-nav-view seam (C6). The host registers it via
// server.ChatDeps.ExtraTools, exactly like a render tool:
//
//	deps.ExtraTools[agent.OpenNavToolName] = agent.NewOpenNavTool()
//
// When the LLM calls open_nav_view({"destination": "<opaque>"}), the tool:
//  1. validates that a non-empty destination was supplied,
//  2. returns a short confirmation string as the tool result (so the LLM sees
//     the navigation happened), and
//  3. emits an EventOpenNav carrying the destination, which the server relays
//     as an "open_nav" SSE event for the frontend to act on.
//
// The returned struct is a value; a host may tweak fields before registering —
// e.g. set RequiredAfterInterest to the ID of its navigation interest so the
// agent forces the call when that interest was loaded but the model forgot to
// navigate.
func NewOpenNavTool() InternalTool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"destination": map[string]any{
				"type": "string",
				"description": "The navigation destination to open, as an opaque " +
					"identifier chosen from the interest's destination catalog " +
					"(e.g. a route path or a stable screen id).",
			},
		},
		"required": []string{"destination"},
	})
	return InternalTool{
		Def: llm.ToolDef{
			Name: OpenNavToolName,
			Description: "Open a navigation destination in the host application. " +
				"Call this to bring up a specific screen/page the user asked for. " +
				"The destination must be one of the entries listed in the active " +
				"navigation interest's destination catalog.",
			Schema: schema,
		},
		Handler: openNavHandler,
		Emit:    openNavEmit,
	}
}

// openNavHandler validates the destination and returns a confirmation string.
func openNavHandler(_ context.Context, input json.RawMessage) (string, error) {
	dest, err := parseOpenNavDestination(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Opened navigation destination %q.", dest), nil
}

// openNavEmit returns the EventOpenNav side-channel event for a successful
// open_nav_view call. It is only invoked after openNavHandler succeeds, so the
// destination is guaranteed non-empty; on the defensive off-chance the input
// no longer parses, it emits nothing.
func openNavEmit(input json.RawMessage) []Event {
	dest, err := parseOpenNavDestination(input)
	if err != nil {
		return nil
	}
	return []Event{{
		Type:    EventOpenNav,
		OpenNav: &OpenNavPayload{Destination: dest},
	}}
}

func parseOpenNavDestination(input json.RawMessage) (string, error) {
	var req openNavInput
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("%s: invalid input: %w", OpenNavToolName, err)
	}
	dest := strings.TrimSpace(req.Destination)
	if dest == "" {
		return "", fmt.Errorf("%s: 'destination' is required", OpenNavToolName)
	}
	return dest, nil
}
