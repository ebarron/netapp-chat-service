package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ebarron/netapp-chat-service/llm"
)

// MockRouter is a deterministic Router replacement for tests. It allows tests
// to define available tools and their results without any MCP server.
//
// Design ref: docs/chatbot-design-spec.md §9.3
type MockRouter struct {
	mu          sync.RWMutex
	tools       []llm.ToolDef
	results     map[string]string // tool name -> result text
	errors      map[string]error  // tool name -> error
	calls       []llm.ToolCall    // recorded calls
	servers     []string          // simulated connected server names
	toolServers map[string]string // tool name -> server name
}

// NewMockRouter creates a MockRouter with the given tools pre-registered.
func NewMockRouter(tools []llm.ToolDef) *MockRouter {
	return &MockRouter{
		tools:   tools,
		results: make(map[string]string),
		errors:  make(map[string]error),
	}
}

// SetResult configures the response for a tool call.
func (m *MockRouter) SetResult(toolName, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[toolName] = result
}

// SetError configures an error response for a tool call.
func (m *MockRouter) SetError(toolName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[toolName] = err
}

// SetServers sets the list of connected server names.
func (m *MockRouter) SetServers(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = names
}

// AddTool appends a tool definition to the mock.
func (m *MockRouter) AddTool(tool llm.ToolDef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools = append(m.tools, tool)
}

// Tools returns the configured tool definitions.
func (m *MockRouter) Tools() []llm.ToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	defs := make([]llm.ToolDef, len(m.tools))
	copy(defs, m.tools)
	return defs
}

// CallTool records the call and returns the configured result or error.
func (m *MockRouter) CallTool(_ context.Context, tc llm.ToolCall) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, tc)
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, ok := m.errors[tc.Name]; ok {
		return "", err
	}
	if result, ok := m.results[tc.Name]; ok {
		return result, nil
	}
	return "", fmt.Errorf("mock: no result configured for tool %q", tc.Name)
}

// ConnectedServers returns the simulated server names.
func (m *MockRouter) ConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, len(m.servers))
	copy(names, m.servers)
	return names
}

// SetToolServer maps a tool name to a server name for ToolMap().
func (m *MockRouter) SetToolServer(toolName, serverName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toolServers == nil {
		m.toolServers = make(map[string]string)
	}
	m.toolServers[toolName] = serverName
}

// ToolMap returns the tool-name-to-server-name mapping.
func (m *MockRouter) ToolMap() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]string, len(m.toolServers))
	for k, v := range m.toolServers {
		result[k] = v
	}
	return result
}

// Calls returns all recorded tool calls.
func (m *MockRouter) Calls() []llm.ToolCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	calls := make([]llm.ToolCall, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// ToolRouter is the interface that both Router and MockRouter satisfy.
// Used by the agent loop to decouple from the real MCP implementation.
type ToolRouter interface {
	Tools() []llm.ToolDef
	CallTool(ctx context.Context, tc llm.ToolCall) (string, error)
	ConnectedServers() []string
	ToolMap() map[string]string
}

// Compile-time interface checks.
var _ ToolRouter = (*Router)(nil)
var _ ToolRouter = (*MockRouter)(nil)

// MockTool is a helper to create a simple tool definition for tests.
func MockTool(name, description string) llm.ToolDef {
	schema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
	return llm.ToolDef{
		Name:        name,
		Description: description,
		Schema:      schema,
	}
}
// MockReadOnlyTool returns a MockTool with ReadOnlyHint=true so it survives
// read-only mode filtering.
func MockReadOnlyTool(name, description string) llm.ToolDef {
        t := MockTool(name, description)
        t.ReadOnlyHint = true
        return t
}