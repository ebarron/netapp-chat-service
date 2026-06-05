// Package agent implements the chatbot's agentic tool-use loop. It
// orchestrates the conversation between the LLM and MCP tool servers,
// streaming events back to the caller.
//
// Design ref: docs/chatbot-design-spec.md §2.2
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// DefaultMaxIterations is the safety limit for tool-call rounds per user
// message. After this many iterations the LLM is asked to summarize.
const DefaultMaxIterations = 10

// MaxToolsPerRequest is the hard cap on tools sent to the LLM per request.
// Azure OpenAI rejects requests with > 128 tools (HTTP 400), and OpenAI
// accuracy degrades sharply above ~20 tools per turn. Enforced in
// (*Agent).filteredTools.
const MaxToolsPerRequest = 128

// Tool-routing modes (S7 supervisor). These are the canonical values for the
// tool_routing.mode config key; config validation and the agent loop both
// reference them so there is a single source of truth.
const (
	// ToolRoutingOff disables the supervisor. Tool selection is byte-for-byte
	// identical to the pre-S7 behavior: every enabled capability's tools are
	// sent on every turn. This is the default.
	ToolRoutingOff = "off"
	// ToolRoutingInBand enables the in-band supervisor (S7a): the main model
	// self-selects capability groups via the load_tools internal tool before
	// the worker loop loads those groups' tools. No dedicated routing call.
	ToolRoutingInBand = "in-band"
	// ToolRoutingRouter selects the dedicated router model (S7b). Parsed and
	// validated as a legal mode but not yet implemented; startup wiring
	// rejects it until the S7b path is built.
	ToolRoutingRouter = "router"
)

// ValidToolRoutingMode reports whether mode is a recognized tool_routing mode.
// An empty string is treated as ToolRoutingOff and is considered valid.
func ValidToolRoutingMode(mode string) bool {
	switch mode {
	case "", ToolRoutingOff, ToolRoutingInBand, ToolRoutingRouter:
		return true
	}
	return false
}

// ErrTooManyTools is returned by filteredTools when the assembled tool list
// exceeds MaxToolsPerRequest. The chat handler should surface a clear
// message to the user instead of letting the LLM call fail.
var ErrTooManyTools = errors.New("too many tools enabled")

// maxRateLimitRetries is the number of times to retry after a 429 rate-limit error.
const maxRateLimitRetries = 2

// defaultRetryDelay is used when the retry delay can't be parsed from the error.
const defaultRetryDelay = 5 * time.Second

// retryDelayRe matches "Please try again in 2.304s" in OpenAI 429 responses.
var retryDelayRe = regexp.MustCompile(`try again in ([\d.]+)s`)

// parseRateLimitDelay extracts the retry delay from a rate-limit error message.
func parseRateLimitDelay(errMsg string) time.Duration {
	if m := retryDelayRe.FindStringSubmatch(errMsg); len(m) == 2 {
		if secs, err := strconv.ParseFloat(m[1], 64); err == nil {
			// Add a small buffer to avoid hitting the limit again immediately.
			return time.Duration(secs*1000+500) * time.Millisecond
		}
	}
	return defaultRetryDelay
}

// isRateLimitError returns true if the error message indicates a 429 rate-limit.
func isRateLimitError(errMsg string) bool {
	return strings.Contains(errMsg, "429") && strings.Contains(errMsg, "rate_limit")
}

// InternalToolHandler is a function that handles an internal tool call.
// It receives the raw JSON input and returns a result string or error.
type InternalToolHandler func(ctx context.Context, input json.RawMessage) (string, error)

// InternalTool bundles a tool definition with its handler so the agent
// can advertise the tool to the LLM and execute it locally.
type InternalTool struct {
	Def           llm.ToolDef
	Handler       InternalToolHandler
	ReadWriteOnly bool // if true, excluded when Mode is not "read-write"
	EmitResult    bool // if true, tool result is also emitted as EventText
	// RequiredAfterInterest makes this tool mandatory when the named interest
	// was loaded via get_interest. If the LLM finishes without calling the
	// tool, the agent injects a system message and forces a retry. This
	// prevents LLMs from skipping render tools.
	RequiredAfterInterest string
}

// Event is emitted by the agent loop to inform the caller about progress.
// The caller (typically the SSE handler) converts these to SSE events.
type Event struct {
	Type       EventType      `json:"type"`
	Text       string         `json:"text,omitempty"`        // for EventText
	ToolCall   *llm.ToolCall  `json:"tool_call,omitempty"`   // for EventToolStart
	ToolName   string         `json:"tool_name,omitempty"`   // for EventToolResult / EventToolError
	ToolResult string         `json:"tool_result,omitempty"` // for EventToolResult
	Error      string         `json:"error,omitempty"`       // for EventToolError, EventError
	Capability string         `json:"capability,omitempty"`  // MCP capability ID (Phase 2)
	ApprovalID string         `json:"approval_id,omitempty"` // for EventToolApprovalRequired (Phase 2)
	Canvas     *CanvasPayload `json:"canvas,omitempty"`      // for EventCanvasOpen
}

// CanvasPayload holds the data for a canvas_open SSE event.
type CanvasPayload struct {
	TabID     string          `json:"tab_id"`
	Title     string          `json:"title"`
	Kind      string          `json:"kind"`
	Qualifier string          `json:"qualifier"`
	Content   json.RawMessage `json:"content"`
}

// EventType enumerates agent-level event kinds.
type EventType int

const (
	// EventText is a streamed text token from the LLM.
	EventText EventType = iota
	// EventToolStart signals that a tool call is about to execute.
	EventToolStart
	// EventToolResult carries the tool execution result.
	EventToolResult
	// EventToolError signals a tool execution failure (non-fatal, fed back to LLM).
	EventToolError
	// EventDone signals the end of the agent loop.
	EventDone
	// EventError signals a fatal error (loop stops).
	EventError
	// EventToolApprovalRequired signals that a tool call needs user approval (Ask mode).
	EventToolApprovalRequired
	// EventTextClear tells the UI to clear any accumulated assistant text.
	// Emitted when a streaming turn produced "thinking" text alongside tool
	// calls (common with Claude models). The text was shown during streaming
	// for feedback but should not persist into the final message.
	EventTextClear
	// EventCanvasOpen tells the UI to open content in a canvas tab.
	// Emitted when the LLM uses a canvas-object-detail or canvas-dashboard
	// code fence, signaling the content should be pinned rather than inline.
	EventCanvasOpen
)

