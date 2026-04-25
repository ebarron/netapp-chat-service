package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Discoverer finds MCP servers dynamically.
type Discoverer interface {
	// Discover returns the current set of MCP servers.
	Discover(ctx context.Context) ([]ServerConfig, error)
}

// StaticDiscoverer returns a fixed list of servers (equivalent to explicit config).
type StaticDiscoverer struct {
	Servers []ServerConfig
}

func (s *StaticDiscoverer) Discover(_ context.Context) ([]ServerConfig, error) {
	return s.Servers, nil
}

// DockerDiscoverer discovers MCP servers from Docker container labels.
// Containers must have the label "mcp.discover=true" and the following
// additional labels:
//
//   - mcp.name            — server name (e.g. "harvest-mcp")
//   - mcp.endpoint        — HTTP endpoint URL (e.g. "http://harvest-mcp:8082")
//   - mcp.capability      — capability ID (optional, e.g. "harvest")
//   - mcp.read_only_tools — comma-separated allowlist of tool names to treat
//                          as read-only when an MCP server doesn't publish
//                          ToolAnnotations.ReadOnlyHint (optional)
//
// Only running containers are considered.
type DockerDiscoverer struct {
	// SocketPath is the Docker socket path. Defaults to /var/run/docker.sock.
	SocketPath string
	// LabelPrefix is the prefix for discovery labels. Defaults to "mcp.".
	LabelPrefix string

	client *http.Client
	logger *slog.Logger
}

// NewDockerDiscoverer creates a DockerDiscoverer.
func NewDockerDiscoverer(logger *slog.Logger, opts ...DockerOption) *DockerDiscoverer {
	d := &DockerDiscoverer{
		SocketPath:  "/var/run/docker.sock",
		LabelPrefix: "mcp.",
		logger:      logger,
	}
	for _, opt := range opts {
		opt(d)
	}
	d.client = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", d.SocketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
	return d
}

// DockerOption configures a DockerDiscoverer.
type DockerOption func(*DockerDiscoverer)

// WithSocketPath sets the Docker socket path.
func WithSocketPath(path string) DockerOption {
	return func(d *DockerDiscoverer) { d.SocketPath = path }
}

// WithLabelPrefix sets the label prefix for discovery.
func WithLabelPrefix(prefix string) DockerOption {
	return func(d *DockerDiscoverer) { d.LabelPrefix = prefix }
}

// dockerContainer is the subset of Docker container JSON we need.
type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
	State  string            `json:"State"`
}

// Discover queries the Docker API for containers with discovery labels.
func (d *DockerDiscoverer) Discover(ctx context.Context) ([]ServerConfig, error) {
	discoverLabel := d.LabelPrefix + "discover"

	// Build filter: {"label":["mcp.discover=true"]}
	filters := map[string][]string{
		"label":  {discoverLabel + "=true"},
		"status": {"running"},
	}
	filtersJSON, _ := json.Marshal(filters)

	u := fmt.Sprintf("http://docker/v1.41/containers/json?filters=%s",
		url.QueryEscape(string(filtersJSON)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("docker discovery: build request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker discovery: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker discovery: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("docker discovery: decode response: %w", err)
	}

	var servers []ServerConfig
	nameLabel := d.LabelPrefix + "name"
	endpointLabel := d.LabelPrefix + "endpoint"
	readOnlyToolsLabel := d.LabelPrefix + "read_only_tools"

	for _, c := range containers {
		name := c.Labels[nameLabel]
		endpoint := c.Labels[endpointLabel]

		if name == "" || endpoint == "" {
			containerName := ""
			if len(c.Names) > 0 {
				containerName = c.Names[0]
			}
			d.logger.Warn("docker discovery: container missing required labels, skipping",
				"container", containerName,
				"id", c.ID[:12],
				"has_name", name != "",
				"has_endpoint", endpoint != "",
			)
			continue
		}

		var readOnlyTools []string
		if raw := strings.TrimSpace(c.Labels[readOnlyToolsLabel]); raw != "" {
			for _, t := range strings.Split(raw, ",") {
				if t = strings.TrimSpace(t); t != "" {
					readOnlyTools = append(readOnlyTools, t)
				}
			}
		}

		servers = append(servers, ServerConfig{
			Name:          name,
			Endpoint:      endpoint,
			ReadOnlyTools: readOnlyTools,
		})
	}

	return servers, nil
}

// RunDiscovery starts a background goroutine that periodically discovers MCP
// servers and reconciles Router connections. New servers are connected,
// removed servers are disconnected. The goroutine runs until ctx is cancelled.
func (r *Router) RunDiscovery(ctx context.Context, discoverer Discoverer, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reconcile(ctx, discoverer)
			}
		}
	}()
}

// reconcile compares discovered servers against current connections and
// connects/disconnects as needed.
func (r *Router) reconcile(ctx context.Context, discoverer Discoverer) {
	discovered, err := discoverer.Discover(ctx)
	if err != nil {
		r.logger.Warn("mcp discovery failed", "error", err)
		return
	}

	// Build set of discovered server names.
	want := make(map[string]ServerConfig, len(discovered))
	for _, s := range discovered {
		want[s.Name] = s
	}

	// Disconnect servers that are no longer discovered.
	current := r.ConnectedServers()
	for _, name := range current {
		if _, ok := want[name]; !ok {
			r.logger.Info("mcp discovery: disconnecting removed server", "server", name)
			if err := r.Disconnect(name); err != nil {
				r.logger.Warn("mcp discovery: disconnect error", "server", name, "error", err)
			}
		}
	}

	// Connect newly discovered servers and reconnect any whose config has
	// drifted (e.g. ReadOnlyTools allowlist changed via a container label edit).
	currentSet := make(map[string]bool, len(current))
	for _, name := range current {
		currentSet[name] = true
	}
	for name, cfg := range want {
		if !currentSet[name] {
			r.logger.Info("mcp discovery: connecting new server", "server", name, "endpoint", cfg.Endpoint)
			connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := r.Connect(connectCtx, cfg); err != nil {
				r.logger.Warn("mcp discovery: connect failed", "server", name, "error", err)
			}
			cancel()
			continue
		}
		// Already connected — detect config drift and reconnect if needed.
		existing, ok := r.ServerConfigOf(name)
		if ok && !serverConfigsEqual(existing, cfg) {
			r.logger.Info("mcp discovery: server config changed, reconnecting",
				"server", name)
			connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := r.Connect(connectCtx, cfg); err != nil {
				r.logger.Warn("mcp discovery: reconnect failed", "server", name, "error", err)
			}
			cancel()
		}
	}
}

// serverConfigsEqual reports whether two ServerConfig values are equivalent
// for the purpose of detecting whether a reconnect is required.
func serverConfigsEqual(a, b ServerConfig) bool {
	if a.Name != b.Name || a.Endpoint != b.Endpoint {
		return false
	}
	if !stringSlicesEqual(a.ReadOnlyTools, b.ReadOnlyTools) {
		return false
	}
	if len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, v := range a.Headers {
		if b.Headers[k] != v {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
