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
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ebarron/netapp-chat-service/llm"
)

// headerRoundTripper wraps an http.RoundTripper to inject extra headers.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}

// ServerConfig describes how to connect to a single MCP server.
type ServerConfig struct {
	Name     string            `yaml:"name" json:"name"`         // e.g. "harvest-mcp", "ontap-mcp"
	Endpoint string            `yaml:"endpoint" json:"endpoint"` // e.g. "http://localhost:8082"
	Headers  map[string]string `yaml:"headers" json:"headers"`   // extra HTTP headers (e.g. auth tokens)
	// ReadOnlyTools is an allowlist of tool names that should be treated as
	// read-only for filtering, overriding/supplementing MCP annotations. Use
	// this for MCPs we don't control (e.g. Grafana, third-party) that don't
	// publish ToolAnnotations.ReadOnlyHint.
	ReadOnlyTools []string `yaml:"read_only_tools" json:"read_only_tools,omitempty"`
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

	// Build a custom http.Client that injects auth headers if configured.
	var httpClient *http.Client
	if len(cfg.Headers) > 0 {
		httpClient = &http.Client{
			Transport: &headerRoundTripper{
				base:    http.DefaultTransport,
				headers: cfg.Headers,
			},
		}
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:   cfg.Endpoint,
		HTTPClient: httpClient,
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
		allowSet := make(map[string]bool, len(sc.cfg.ReadOnlyTools))
		for _, t := range sc.cfg.ReadOnlyTools {
			allowSet[t] = true
		}
		for _, tool := range sc.tools {
			if prev, exists := r.toolMap[tool.Name]; exists {
				r.logger.Warn("duplicate MCP tool name, skipping",
					"tool", tool.Name, "server", name, "kept", prev)
				continue
			}
			r.toolMap[tool.Name] = name
			r.toolDefs = append(r.toolDefs, r.convertTool(tool, name, allowSet))
		}
	}
}

// convertTool converts an MCP tool to an llm.ToolDef. ReadOnlyHint is
// populated from the MCP tool annotations when present, then overridden by
// the per-server allowlist. Tools without annotations and not on the
// allowlist default to ReadOnlyHint=false (i.e. assumed write) so they are
// filtered out in read-only mode unless explicitly marked safe.
func (r *Router) convertTool(t *mcp.Tool, serverName string, allowSet map[string]bool) llm.ToolDef {
	schema, _ := json.Marshal(t.InputSchema)
	def := llm.ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Schema:      schema,
	}
	switch {
	case t.Annotations != nil:
		def.ReadOnlyHint = t.Annotations.ReadOnlyHint
		if t.Annotations.DestructiveHint != nil {
			def.DestructiveHint = *t.Annotations.DestructiveHint
		}
	case allowSet[t.Name]:
		// Allowlist override for unannotated tools.
	default:
		r.logger.Debug("mcp tool has no annotations, treating as write-capable",
			"server", serverName, "tool", t.Name)
	}
	if allowSet[t.Name] {
		def.ReadOnlyHint = true
	}
	return def
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