// Agent runs the tool-use loop. It holds the LLM provider and MCP router.
type Agent struct {
	Provider      llm.Provider
	Router        mcpclient.ToolRouter
	SystemPrompt  string
	Model         string
	MaxIterations int
	Logger        *slog.Logger
	// Phase 2: capability filtering
	CapStates capability.CapabilityMap // nil = no filtering
	Mode      string                   // "read-only" or "read-write"
	// ToolServerMap maps tool name -> capability ID for ask-mode routing.
	// Populated at agent creation from the router.
	ToolServerMap map[string]string
	// ApprovalFunc is called when a tool requires user approval (Ask mode).
	// It returns true if approved. If nil, ask-mode tools are auto-approved.
	ApprovalFunc func(capID, toolName string, tc llm.ToolCall) bool
	// InternalTools are handled locally by the agent, not routed through MCP.
	// Keyed by tool name.
	InternalTools map[string]InternalTool

	// --- In-band tool routing (S7a). Active only when ToolRoutingMode is
	// ToolRoutingInBand; otherwise these are inert and behavior is identical
	// to today. ---

	// ToolRoutingMode is one of ToolRoutingOff (default), ToolRoutingInBand,
	// or ToolRoutingRouter. Set via WithToolRouting.
	ToolRoutingMode string
	// AlwaysOnGroups are capability/group IDs loaded from the first turn
	// without the model calling load_tools.
	AlwaysOnGroups []string
	// MaxRoutedTools optionally caps the post-routing tool list (0 = no cap
	// beyond MaxToolsPerRequest).
	MaxRoutedTools int
	// ForceGroupLoad, when true, makes the agent inject one corrective nudge
	// if the model tries to answer (tool-lessly) before loading any group
	// while groups are available. Defaults on for in-band via WithToolRouting.
	ForceGroupLoad bool

	// groups is the auto-derived group menu offered this turn (capability
	// registry filtered by enabled). Used to validate load_tools IDs and for
	// telemetry. Immutable after construction.
	groups []capability.Group
	// routingMu guards loadedGroups and stats (load_tools handlers run in
	// parallel with other tool calls).
	routingMu sync.Mutex
	// loadedGroups is the set of group IDs the model has loaded this turn.
	loadedGroups map[string]bool
	// loadedTools is the set of individual tool names the model has loaded this
	// turn via load_tools(tools:[…]) (S8 intra-group selection). A tool here is
	// activated without activating its whole group.
	loadedTools map[string]bool
	// stats accumulates per-run routing telemetry (Layer 5).
	stats RoutingStats
}

// RoutingStats captures per-run in-band routing telemetry. It is the empirical
// basis for the S7a→S7b graduation decision (skip / misroute rates). Read it
// via (*Agent).LastRoutingStats after a Run.
type RoutingStats struct {
	Mode          string   // routing mode for the run
	GroupsOffered int      // number of groups in the menu this turn
	GroupsLoaded  []string // final set of loaded group IDs (sorted; includes groups owning any individually-loaded tool)
	ToolsLoaded   []string // individually-loaded tool names (S8 intra-group selection; sorted)
	LoadCalls     int      // number of load_tools invocations
	Reloads       int      // load_tools invocations after the first (mid-task re-loads)
	Skipped       bool     // a final answer was produced without ever loading a group while groups were available
	Compliant     bool     // at least one group was loaded before finishing
}

// New creates an Agent with the given dependencies.
func New(provider llm.Provider, router mcpclient.ToolRouter, opts ...Option) *Agent {
	a := &Agent{
		Provider:      provider,
		Router:        router,
		MaxIterations: DefaultMaxIterations,
		Logger:        slog.Default(),
	}
	for _, o := range opts {
		o(a)
	}
	// When in-band routing is enabled, register the internal load_tools tool
	// here (after all options are applied) so it can't be clobbered by the
	// order of WithInternalTools vs WithToolRouting, and initialize per-run
	// routing state.
	if a.ToolRoutingMode == ToolRoutingInBand {
		if a.loadedGroups == nil {
			a.loadedGroups = make(map[string]bool)
		}
		if a.loadedTools == nil {
			a.loadedTools = make(map[string]bool)
		}
		if a.InternalTools == nil {
			a.InternalTools = make(map[string]InternalTool)
		}
		a.InternalTools["load_tools"] = InternalTool{
			Def:     loadToolsDef(),
			Handler: a.handleLoadTools,
		}
	}
	return a
}

// Option configures an Agent.
type Option func(*Agent)

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) { a.SystemPrompt = prompt }
}

// WithModel overrides the model name for requests.
func WithModel(model string) Option {
	return func(a *Agent) { a.Model = model }
}

// WithMaxIterations sets the tool-call iteration limit.
func WithMaxIterations(n int) Option {
	return func(a *Agent) { a.MaxIterations = n }
}

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(a *Agent) { a.Logger = l }
}

// WithCapabilityFilter sets capability states and mode for tool filtering.
// Tools from capabilities in StateOff are excluded. In read-only mode,
// tools annotated as write/destructive are also excluded.
func WithCapabilityFilter(states capability.CapabilityMap, mode string) Option {
	return func(a *Agent) {
		a.CapStates = states
		a.Mode = mode
	}
}

// WithToolServerMap sets the tool-to-capability mapping for ask-mode routing.
func WithToolServerMap(m map[string]string) Option {
	return func(a *Agent) { a.ToolServerMap = m }
}

// WithApprovalFunc sets the callback for ask-mode tool approval.
func WithApprovalFunc(fn func(capID, toolName string, tc llm.ToolCall) bool) Option {
	return func(a *Agent) { a.ApprovalFunc = fn }
}

// WithInternalTools registers tools that the agent handles locally
// instead of routing through MCP.
func WithInternalTools(tools map[string]InternalTool) Option {
	return func(a *Agent) { a.InternalTools = tools }
}

// WithToolRouting configures the in-band supervisor (S7a). mode selects the
// routing strategy (ToolRoutingOff disables it). groups is the auto-derived
// group menu for this turn (from capability.BuildGroups). alwaysOn lists
// groups loaded from turn 1. maxTools optionally caps the post-routing list
// (0 = no extra cap). forceGroupLoad enables the one-shot corrective nudge
// when the model answers before loading a group.
//
// When mode is not ToolRoutingInBand this is inert; the agent behaves exactly
// as it does today.
func WithToolRouting(mode string, groups []capability.Group, alwaysOn []string, maxTools int, forceGroupLoad bool) Option {
	return func(a *Agent) {
		a.ToolRoutingMode = mode
		a.groups = groups
		a.AlwaysOnGroups = alwaysOn
		a.MaxRoutedTools = maxTools
		a.ForceGroupLoad = forceGroupLoad
	}
}

