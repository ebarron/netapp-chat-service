package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the chat service.
type Config struct {
	LLM          llm.ProviderConfig `yaml:"llm"`           // LLM provider settings.
	MCPServers   []MCPServer        `yaml:"mcp_servers"`   // MCP servers to connect to.
	MCPDiscovery MCPDiscoveryConfig `yaml:"mcp_discovery"` // Dynamic MCP server discovery.
	Capabilities CapabilitiesConfig `yaml:"capabilities"`  // Capability definitions derived from MCP servers.
	Interests    InterestsConfig    `yaml:"interests"`     // Interest directories to load.
	Product      ProductConfig      `yaml:"product"`       // Product identity for the system prompt.
	Server       ServerConfig       `yaml:"server"`        // HTTP server settings.
	UI           UIConfig           `yaml:"ui"`            // Built-in chat UI settings.
	ToolRouting  ToolRoutingConfig  `yaml:"tool_routing"`  // High-tool-count scaling supervisor (S7).
}

// MCPServer defines an MCP server connection.
type MCPServer struct {
	Name       string            `yaml:"name"`
	URL        string            `yaml:"url"`
	Capability string            `yaml:"capability"`
	Headers    map[string]string `yaml:"headers"` // extra HTTP headers (e.g. auth tokens)
	// ReadOnlyTools is an allowlist of tool names treated as read-only when
	// the server's tools lack proper MCP annotations. Used for filtering in
	// read-only mode.
	ReadOnlyTools []string `yaml:"read_only_tools"`
	// ForwardHeaders is an allowlist of inbound HTTP header names to relay from
	// the current chat request onto outbound requests to this server. Values
	// are opaque; the host application defines and interprets them.
	ForwardHeaders []string `yaml:"forward_headers"`
	// CapabilityName is an optional human-readable label for this server's
	// capability/group, shown in the tool-routing group menu (S7a). When
	// empty the capability ID is used. Operator config only — never derived
	// from product/view semantics.
	CapabilityName string `yaml:"capability_name"`
	// CapabilityDescription is an optional one-line description of what this
	// server's tools cover, shown in the tool-routing group menu to help the
	// model route. When empty the menu auto-derives a description from the
	// server's tool names. Operator config only.
	CapabilityDescription string `yaml:"capability_description"`
}

// ToolRoutingConfig controls the high-tool-count scaling supervisor (S7).
// See docs/high-tool-count-scaling.md. The zero value (mode unset) is
// equivalent to mode "off" and reproduces today's behavior exactly.
type ToolRoutingConfig struct {
	// Mode is the routing mode: "off" (default), "in-band" (S7a), or
	// "router" (S7b, not yet implemented).
	Mode string `yaml:"mode"`
	// MaxTools optionally caps the post-routing tool list. 0 means no cap
	// beyond agent.MaxToolsPerRequest.
	MaxTools int `yaml:"max_tools"`
	// AlwaysOn lists capability/group IDs that are loaded from the first turn
	// without the model having to call load_tools.
	AlwaysOn []string `yaml:"always_on"`
}

// normalize fills defaults and validates the tool-routing config. An empty
// mode becomes "off". An unrecognized mode is a config error. "router" is
// accepted here (it is a legal mode) but rejected later at startup wiring
// until S7b is implemented.
func (t *ToolRoutingConfig) normalize() error {
	if t.Mode == "" {
		t.Mode = agent.ToolRoutingOff
	}
	if !agent.ValidToolRoutingMode(t.Mode) {
		return fmt.Errorf("tool_routing.mode %q is invalid (want %q, %q, or %q)",
			t.Mode, agent.ToolRoutingOff, agent.ToolRoutingInBand, agent.ToolRoutingRouter)
	}
	if t.MaxTools < 0 {
		return fmt.Errorf("tool_routing.max_tools %d is invalid (must be >= 0)", t.MaxTools)
	}
	return nil
}

// CapabilitiesConfig defines capability defaults.
type CapabilitiesConfig struct {
	Defaults map[string]string `yaml:"defaults"` // capability ID → "off"/"ask"/"allow"
}

