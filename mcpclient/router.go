// Package mcpclient manages connections to MCP servers and routes tool calls.
// It wraps the official MCP Go SDK's client session, adding tool caching,
// connection lifecycle management, and conversion to/from the llm package types.
//
// Design ref: docs/chatbot-design-spec.md §5.4
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ebarron/netapp-chat-service/llm"
)

// ServerConfig describes how to connect to a single MCP server.
type ServerConfig struct {
	Name     string `yaml:"name" json:"name"`         // e.g. "harvest-mcp", "ontap-mcp"
	Endpoint string `yaml:"endpoint" json:"endpoint"` // e.g. "http://localhost:8082"
}

// Router manages connections to multiple MCP servers. It discovers tools from
// each server, provides a merged tool list, and routes tool calls to the
// correct server.
type Router struct {
	mu       sync.RWMutex
	servers  map[string]*serverConn // keyed by ServerConfig.Name
	toolMap  map[string]string      // tool name -> server name
	toolDefs []llm.ToolDef          // cached merged list
	logger   *slog.Logger
}

// serverConn holds a live MCP client session plus its tool cache.
type serverConn struct {
	cfg     ServerConfig
	client  *mcp.Client
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

// NewRouter creates a Router. It does not connect immediately -- call Connect
// to establish sessions.
func NewRouter(logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		servers: make(map[string]*serverConn),
		toolMap: make(map[string]string),
		logger:  logger,
	}
}

// Connect establishes a session with the given MCP server and discovers its
// tools. If the server is already connected, it reconnects.
func (r *Router) Connect(ctx context.Context, cfg ServerConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close existing connection if any.
	if existing, ok := r.servers[cfg.Name]; ok {
		if existing.session != nil {
			_ = existing.session.Close()
		}
		delete(r.servers, cfg.Name)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nabox-chatbot",
		Version: "1.0.0",
	}, nil)

	transport := &mcp.StreamableClientTransport{
		Endpoint: cfg.Endpoint,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp connect %q: %w", cfg.Name, err)
	}

	// Discover tools.
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			_ = session.Close()
			return fmt.Errorf("mcp list tools %q: %w", cfg.Name, err)
		}
		tools = append(tools, tool)
	}

	r.servers[cfg.Name] = &serverConn{
		cfg:     cfg,
		client:  client,
		session: session,
		tools:   tools,
	}

	r.logger.Info("mcp connected",
		"server", cfg.Name,
		"endpoint", cfg.Endpoint,
		"tools", len(tools),
	)

	r.rebuildToolIndex()
	return nil
}

// Disconnect closes the session for the named server and removes its tools.
func (r *Router) Disconnect(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sc, ok := r.servers[name]
	if !ok {
		return nil
	}

	var err error
	if sc.session != nil {
		err = sc.session.Close()
	}
	delete(r.servers, name)
	r.rebuildToolIndex()
	return err
}

// Tools returns the merged tool definitions from all connected servers,
// converted to llm.ToolDef for the LLM provider.
func (r *Router) Tools() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Return a copy to avoid races.
	defs := make([]llm.ToolDef, len(r.toolDefs))
	copy(defs, r.toolDefs)
	return defs
}

// CallTool routes a tool call to the correct MCP server and returns the result
// as a string (text content concatenated).
func (r *Router) CallTool(ctx context.Context, tc llm.ToolCall) (string, error) {
	r.mu.RLock()
	serverName, ok := r.toolMap[tc.Name]
	if !ok {
		r.mu.RUnlock()
		return "", fmt.Errorf("unknown tool: %q", tc.Name)
	}
	sc := r.servers[serverName]
	r.mu.RUnlock()

	// Parse input arguments.
	var args map[string]any
	if len(tc.Input) > 0 {
		if err := json.Unmarshal(tc.Input, &args); err != nil {
			return "", fmt.Errorf("invalid tool input for %q: %w", tc.Name, err)
		}
	}

	result, err := sc.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tc.Name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("tool call %q failed: %w", tc.Name, err)
	}

	if result.IsError {
		return "", fmt.Errorf("tool %q returned error: %s", tc.Name, extractText(result))
	}

	return extractText(result), nil
}

// ToolMap returns a copy of the tool-name-to-server-name mapping.
// Used to build capability routing for the agent.
func (r *Router) ToolMap() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string, len(r.toolMap))
	for k, v := range r.toolMap {
		result[k] = v
	}
	return result
}

// ConnectedServers returns the names of currently connected MCP servers.
func (r *Router) ConnectedServers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	return names
}

// Close disconnects all MCP servers.
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, sc := range r.servers {
		if sc.session != nil {
			if err := sc.session.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(r.servers, name)
	}
	r.toolDefs = nil
	r.toolMap = make(map[string]string)
	return firstErr
}

// rebuildToolIndex rebuilds the merged tool list and routing map.
// Must be called with r.mu held.
func (r *Router) rebuildToolIndex() {
	r.toolMap = make(map[string]string)
	r.toolDefs = nil

	for name, sc := range r.servers {
		for _, tool := range sc.tools {
			if prev, exists := r.toolMap[tool.Name]; exists {
				r.logger.Warn("duplicate MCP tool name, skipping",
					"tool", tool.Name, "server", name, "kept", prev)
				continue
			}
			r.toolMap[tool.Name] = name
			r.toolDefs = append(r.toolDefs, convertTool(tool))
		}
	}
}

// convertTool converts an MCP tool to an llm.ToolDef.
func convertTool(t *mcp.Tool) llm.ToolDef {
	schema, _ := json.Marshal(t.InputSchema)
	return llm.ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Schema:      schema,
	}
}

// extractText concatenates all text content blocks from a tool result.
func extractText(result *mcp.CallToolResult) string {
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if text != "" {
				text += "\n"
			}
			text += tc.Text
		}
	}
	return text
}

// ConnectAll connects to all the given MCP servers with retries. Each server
// is attempted up to maxAttempts times with retryDelay between attempts.
// Servers that fail all attempts are logged but do not cause an error return —
// the router gracefully handles missing servers at tool-call time.
func (r *Router) ConnectAll(servers []ServerConfig, maxAttempts int, retryDelay time.Duration) {
	for _, srv := range servers {
		var connected bool
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := r.Connect(ctx, srv); err != nil {
				r.logger.Warn("could not connect to MCP server",
					"server", srv.Name, "endpoint", srv.Endpoint,
					"attempt", attempt, "error", err)
				cancel()
				time.Sleep(retryDelay)
				continue
			}
			cancel()
			r.logger.Info("connected to MCP server", "server", srv.Name, "endpoint", srv.Endpoint)
			connected = true
			break
		}
		if !connected {
			r.logger.Error("failed to connect to MCP server after retries",
				"server", srv.Name, "endpoint", srv.Endpoint,
				"attempts", maxAttempts)
		}
	}
}