// Run executes the agentic tool-use loop for a user message. It calls the
// provided emit function for each event. The conversation history is carried
// in messages; the caller manages session state.
//
// The loop:
//  1. Gathers tools from the Router
//  2. Sends messages + tools to the LLM via streaming
//  3. On text tokens -> emit EventText
//  4. On tool calls -> execute via Router, emit EventToolStart/Result/Error,
//     append results to messages and re-send to LLM
//  5. Repeat until the LLM produces a text-only response or max iterations
//  6. Emit EventDone
func (a *Agent) Run(ctx context.Context, messages []llm.Message, emit func(Event)) {
	// Wrap emit with a canvas fence interceptor so that canvas-object-detail
	// and canvas-dashboard code fences are converted to EventCanvasOpen events.
	originalEmit := emit
	interceptor := newCanvasFenceInterceptor(originalEmit)
	emit = func(evt Event) {
		if evt.Type == EventText {
			interceptor.HandleToken(evt.Text)
			return
		}
		// Flush any buffered text before emitting non-text events.
		interceptor.Flush()
		originalEmit(evt)
	}
	// Defer a final flush in case the stream ends mid-buffer.
	defer interceptor.Flush()

	// Reset per-run in-band routing state (the agent is normally built fresh
	// per message, but resetting keeps reuse deterministic) and arrange to
	// emit routing telemetry when the run ends.
	if a.ToolRoutingMode == ToolRoutingInBand {
		a.routingMu.Lock()
		a.loadedGroups = make(map[string]bool)
		a.loadedTools = make(map[string]bool)
		a.stats = RoutingStats{Mode: a.ToolRoutingMode, GroupsOffered: len(a.groups)}
		a.routingMu.Unlock()
		defer a.logRoutingStats()
	}

	tools, err := a.filteredTools()
	if err != nil {
		a.Logger.Error("filteredTools failed", "error", err)
		emit(Event{Type: EventError, Error: err.Error()})
		emit(Event{Type: EventDone})
		return
	}

	maxIter := a.MaxIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxIterations
	}

	runStart := time.Now()

	// Track interest-to-tool enforcement. When an InternalTool has
	// RequiredAfterInterest set, the agent ensures the tool is called
	// after the corresponding interest is loaded via get_interest.
	loadedInterests := map[string]bool{}
	calledTools := map[string]bool{}

	// groupLoadNudged tracks whether we've already issued the one-shot
	// forced-first-step correction for in-band routing this run.
	groupLoadNudged := false

	// Pre-scan message history for interests loaded in previous turns
	// so RequiredAfterInterest enforcement works on follow-up messages
	// (where the LLM won't re-call get_interest because it's already
	// in context).
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			if tc.Name == "get_interest" {
				var args struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(tc.Input, &args) == nil && args.ID != "" {
					loadedInterests[args.ID] = true
				}
			}
		}
	}

	for iteration := 0; iteration < maxIter; iteration++ {
		iterStart := time.Now()
		req := llm.ChatRequest{
			Messages: messages,
			Tools:    tools,
			System:   a.SystemPrompt,
			Model:    a.Model,
		}

		var pendingToolCalls []llm.ToolCall
		var hadError bool

		var streamErr error
		for retry := 0; retry <= maxRateLimitRetries; retry++ {
			streamErr = nil
			llmStart := time.Now()
			firstToken := true
			for ev, err := range a.Provider.ChatStream(ctx, req) {
				if err != nil {
					streamErr = err
					break
				}

				if firstToken {
					a.Logger.Info("llm first token",
						"iteration", iteration+1,
						"ttft", time.Since(llmStart).Round(time.Millisecond),
					)
					firstToken = false
				}

				switch ev.Type {
				case llm.EventText:
					emit(Event{Type: EventText, Text: ev.Delta})

				case llm.EventToolCall:
					if ev.ToolCall != nil {
						a.Logger.Info("llm requested tool",
							"iteration", iteration+1,
							"tool", ev.ToolCall.Name, "args", ev.ToolCall.Input, "elapsed", time.Since(llmStart).Round(time.Millisecond),
						)
						pendingToolCalls = append(pendingToolCalls, *ev.ToolCall)
					}

				case llm.EventDone:
					// stream finished for this turn
				}
			}

			a.Logger.Info("llm stream complete",
				"iteration", iteration+1,
				"duration", time.Since(llmStart).Round(time.Millisecond),
				"tool_calls", len(pendingToolCalls),
			)

			if streamErr == nil {
				break
			}

			// If it's a rate-limit error and we have retries left, wait and try again.
			if isRateLimitError(streamErr.Error()) && retry < maxRateLimitRetries {
				delay := parseRateLimitDelay(streamErr.Error())
				a.Logger.Warn("rate limited, retrying", "delay", delay, "retry", retry+1, "iteration", iteration)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					emit(Event{Type: EventError, Error: "Request cancelled"})
					return
				}
			}

			// Non-retryable error or retries exhausted.
			a.Logger.Error("llm stream error", "error", streamErr, "iteration", iteration)
			emit(Event{Type: EventError, Error: streamErr.Error()})
			return
		}

		// If no tool calls, the LLM produced a final text response.
		if len(pendingToolCalls) == 0 {
			// In-band routing forced-first-step (optional): if the model is
			// answering without having activated any group while groups are
			// available, nudge it once to load the relevant group(s) before
			// allowing a tool-less answer. After one nudge we let the answer
			// stand (graceful fallback) — a genuine no-tool answer is valid.
			if a.shouldForceGroupLoad() && !groupLoadNudged {
				groupLoadNudged = true
				a.Logger.Warn("in-band routing: answer attempted before loading a tool group, nudging",
					"iteration", iteration+1)
				emit(Event{Type: EventTextClear})
				messages = append(messages, llm.Message{
					Role:    llm.RoleSystem,
					Content: "You attempted to answer without loading any tool group. If the user's request needs tools, call load_tools with the relevant Group ID(s) now, then answer. If it genuinely needs no tools, you may answer directly.",
				})
				continue
			}
			// Check if any required tools were skipped.
			if missing := a.missingRequiredTool(loadedInterests, calledTools); missing != "" {
				a.Logger.Warn("LLM skipped required tool, forcing retry",
					"tool", missing, "iteration", iteration+1)
				// Clear the text the LLM just streamed — it's not the
				// expected visual output.
				emit(Event{Type: EventTextClear})
				messages = append(messages, llm.Message{
					Role:    llm.RoleSystem,
					Content: fmt.Sprintf("You MUST call the %s tool now. The frontend cannot render this view without it. Do not produce text — call the tool.", missing),
				})
				continue
			}
			a.Logger.Info("agent done (text response)",
				"total_duration", time.Since(runStart).Round(time.Millisecond),
				"iterations", iteration+1,
			)
			emit(Event{Type: EventDone})
			return
		}

		// Tool calls detected — clear any "thinking" text that was
		// streamed during this turn. Claude (via OpenAI-compatible
		// proxies) emits text alongside tool calls; OpenAI models
		// don't, so this event is a no-op for them.
		emit(Event{Type: EventTextClear})

		// Build the assistant message with tool calls for the history.
		assistantMsg := llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: pendingToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute tool calls in parallel. The LLM batches tool calls
		// within a single response only when they are independent, so
		// parallel execution is safe and significantly faster for
		// multi-tool rounds like dashboard builds.
		type toolResult struct {
			index   int
			message llm.Message
			events  []Event
			isError bool
		}

		results := make([]toolResult, len(pendingToolCalls))
		var wg sync.WaitGroup
		toolsStart := time.Now()

		for i, tc := range pendingToolCalls {
			wg.Add(1)
			go func(idx int, tc llm.ToolCall) {
				defer wg.Done()
				tr := toolResult{index: idx}
				toolStart := time.Now()

				// Check if this is an internal tool (handled locally, not via MCP).
				if it, ok := a.InternalTools[tc.Name]; ok {
					tr.events = append(tr.events, Event{
						Type:     EventToolStart,
						ToolCall: &tc,
						ToolName: tc.Name,
					})

					result, err := it.Handler(ctx, tc.Input)
					if err != nil {
						a.Logger.Warn("internal tool call failed",
							"tool", tc.Name, "error", err)
						tr.events = append(tr.events, Event{
							Type:     EventToolError,
							ToolName: tc.Name,
							Error:    err.Error(),
						})
						tr.message = llm.Message{
							Role:       llm.RoleTool,
							Content:    fmt.Sprintf("Error executing tool %s: %s", tc.Name, err.Error()),
							ToolCallID: tc.ID,
						}
						tr.isError = true
					} else {
						if it.EmitResult {
							tr.events = append(tr.events, Event{
								Type: EventText,
								Text: result + "\n\n",
							})
						}
						tr.events = append(tr.events, Event{
							Type:       EventToolResult,
							ToolName:   tc.Name,
							ToolResult: result,
						})
						tr.message = llm.Message{
							Role:       llm.RoleTool,
							Content:    result,
							ToolCallID: tc.ID,
						}
					}
					results[idx] = tr
					a.Logger.Info("tool completed",
						"tool", tc.Name, "type", "internal",
						"duration", time.Since(toolStart).Round(time.Millisecond),
					)
					return
				}

				// Determine the capability for this tool.
				capID := ""
				if a.ToolServerMap != nil {
					capID = a.ToolServerMap[tc.Name]
				}

				// Check ask mode: if the capability is in Ask state, request approval.
				// In StateAskOnWrite, only require approval for write tools (a tool is
				// considered read-only when its ReadOnlyHint annotation is true; tools
				// without the hint are treated as writes).
				if a.CapStates != nil && capID != "" {
					state, ok := a.CapStates[capID]
					needsApproval := ok && (state == capability.StateAsk ||
						(state == capability.StateAskOnWrite && !a.isReadOnlyTool(tc.Name)))
					if needsApproval {
						if a.ApprovalFunc != nil {
							approved := a.ApprovalFunc(capID, tc.Name, tc)
							if !approved {
								tr.events = append(tr.events, Event{
									Type:       EventToolError,
									ToolName:   tc.Name,
									Capability: capID,
									Error:      "Tool call denied by user",
								})
								tr.message = llm.Message{
									Role:       llm.RoleTool,
									Content:    "User denied this tool call.",
									ToolCallID: tc.ID,
								}
								results[idx] = tr
								return
							}
						}
					}
				}

				tr.events = append(tr.events, Event{
					Type:       EventToolStart,
					ToolCall:   &tc,
					ToolName:   tc.Name,
					Capability: capID,
				})

				result, err := a.Router.CallTool(ctx, tc)
				if err != nil {
					a.Logger.Warn("tool call failed",
						"tool", tc.Name,
						"error", err,
						"iteration", iteration,
					)
					tr.events = append(tr.events, Event{
						Type:     EventToolError,
						ToolName: tc.Name,
						Error:    err.Error(),
					})
					tr.message = llm.Message{
						Role:       llm.RoleTool,
						Content:    fmt.Sprintf("Error executing tool %s: %s", tc.Name, err.Error()),
						ToolCallID: tc.ID,
					}
					tr.isError = true
				} else {
					tr.events = append(tr.events, Event{
						Type:       EventToolResult,
						ToolName:   tc.Name,
						ToolResult: result,
					})
					tr.message = llm.Message{
						Role:       llm.RoleTool,
						Content:    result,
						ToolCallID: tc.ID,
					}
				}
				results[idx] = tr
				a.Logger.Info("tool completed",
					"tool", tc.Name, "type", "mcp",
					"duration", time.Since(toolStart).Round(time.Millisecond),
					"error", tr.isError,
				)
			}(i, tc)
		}

		wg.Wait()
		toolsDuration := time.Since(toolsStart)

		// Emit events and collect messages in original order.
		for _, tr := range results {
			for _, ev := range tr.events {
				emit(ev)
			}
			messages = append(messages, tr.message)
			if tr.isError {
				hadError = true
			}
		}

		// Track which tools were called and which interests were loaded
		// for the required-tool enforcement check.
		for _, tc := range pendingToolCalls {
			calledTools[tc.Name] = true
			if tc.Name == "get_interest" {
				var args struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(tc.Input, &args) == nil && args.ID != "" {
					loadedInterests[args.ID] = true
				}
			}
		}

		_ = hadError // used for future capability filtering

		// In-band routing: a load_tools call this round may have changed the
		// active group set, so recompute the tool list for the next
		// iteration. For mode:off this is never reached and `tools` stays
		// exactly as computed once before the loop (byte-identical behavior).
		if a.ToolRoutingMode == ToolRoutingInBand {
			newTools, ferr := a.filteredTools()
			if ferr != nil {
				a.Logger.Error("filteredTools failed after routing update", "error", ferr)
				emit(Event{Type: EventError, Error: ferr.Error()})
				emit(Event{Type: EventDone})
				return
			}
			tools = newTools
		}

		a.Logger.Info("tool round complete",
			"iteration", iteration+1,
			"tools_called", len(pendingToolCalls),
			"tools_duration", toolsDuration.Round(time.Millisecond),
			"iteration_duration", time.Since(iterStart).Round(time.Millisecond),
		)
	}

	// Max iterations reached — ask LLM to summarize.
	a.Logger.Warn("max iterations reached", "max", maxIter)
	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: "Tool call limit reached. Please summarize what you have so far and provide your best answer to the user.",
	})

	req := llm.ChatRequest{
		Messages: messages,
		Tools:    nil, // no tools — force text response
		System:   a.SystemPrompt,
		Model:    a.Model,
	}

	for retry := 0; retry <= maxRateLimitRetries; retry++ {
		var summaryErr error
		for ev, err := range a.Provider.ChatStream(ctx, req) {
			if err != nil {
				summaryErr = err
				break
			}
			if ev.Type == llm.EventText {
				emit(Event{Type: EventText, Text: ev.Delta})
			}
		}

		if summaryErr == nil {
			break
		}

		if isRateLimitError(summaryErr.Error()) && retry < maxRateLimitRetries {
			delay := parseRateLimitDelay(summaryErr.Error())
			a.Logger.Warn("rate limited during summary, retrying", "delay", delay, "retry", retry+1)
			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				emit(Event{Type: EventError, Error: "Request cancelled"})
				return
			}
		}

		emit(Event{Type: EventError, Error: summaryErr.Error()})
		return
	}
	emit(Event{Type: EventDone})
}

