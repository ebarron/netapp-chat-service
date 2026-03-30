// Package agent implements the chatbot's agentic tool-use loop. It
// orchestrates the conversation between the LLM and MCP tool servers,
// streaming events back to the caller.
//
// Design ref: docs/chatbot-design-spec.md §2.2
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebarron/netapp-chat-service/internal/capability"
	"github.com/ebarron/netapp-chat-service/internal/llm"
	"github.com/ebarron/netapp-chat-service/internal/mcpclient"
)

// DefaultMaxIterations is the safety limit for tool-call rounds per user
// message. After this many iterations the LLM is asked to summarize.
const DefaultMaxIterations = 10

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

	tools := a.filteredTools()

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
				if a.CapStates != nil && capID != "" {
					if state, ok := a.CapStates[capID]; ok && state == capability.StateAsk {
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
}

// BuildSystemPrompt constructs the system prompt from the current state.
// This is a convenience to generate the prompt that includes tool context.
// If interestIndex is non-empty, the chart format spec and interest catalog
// are appended so the LLM knows how to produce dashboard panels.
// If canvasTabs is non-empty, a canvas context section is appended so the
// LLM knows what items the user has pinned in the canvas.
func BuildSystemPrompt(cfg SystemPromptConfig, router mcpclient.ToolRouter, interestIndex string, canvasTabs ...CanvasTabSummary) string {
	servers := router.ConnectedServers()
	tools := router.Tools()

	prompt := fmt.Sprintf("You are the %s", cfg.ProductName)
	if cfg.ProductDescription != "" {
		prompt += ", " + cfg.ProductDescription
	}
	prompt += "\n\n"

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
		prompt += "4. If the interest tells you to call a **render tool** (e.g. `render_volume_detail`),\n"
		prompt += "   you MUST call it — that is the ONLY way to produce the view. The frontend cannot\n"
		prompt += "   display volume details from your text. NEVER skip a render tool call, even if you\n"
		prompt += "   already have the data from a previous turn. Always call the render tool.\n"
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
{"type":"proposal","title":"string","command":"string","format":"ontap-cli"}

### Dashboard-only panel types

**alert-summary** — Severity count badges (clickable). Do not include "ok" — only real alert severities.
{"type":"alert-summary","data":{"critical":number,"warning":number,"info":number}}

**resource-table** — Clickable resource list
{"type":"resource-table","title":"string","columns":["Col1","Col2",...],"rows":[{"name":"string (always required — used for click target)","Col1":"value","Col2":"value",...}]}
Row objects MUST include a key for every entry in "columns" whose name matches the column exactly. The "name" field is always required (used for the click action) and should also appear under the first column key.
For ONTAP resources, always include hidden "cluster" and "svm" fields in each row (not in columns) so the click action can uniquely identify the resource. Example: {"name":"vol1","Volume":"vol1","Used %":33,"cluster":"cls1","svm":"svm1"}

**action-button** — Clickable action triggers
{"type":"action-button","buttons":[{"label":"string","action":"execute|message","tool":"string (for execute)","params":{} (for execute),"message":"string (for message)","icon":"string (opt)","variant":"primary|outline"}]}

### Object detail — use language "object-detail"

For questions about a single entity (volume, cluster, alert, SVM, aggregate),
produce a rich detail view instead of a dashboard:

` + "```" + `object-detail
{
  "type": "object-detail",
  "kind": "volume | cluster | alert | aggregate | svm | string",
  "name": "Display name or title",
  "status": "critical | warning | ok | info",
  "subtitle": "Brief context line",
  "qualifier": "identity context appended to action messages (see below)",
  "sections": [
    { "title": "Section Title", "layout": "properties|chart|alert-list|timeline|actions|text|table", "data": { ... } }
  ]
}
` + "```" + `

The **qualifier** field carries the identity keys needed to uniquely look up this object in follow-up requests. The UI automatically appends it to every action message from this detail view. Examples by kind:
- volume: "on SVM vdbench on cluster cls1"
- svm: "on cluster cls1"
- aggregate: "on cluster cls1"
- alert: "(alert-id abc123)" or similar unique identifier
- cluster: omit or leave empty (cluster name alone is unique)
Always set qualifier so action buttons and property links work without losing context.

**Per-item qualifier override:** Property items and action buttons support an optional per-item "qualifier" field that overrides the card-level qualifier for that specific link. This is essential when a link targets a *different kind* of object whose identity keys differ from the current object.
- Set "qualifier": "" (empty string) to suppress the qualifier entirely — use this for links to clusters (cluster name alone is unique).
- Set "qualifier": "on cluster cls1" for links to SVMs or aggregates from a volume detail (the target needs cluster context but not SVM context).
- Omit the per-item qualifier to inherit the card-level qualifier — use this for same-kind follow-ups (e.g. "Show snapshots" on a volume detail).
Example property item linking to a cluster: {"label":"Cluster","value":"cls1","link":"Show cluster cls1","qualifier":""}
Example property item linking to an SVM from a volume: {"label":"SVM","value":"svm1","link":"Tell me about SVM svm1","qualifier":"on cluster cls1"}
Example action button with no override (inherits card qualifier): {"label":"Show Snapshots","action":"message","message":"Show snapshots for vol1"}

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
2. Infer the metadata: pick a short lowercase-hyphen id, a human name, relevant triggers, and the required capabilities (harvest, ontap, grafana)
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

// filteredTools returns tools from the router, filtered by capability states,
// plus any internal tools registered on the agent.
func (a *Agent) filteredTools() []llm.ToolDef {
	allTools := a.Router.Tools()
	if a.CapStates == nil {
		return a.appendInternalTools(allTools)
	}

	// Filter tools by checking if the tool's capability is off.
	// We use ToolServerMap to determine which capability each tool belongs to.
	if a.ToolServerMap == nil {
		return a.appendInternalTools(allTools)
	}

	var filtered []llm.ToolDef
	for _, t := range allTools {
		capID := a.ToolServerMap[t.Name]
		if state, ok := a.CapStates[capID]; ok && state == capability.StateOff {
			continue
		}
		filtered = append(filtered, t)
	}
	return a.appendInternalTools(filtered)
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