// InterestsConfig defines interest loading paths.
type InterestsConfig struct {
	Dirs []string `yaml:"dirs"` // directories to load interests from
}

// ProductConfig defines the product identity for the system prompt.
type ProductConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Guidelines  []string `yaml:"guidelines"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Addr string `yaml:"addr"` // listen address (default ":8090")
}

// UIConfig controls the built-in chat UI shell.
type UIConfig struct {
	Enabled bool `yaml:"enabled"` // serve the embedded chat UI at /
}

// MCPDiscoveryConfig controls dynamic MCP server discovery.
type MCPDiscoveryConfig struct {
	// Mode is the discovery mode: "static" (default) uses mcp_servers list,
	// "docker" discovers servers from Docker container labels.
	Mode string `yaml:"mode"`
	// PollInterval is how often to poll for server changes (default 30s).
	// Only used when mode is "docker".
	PollInterval time.Duration `yaml:"poll_interval"`
	// SocketPath is the Docker socket path (default /var/run/docker.sock).
	SocketPath string `yaml:"socket_path"`
	// LabelPrefix is the label prefix for discovery (default "mcp.").
	LabelPrefix string `yaml:"label_prefix"`
}

// Discoverer creates a Discoverer from the config. For static mode (or when
// mode is empty), it returns a StaticDiscoverer wrapping MCPServers. For
// docker mode, it returns a DockerDiscoverer.
func (c *Config) Discoverer(logger *slog.Logger) mcpclient.Discoverer {
	switch c.MCPDiscovery.Mode {
	case "docker":
		var opts []mcpclient.DockerOption
		if c.MCPDiscovery.SocketPath != "" {
			opts = append(opts, mcpclient.WithSocketPath(c.MCPDiscovery.SocketPath))
		}
		if c.MCPDiscovery.LabelPrefix != "" {
			opts = append(opts, mcpclient.WithLabelPrefix(c.MCPDiscovery.LabelPrefix))
		}
		return mcpclient.NewDockerDiscoverer(logger, opts...)
	default:
		return &mcpclient.StaticDiscoverer{Servers: c.ServerConfigs()}
	}
}

// DiscoveryInterval returns the poll interval for discovery, with a default.
func (c *Config) DiscoveryInterval() time.Duration {
	if c.MCPDiscovery.PollInterval > 0 {
		return c.MCPDiscovery.PollInterval
	}
	return 30 * time.Second
}

// Load reads and parses a config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8090"
	}

	if err := cfg.ToolRouting.normalize(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// ServerConfigs converts MCP server config to mcpclient.ServerConfig slice.
func (c *Config) ServerConfigs() []mcpclient.ServerConfig {
	configs := make([]mcpclient.ServerConfig, len(c.MCPServers))
	for i, s := range c.MCPServers {
		configs[i] = mcpclient.ServerConfig{
			Name:           s.Name,
			Endpoint:       s.URL,
			Headers:        s.Headers,
			ReadOnlyTools:  s.ReadOnlyTools,
			ForwardHeaders: s.ForwardHeaders,
		}
	}
	return configs
}

// BuildCapabilities constructs capability definitions from MCP server config.
func (c *Config) BuildCapabilities() []capability.Capability {
	var caps []capability.Capability
	for _, s := range c.MCPServers {
		if s.Capability == "" {
			continue
		}
		state := capability.StateAsk
		if def, ok := c.Capabilities.Defaults[s.Capability]; ok {
			if st := capability.State(def); st.Valid() {
				state = st
			}
		}
		name := s.CapabilityName
		if name == "" {
			name = s.Capability
		}
		caps = append(caps, capability.Capability{
			ID:          s.Capability,
			Name:        name,
			Description: s.CapabilityDescription,
			State:       state,
			ServerName:  s.Name,
		})
	}
	return caps
}

// PromptConfig converts the product config to an agent.SystemPromptConfig.
func (c *Config) PromptConfig() agent.SystemPromptConfig {
	return agent.SystemPromptConfig{
		ProductName:        c.Product.Name,
		ProductDescription: c.Product.Description,
		Guidelines:         c.Product.Guidelines,
	}
}