// missingRequiredTool returns the name of an internal tool that should have
// been called (based on RequiredAfterInterest) but wasn't, or "" if all
// requirements are satisfied. Only one tool is returned per check so the
// agent can retry one at a time.
func (a *Agent) missingRequiredTool(loadedInterests, calledTools map[string]bool) string {
	for name, it := range a.InternalTools {
		if it.RequiredAfterInterest == "" {
			continue
		}
		if loadedInterests[it.RequiredAfterInterest] && !calledTools[name] {
			return name
		}
	}
	return ""
}

// CanvasTabSummary is a compact description of an open canvas tab,
// sent from the frontend to give the LLM context about what the user
// can currently see pinned in the canvas.
type CanvasTabSummary struct {
	TabID         string            `json:"tab_id"`
	Kind          string            `json:"kind"`
	Name          string            `json:"name"`
	Qualifier     string            `json:"qualifier"`
	Status        string            `json:"status,omitempty"`
	KeyProperties map[string]string `json:"key_properties,omitempty"`
}

// SystemPromptConfig configures the identity and domain context injected into
// the system prompt. Products supply their own config so the agent package
// remains product-agnostic.
type SystemPromptConfig struct {
	// ProductName is the assistant's display name (e.g. "NAbox Assistant").
	ProductName string
	// ProductDescription is the paragraph after the name describing the
	// product context (monitoring stack, data sources, etc.).
	ProductDescription string
	// Guidelines are appended after the role section. Include any product-
	// specific guidelines such as URL rewriting rules.
	Guidelines []string
	// Vocabulary is a free-form markdown block appended after the generic
	// Guidelines and before the connected-data-sources list. Products use
	// this to inject domain-specific guidance (entity kinds, link patterns,
	// CLI proposal formats, etc.) without modifying the agent package. The
	// chat service ships no vocabulary by default — empty string means no
	// block is appended.
	Vocabulary string
}

// BuildSystemPrompt constructs the system prompt from the current state.
// This is a convenience to generate the prompt that includes tool context.
// If interestIndex is non-empty, the chart format spec and interest catalog
// are appended so the LLM knows how to produce dashboard panels.
// If canvasTabs is non-empty, a canvas context section is appended so the
// LLM knows what items the user has pinned in the canvas.
//
// This is the no-tool-routing form, byte-for-byte identical to today. For the
// in-band supervisor (S7a), use BuildSystemPromptWithRouting.
func BuildSystemPrompt(cfg SystemPromptConfig, router mcpclient.ToolRouter, interestIndex string, canvasTabs ...CanvasTabSummary) string {
	return BuildSystemPromptWithRouting(cfg, router, interestIndex, "", canvasTabs...)
}

// BuildSystemPromptWithRouting is BuildSystemPrompt plus an optional in-band
// tool-routing group index (S7a). When groupIndex is empty the output is
// byte-for-byte identical to BuildSystemPrompt — this is the mode:off
// guarantee. When groupIndex is non-empty, a "Tool Groups" section is appended
// instructing the model to call load_tools(group) before answering, reusing
// the forced-first-step contract proven by the interest path.
func BuildSystemPromptWithRouting(cfg SystemPromptConfig, router mcpclient.ToolRouter, interestIndex, groupIndex string, canvasTabs ...CanvasTabSummary) string {
	servers := router.ConnectedServers()
	tools := router.Tools()

	prompt := fmt.Sprintf("You are the %s", cfg.ProductName)
	if cfg.ProductDescription != "" {
		prompt += ", " + cfg.ProductDescription
	}
	prompt += "\n\n"

	// Authoritative wall-clock time. The LLM's training-data cutoff makes it
	// unreliable at guessing "now" — supplying both RFC3339 and Unix epoch
	// seconds prevents stale timestamps in metrics_range_query calls (which
	// silently return empty result sets when `end` is in the past).
	now := time.Now().UTC()
	prompt += fmt.Sprintf("Current time: %s (Unix epoch seconds: %d). Use this as \"now\" for any time-windowed query (e.g. metrics_range_query `end` parameter). Do NOT use a date from your training data.\n\n",
		now.Format(time.RFC3339), now.Unix())

	prompt += `Your role:
- Help administrators understand their storage infrastructure health
- Run diagnostic queries using the tools available to you
- Explain metrics, alerts, and capacity trends
- Provide actionable recommendations

Guidelines:
- Use markdown formatting: tables for tabular data, code blocks for CLI commands
- Be concise but thorough
- When querying metrics, explain what you're looking for and interpret the results
- If a tool call fails, explain the error and suggest alternatives
- Always confirm destructive operations before proceeding
`
	for _, g := range cfg.Guidelines {
		prompt += "- " + g + "\n"
	}
	prompt += "\n"

	if cfg.Vocabulary != "" {
		prompt += cfg.Vocabulary
		if !strings.HasSuffix(cfg.Vocabulary, "\n") {
			prompt += "\n"
		}
		prompt += "\n"
	}

	if len(servers) > 0 {
		prompt += "\nConnected data sources: "
		for i, s := range servers {
			if i > 0 {
				prompt += ", "
			}
			prompt += s
		}
		prompt += "\n"
	}

	if len(tools) > 0 {
		prompt += fmt.Sprintf("\nYou have access to %d tools from the connected data sources. Use them to answer questions about the storage infrastructure.\n", len(tools))
	} else {
		prompt += "\nNo tools are currently available. You can still answer general questions about NetApp storage.\n"
	}

	// Append the in-band tool-routing group index (S7a) when present. This is
	// the forced-first-step contract: the model must load the relevant
	// group(s) before answering. Phrasing mirrors the proven interest nag.
	if groupIndex != "" {
		prompt += "\n## Tool Groups\n\n"
		prompt += "The available tools are organized into groups. Their schemas are NOT all loaded up front.\n"
		prompt += "**CRITICAL**: Before answering a request that needs tools, look at the table below and decide\n"
		prompt += "which group(s) are relevant, then call `load_tools` with those group IDs as your very first\n"
		prompt += "tool call to make their tools available. Do NOT skip this step — you cannot call a group's\n"
		prompt += "tools until you have loaded that group. You may call `load_tools` again at any time if you\n"
		prompt += "discover mid-task that you need another group; calls are additive.\n\n"
		prompt += "If the user's request does not require any tools (e.g. a greeting or a general question you\n"
		prompt += "can answer directly), you do not need to call `load_tools`.\n\n"
		prompt += groupIndex
		prompt += "\nCall `load_tools` with one or more of the Group IDs above, e.g. `load_tools({\"groups\": [\"<id>\"]})`.\n"
		prompt += "For any group shown as a **Large server** with its tools listed individually, load only the\n"
		prompt += "specific tools you need instead of the whole group, e.g. `load_tools({\"tools\": [\"<tool_name>\"]})`.\n"
	}

	// Append chart format spec and interest index when interests are available.
	if interestIndex != "" {
		prompt += chartFormatSpec
		prompt += "\n## Response Interests\n\n"
		prompt += "You have a catalog of predefined response layouts for common topics.\n"
		prompt += "**CRITICAL**: Before answering any user message, check if it **semantically matches**\n"
		prompt += "any trigger phrase in the table below. The match does not need to be exact — if the\n"
		prompt += "user's intent is clearly related to a trigger (e.g. \"provision me a 200GB volume\"\n"
		prompt += "matches \"provision a volume\"), you MUST call get_interest(id) as your very first\n"
		prompt += "tool call to retrieve the full response instructions. Do NOT skip this step — do not\n"
		prompt += "ask clarifying questions or call other tools before loading the interest.\n\n"
		prompt += "**IMPORTANT**: The interest body contains **executable instructions**, not a template.\n"
		prompt += "When it says to call a tool (e.g. metrics_query, metrics_range_query, get_active_alerts),\n"
		prompt += "you MUST actually call those tools and use the real data in your dashboard. Do NOT produce\n"
		prompt += "a dashboard with empty or placeholder data. The sequence is always:\n"
		prompt += "1. Call get_interest to load the instructions\n"
		prompt += "2. Call **EVERY** data-gathering tool described in the interest body. Interests typically\n"
		prompt += "   require 5-10 separate tool calls (multiple metrics_query and metrics_range_query calls\n"
		prompt += "   with different PromQL queries). Execute ALL of them — do NOT stop after one or two\n"
		prompt += "   queries and do NOT fabricate or omit data you did not receive from a tool call.\n"
		prompt += "   Read the interest body carefully: each numbered section that mentions a tool call\n"
		prompt += "   is a separate query you must execute.\n"
		prompt += "3. Only after receiving ALL tool results, produce the dashboard with that real data\n"
		prompt += "4. If the interest tells you to call a **render tool** (a product-supplied tool whose name\n"
		prompt += "   typically starts with `render_`), you MUST call it — that is the ONLY way to produce\n"
		prompt += "   the view. The frontend cannot reconstruct the rendered output from your text. NEVER\n"
		prompt += "   skip a render tool call, even if you already have the data from a previous turn.\n"
		prompt += "   Always call the render tool.\n"
		prompt += "5. Check the **Target** column. If it says `canvas`, emit the final output block\n"
		prompt += "   using `canvas-object-detail` or `canvas-dashboard` fences (see Canvas fences above).\n"
		prompt += "   If target is `chat` (or omitted), use the regular fence.\n\n"
		prompt += interestIndex
		prompt += "\n**Scope check**: After loading an interest with get_interest, read the SCOPE EXCLUSIONS\n"
		prompt += "section (if present) before proceeding. If the user's message matches an exclusion,\n"
		prompt += "stop following the interest and answer normally in chat instead.\n\n"
		prompt += "If the user's question does not clearly relate to any trigger phrase above, answer\n"
		prompt += "normally without calling get_interest.\n"
		prompt += interestManagementSpec
	}

	// Append canvas context when the user has pinned tabs.
	if len(canvasTabs) > 0 {
		prompt += "\n## Canvas Context\n\n"
		prompt += "The user has the following items pinned in the canvas (visible alongside this chat):\n\n"
		prompt += "| Tab | Kind | Name | Status | Context |\n"
		prompt += "|-----|------|------|--------|---------|\n"
		for i, tab := range canvasTabs {
			status := tab.Status
			if status == "" {
				status = "-"
			}
			qualifier := tab.Qualifier
			if qualifier == "" {
				qualifier = "-"
			}
			prompt += fmt.Sprintf("| %d | %s | %s | %s | %s |\n", i+1, tab.Kind, tab.Name, status, qualifier)
		}
		prompt += "\nThe user can see these items without scrolling. You can refer to them "
		prompt += "(\"the volume in your canvas\", \"as shown in the cluster detail\") without "
		prompt += "repeating their full content. When the user asks follow-up questions, "
		prompt += "consider whether they're referring to a canvas item.\n\n"
		prompt += "When the user closes a canvas tab, it will no longer appear here. "
		prompt += "Do not reference closed tabs.\n"
	}

	return prompt
}

// chartFormatSpec is a condensed version of the visualization data contract
// (spec Section 5) injected into the system prompt. It gives the LLM the
// vocabulary of chart/panel types and their JSON schemas.
const chartFormatSpec = `
## Chart & Dashboard Format

You can produce visual panels by emitting fenced code blocks. Two formats:

### Single chart — use language "chart"
` + "```" + `chart
{ "type": "<type>", ...fields per type below... }
` + "```" + `

### Multi-panel dashboard — use language "dashboard"
` + "```" + `dashboard
{
  "title": "Dashboard Title",
  "panels": [ { "type": "<type>", "width": "full|half|third", ...fields... }, ... ]
}
` + "```" + `

Panel width defaults to "full". Use "half" for side-by-side pairs, "third" for stat blocks.

### Chart types

**area** — Time-series trend
{"type":"area","title":"string","xKey":"string","yLabel":"string (opt)","series":[{"key":"string","label":"string","color":"string (opt)"}],"data":[{"<xKey>":number,"<seriesKey>":number}]}
For time-series data, the xKey value MUST be the raw unix timestamp (number, in seconds) from the metric query result. Do NOT format timestamps yourself — the UI handles formatting automatically. Example: {"time":1741392000,"iops":5200}.

**bar** — Comparison
{"type":"bar","title":"string","xKey":"string","series":[{"key":"string","label":"string","color":"string (opt)"}],"data":[...]}

**gauge** — Single utilization value
{"type":"gauge","title":"string","value":number,"max":number,"unit":"string","thresholds":{"warning":number,"critical":number}}

**sparkline** — Compact inline trend
{"type":"sparkline","title":"string (opt)","data":[number,...],"color":"string (opt)"}

**status-grid** — Multi-resource health
{"type":"status-grid","title":"string","items":[{"name":"string","status":"ok|warning|critical","detail":"string (opt)"}]}

**stat** — Single prominent value
{"type":"stat","title":"string","value":"string","subtitle":"string (opt)","trend":"up|down|flat (opt)","trendValue":"string (opt)"}

**alert-list** — Active alerts with details (works standalone or in dashboards)
{"type":"alert-list","items":[{"severity":"critical|warning|info","message":"string","time":"string"}]}

**callout** — Highlighted recommendation (works standalone or in dashboards)
{"type":"callout","icon":"string (opt)","title":"string","body":"string"}

**proposal** — Proposed command to execute (works standalone or in dashboards)
{"type":"proposal","title":"string","command":"string","format":"string (product-specific, e.g. shell, sql)"}

### Dashboard-only panel types

**alert-summary** — Severity count badges (clickable). Do not include "ok" — only real alert severities.
{"type":"alert-summary","data":{"critical":number,"warning":number,"info":number}}

**resource-table** — Clickable resource list
{"type":"resource-table","title":"string","columns":["Col1","Col2",...],"rows":[{"name":"string (always required — used for click target)","Col1":"value","Col2":"value",...}]}
Row objects MUST include a key for every entry in "columns" whose name matches the column exactly. The "name" field is always required (used for the click action) and should also appear under the first column key.
Rows may also carry hidden identity fields (keys not listed in "columns") that the click action needs to disambiguate the resource. Product-specific guidance below may list those keys.

**action-button** — Clickable action triggers
{"type":"action-button","buttons":[{"label":"string","action":"execute|message","tool":"string (for execute)","params":{} (for execute),"message":"string (for message)","icon":"string (opt)","variant":"primary|outline"}]}

### Object detail — use language "object-detail"

For questions about a single entity, produce a rich detail view instead of a dashboard:

` + "```" + `object-detail
{
  "type": "object-detail",
  "kind": "string (product-defined entity kind, e.g. 'volume', 'cluster', 'alert')",
  "name": "Display name or title",
  "status": "critical | warning | ok | info",
  "subtitle": "Brief context line",
  "qualifier": "identity context appended to action messages (see below)",
  "sections": [
    { "title": "Section Title", "layout": "properties|chart|alert-list|timeline|actions|text|table", "data": { ... } }
  ]
}
` + "```" + `

The **qualifier** field carries the identity keys needed to uniquely look up this object in follow-up requests. The UI automatically appends it to every action message from this detail view. Always set qualifier so action buttons and property links work without losing context. Product-specific guidance below describes which keys to include for each entity kind.

**Per-item qualifier override:** Property items and action buttons support an optional per-item "qualifier" field that overrides the card-level qualifier for that specific link. This is essential when a link targets a *different kind* of object whose identity keys differ from the current object.
- Set "qualifier": "" (empty string) to suppress the qualifier entirely when the target's name alone is unique.
- Set "qualifier": "<override>" to supply a different identity context when the target needs less or different context than the current object.
- Omit the per-item qualifier to inherit the card-level qualifier — use this for same-kind follow-ups.
Example action button with no override (inherits card qualifier): {"label":"Open Item","action":"message","message":"Show detail for item1"}

Section layouts:
- **properties**: {"columns": 2, "items": [{"label":"string","value":"string","color":"string (opt)","link":"string (opt, injects chat message)","qualifier":"string (opt, overrides card qualifier for this link)"}]}
- **chart**: Any chart type JSON (area, bar, gauge, sparkline, etc.) + optional "annotations": [{"y":number,"label":"string","color":"string","style":"solid|dashed"}]
- **alert-list**: {"items": [{"severity":"string","message":"string","time":"string"}]}
- **timeline**: {"events": [{"time":"string","label":"string","severity":"string (opt)","icon":"string (opt)"}]}
- **actions**: {"buttons": [ActionButton schema from above + optional "qualifier":"string" to override card qualifier]}
- **text**: {"body": "markdown string"}
- **table**: {"columns": ["Col1",...], "rows": [{...}]}

**Output type selection:**
- Questions about a single named entity → object-detail
- Fleet-wide overviews, comparisons, or multi-entity views → dashboard
- Ambiguous → prefer object-detail if one entity is the primary focus
- Chart annotations: limit to 1-2 per chart for readability

### Canvas fences

Some interests have Target: canvas in the catalog. When producing the final output block
for a canvas-targeted interest, use the fence language ` + "`canvas-object-detail`" + ` or
` + "`canvas-dashboard`" + ` instead of the regular ` + "`object-detail`" + ` or ` + "`dashboard`" + `.
The JSON payload is identical — only the fence language changes.

Example:
` + "```canvas-object-detail" + `
{ "type": "object-detail", "kind": "volume", "name": "vol_prod_01", ... }
` + "```" + `

This causes the content to open in a persistent canvas tab beside the chat.
After emitting a canvas fence, also emit a short chat message confirming what
was opened (e.g. "I've opened the volume detail for vol_prod_01 in the canvas.").

You may also use canvas fences for ad-hoc requests when the user explicitly
asks to "pin", "keep open", or "show in the canvas", even if the interest
does not specify canvas as the target.

### Data limits

When building charts, limit data arrays to roughly 50–100 rows. The UI will downsample arrays larger than 200 points, but fewer points render faster and look cleaner. Aggregate or bucket data server-side when the source returns hundreds of data points.
`

// interestManagementSpec is appended to the system prompt when save_interest
// and delete_interest tools are available (read-write mode).
const interestManagementSpec = `

## Interest Management

When the user asks you to create, update, or delete a custom interest, use the
save_interest and delete_interest tools. Follow this workflow:

**Creating a new interest:**
1. Ask clarifying questions if the user's request is vague (what data sources, what layout)
2. Infer the metadata: pick a short lowercase-hyphen id, a human name, relevant triggers, and the required capability IDs (product-specific — match the IDs surfaced by the connected MCP servers)
3. Draft the interest body — a markdown description of the dashboard layout, panels, and analysis steps
4. Show the user the complete interest (id, name, triggers, requires, body) and ask for confirmation
5. Only call save_interest after the user explicitly approves
6. If a required capability is not currently connected, warn the user the interest will not activate until that capability is available

**Updating an existing interest:**
1. Call get_interest(id) to retrieve the current body
2. Apply the requested changes
3. Show the updated interest to the user for confirmation
4. Call save_interest with the updated fields after approval

**Deleting an interest:**
1. Confirm with the user before calling delete_interest
2. Built-in interests cannot be deleted — inform the user if they try

**Listing interests:**
When the user asks "what interests do I have?" or similar, answer from the interest catalog table above.
`

// marshalToolInput is a helper to serialize tool input for display.
func marshalToolInput(input json.RawMessage) string {
	if len(input) == 0 {
		return "{}"
	}
	return string(input)
}

// isReadOnlyTool reports whether the tool with the given name is annotated
// as read-only by the connected MCP server. Tools that are not present
// (e.g. internal tools) are reported as not read-only so that ask-on-write
// errs on the side of prompting.
func (a *Agent) isReadOnlyTool(name string) bool {
	for _, t := range a.Router.Tools() {
		if t.Name == name {
			return t.ReadOnlyHint
		}
	}
	return false
}

// filteredTools returns tools from the router, filtered by capability states
// and the agent's read-only/read-write mode, plus any internal tools
// registered on the agent. It returns ErrTooManyTools (wrapped with detail)
// if the resulting list exceeds MaxToolsPerRequest.
func (a *Agent) filteredTools() ([]llm.ToolDef, error) {
	allTools := a.Router.Tools()
	var mcpTools []llm.ToolDef
	if a.CapStates == nil || a.ToolServerMap == nil {
		// No capability filter configured — pass MCP tools through as-is.
		// Mode filtering only applies when capability filtering is active,
		// which is how the chat handler always wires the agent in
		// production.
		mcpTools = allTools
	} else {
		for _, t := range allTools {
			capID := a.ToolServerMap[t.Name]
			state, hasState := a.CapStates[capID]
			if hasState && state == capability.StateOff {
				continue
			}
			// Write tools are sent to the LLM only when the global mode is
			// read-write OR this capability is in ask-on-write state (which
			// promises an interactive approval before any write executes).
			allowWrites := a.Mode == "read-write" || (hasState && state == capability.StateAskOnWrite)
			if !allowWrites && !t.ReadOnlyHint {
				continue
			}
			mcpTools = append(mcpTools, t)
		}
	}

	// In-band tool routing (S7a): further restrict the (already
	// capability/mode-filtered) MCP tools to the active group set —
	// always-on groups unioned with whatever the model has loaded this turn.
	// Internal tools (load_tools, get_interest, …) are unaffected; they are
	// appended below regardless. When nothing is active, only internal tools
	// remain, which is the intended "load before you act" starting state.
	if a.ToolRoutingMode == ToolRoutingInBand && a.ToolServerMap != nil {
		active := a.activeGroups()
		activeTools := a.activeToolSet()
		routed := make([]llm.ToolDef, 0, len(mcpTools))
		perGroup := make(map[string]int)
		for _, t := range mcpTools {
			capID := a.ToolServerMap[t.Name]
			// A tool is included if its whole group is active (group-level
			// S7a) or it was loaded individually (tool-level S8).
			if !active[capID] && !activeTools[t.Name] {
				continue
			}
			routed = append(routed, t)
			perGroup[capID]++
		}
		if err := a.checkRoutedBudget(perGroup); err != nil {
			a.Logger.Error("routed tool budget exceeded",
				"per_group", perGroup, "max", MaxToolsPerRequest, "max_routed", a.MaxRoutedTools)
			return nil, err
		}
		mcpTools = routed
	}

	tools := a.appendInternalTools(mcpTools)

	if len(tools) > MaxToolsPerRequest {
		// Collect per-capability counts for diagnostics.
		perCap := make(map[string]int)
		if a.ToolServerMap != nil {
			for _, t := range mcpTools {
				perCap[a.ToolServerMap[t.Name]]++
			}
		}
		a.Logger.Error("tool budget exceeded",
			"total", len(tools),
			"max", MaxToolsPerRequest,
			"mode", a.Mode,
			"per_capability", perCap,
		)
		return nil, fmt.Errorf("%w: %d enabled, max %d (mode=%s). Disable an MCP capability or switch to read-only mode in chat settings.",
			ErrTooManyTools, len(tools), MaxToolsPerRequest, a.Mode)
	}

	return tools, nil
}

// loadToolsDef returns the LLM tool definition for the internal load_tools
// tool (S7a). The model calls it to make a capability group's tools available.
func loadToolsDef() llm.ToolDef {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"groups": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Group IDs to load (loads the whole group), taken from the Tool Groups table in the system prompt (e.g. [\"jira\"]). Loading is additive — call again to add more mid-task.",
			},
			"tools": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Individual tool names to load. Use this for groups marked 'Large server' in the system prompt, loading only the specific tools you need (e.g. [\"ontap_get_volume\"]) instead of the whole group. Additive and idempotent.",
			},
		},
	})
	return llm.ToolDef{
		Name:         "load_tools",
		Description:  "Make tools available to call. Pass `groups` to load whole capability groups, and/or `tools` to load individual tools from a large group. You MUST call this before using a tool. Calls are additive and idempotent; supply at least one of groups or tools.",
		Schema:       schema,
		ReadOnlyHint: true,
	}
}

// handleLoadTools is the internal handler for the load_tools tool. It marks the
// requested groups active for the remainder of the turn so subsequent
// filteredTools() calls include their tools. Unknown groups produce a
// structured, recoverable result (not a hard error) so the model can retry
// with a valid ID. Idempotent: repeated calls union into the active set.
func (a *Agent) handleLoadTools(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Groups []string `json:"groups"`
		Tools  []string `json:"tools"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("load_tools: invalid input: %w", err)
	}
	if len(req.Groups) == 0 && len(req.Tools) == 0 {
		return "", fmt.Errorf("load_tools: supply at least one of 'groups' (group IDs) or 'tools' (individual tool names)")
	}

	var knownGroups, unknownGroups, knownTools, unknownTools []string
	a.routingMu.Lock()
	for _, g := range req.Groups {
		if a.groupExistsLocked(g) {
			a.loadedGroups[g] = true
			knownGroups = append(knownGroups, g)
		} else {
			unknownGroups = append(unknownGroups, g)
		}
	}
	for _, t := range req.Tools {
		if a.toolExistsLocked(t) {
			a.loadedTools[t] = true
			knownTools = append(knownTools, t)
		} else {
			unknownTools = append(unknownTools, t)
		}
	}
	// Telemetry: count this invocation; any invocation after the first is a
	// mid-task reload.
	a.stats.LoadCalls++
	if a.stats.LoadCalls > 1 {
		a.stats.Reloads++
	}
	loaded := make([]string, 0, len(a.loadedGroups))
	for g := range a.loadedGroups {
		loaded = append(loaded, g)
	}
	loadedToolList := make([]string, 0, len(a.loadedTools))
	for t := range a.loadedTools {
		loadedToolList = append(loadedToolList, t)
	}
	a.routingMu.Unlock()

	sort.Strings(loaded)
	sort.Strings(loadedToolList)
	loadedStr := "none"
	if len(loaded) > 0 || len(loadedToolList) > 0 {
		parts := make([]string, 0, 2)
		if len(loaded) > 0 {
			parts = append(parts, "groups: "+strings.Join(loaded, ", "))
		}
		if len(loadedToolList) > 0 {
			parts = append(parts, "tools: "+strings.Join(loadedToolList, ", "))
		}
		loadedStr = strings.Join(parts, "; ")
	}

	if len(unknownGroups) > 0 || len(unknownTools) > 0 {
		sort.Strings(unknownGroups)
		sort.Strings(unknownTools)
		var problems []string
		if len(unknownGroups) > 0 {
			problems = append(problems, fmt.Sprintf("unknown group(s): %s (available: %s)",
				strings.Join(unknownGroups, ", "), strings.Join(a.groupIDs(), ", ")))
		}
		if len(unknownTools) > 0 {
			problems = append(problems, fmt.Sprintf("unknown tool(s): %s", strings.Join(unknownTools, ", ")))
		}
		return fmt.Sprintf("Could not load some items — %s. Currently loaded: %s.",
			strings.Join(problems, "; "), loadedStr), nil
	}

	var ack []string
	if len(knownGroups) > 0 {
		sort.Strings(knownGroups)
		ack = append(ack, "group(s): "+strings.Join(knownGroups, ", "))
	}
	if len(knownTools) > 0 {
		sort.Strings(knownTools)
		ack = append(ack, "tool(s): "+strings.Join(knownTools, ", "))
	}
	return fmt.Sprintf("Loaded %s. Their tools are now available to call. Currently loaded: %s.",
		strings.Join(ack, " and "), loadedStr), nil
}

// groupExistsLocked reports whether id names a known group. Caller holds
// routingMu (the group menu is immutable, but this keeps the access pattern
// consistent).
func (a *Agent) groupExistsLocked(id string) bool {
	for _, g := range a.groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

// toolExistsLocked reports whether name is a tool belonging to any offered
// group (the set the model may load individually via load_tools tools:[…]).
// Caller holds routingMu.
func (a *Agent) toolExistsLocked(name string) bool {
	for _, g := range a.groups {
		for _, tn := range g.ToolNames {
			if tn == name {
				return true
			}
		}
	}
	return false
}

// groupIDs returns the sorted list of offered group IDs (for diagnostics).
func (a *Agent) groupIDs() []string {
	ids := make([]string, 0, len(a.groups))
	for _, g := range a.groups {
		ids = append(ids, g.ID)
	}
	sort.Strings(ids)
	return ids
}

// activeGroups returns the set of currently active group IDs: always-on groups
// unioned with everything the model has loaded this turn.
func (a *Agent) activeGroups() map[string]bool {
	active := make(map[string]bool, len(a.AlwaysOnGroups))
	for _, g := range a.AlwaysOnGroups {
		active[g] = true
	}
	a.routingMu.Lock()
	for g := range a.loadedGroups {
		active[g] = true
	}
	a.routingMu.Unlock()
	return active
}

// activeToolSet returns the set of individually-loaded tool names (S8).
func (a *Agent) activeToolSet() map[string]bool {
	a.routingMu.Lock()
	defer a.routingMu.Unlock()
	tools := make(map[string]bool, len(a.loadedTools))
	for t := range a.loadedTools {
		tools[t] = true
	}
	return tools
}

// effectiveLoadedGroupsLocked returns the set of group IDs considered loaded
// for telemetry: explicitly loaded groups unioned with the groups owning any
// individually-loaded tool (S8). Caller holds routingMu.
func (a *Agent) effectiveLoadedGroupsLocked() []string {
	set := make(map[string]bool, len(a.loadedGroups))
	for g := range a.loadedGroups {
		set[g] = true
	}
	for t := range a.loadedTools {
		if capID, ok := a.ToolServerMap[t]; ok && capID != "" {
			set[capID] = true
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// shouldForceGroupLoad reports whether the agent should inject a corrective
// nudge because the model is about to answer tool-lessly without having
// activated any group while groups are available. Always-on groups count as
// active, so deployments that preload a baseline never trigger this.
func (a *Agent) shouldForceGroupLoad() bool {
	if a.ToolRoutingMode != ToolRoutingInBand || !a.ForceGroupLoad {
		return false
	}
	if len(a.groups) == 0 {
		return false
	}
	// A tool-level load (S8) counts as a selection too, so don't nudge if the
	// model has loaded individual tools even without a whole group.
	return len(a.activeGroups()) == 0 && len(a.activeToolSet()) == 0
}

// checkRoutedBudget asserts the post-routing per-group tool counts fit within
// the effective budget. When a single group alone exceeds the budget, the
// error names it — the signal that one server's fan-out is irreducible and
// needs an operator-side fix.
func (a *Agent) checkRoutedBudget(perGroup map[string]int) error {
	budget := MaxToolsPerRequest
	if a.MaxRoutedTools > 0 && a.MaxRoutedTools < budget {
		budget = a.MaxRoutedTools
	}
	total := 0
	for _, n := range perGroup {
		total += n
	}
	if total <= budget {
		return nil
	}
	var offenders []string
	for g, n := range perGroup {
		if n > budget {
			offenders = append(offenders, fmt.Sprintf("%q (%d tools)", g, n))
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		return fmt.Errorf("%w: loaded group %s alone exceeds the %d-tool budget; this server's tool fan-out is irreducible and needs an operator-side fix",
			ErrTooManyTools, strings.Join(offenders, ", "), budget)
	}
	return fmt.Errorf("%w: %d tools across the loaded groups exceed the %d-tool budget; load fewer groups per turn",
		ErrTooManyTools, total, budget)
}

// LastRoutingStats returns a copy of the telemetry from the most recent Run.
// Meaningful only when in-band routing is enabled.
func (a *Agent) LastRoutingStats() RoutingStats {
	a.routingMu.Lock()
	defer a.routingMu.Unlock()
	s := a.stats
	s.GroupsLoaded = a.effectiveLoadedGroupsLocked()
	tools := make([]string, 0, len(a.loadedTools))
	for t := range a.loadedTools {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	s.ToolsLoaded = tools
	return s
}

// logRoutingStats finalizes and emits the per-run routing telemetry (Layer 5).
func (a *Agent) logRoutingStats() {
	a.routingMu.Lock()
	loaded := a.effectiveLoadedGroupsLocked()
	tools := make([]string, 0, len(a.loadedTools))
	for t := range a.loadedTools {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	a.stats.GroupsLoaded = loaded
	a.stats.ToolsLoaded = tools
	a.stats.Skipped = a.stats.GroupsOffered > 0 && a.stats.LoadCalls == 0 && len(a.AlwaysOnGroups) == 0
	a.stats.Compliant = a.stats.LoadCalls > 0
	stats := a.stats
	a.routingMu.Unlock()

	a.Logger.Info("tool routing summary",
		"mode", stats.Mode,
		"groups_offered", stats.GroupsOffered,
		"groups_loaded", loaded,
		"tools_loaded", tools,
		"load_calls", stats.LoadCalls,
		"reloads", stats.Reloads,
		"skipped", stats.Skipped,
		"compliant", stats.Compliant,
	)
}

// appendInternalTools adds internal tool definitions to the tool list.
// Read-write-only tools are excluded when the agent mode is not "read-write".
func (a *Agent) appendInternalTools(tools []llm.ToolDef) []llm.ToolDef {
	for _, it := range a.InternalTools {
		if it.ReadWriteOnly && a.Mode != "read-write" {
			continue
		}
		tools = append(tools, it.Def)
	}
	return tools
}
