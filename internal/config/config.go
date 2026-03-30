package config

import (
	"fmt"
	"os"

	"github.com/ebarron/netapp-chat-service/internal/agent"
	"github.com/ebarron/netapp-chat-service/internal/capability"
	"github.com/ebarron/netapp-chat-service/internal/llm"
	"github.com/ebarron/netapp-chat-service/internal/mcpclient"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the chat service.
type Config struct {
	LLM          llm.ProviderConfig `yaml:"llm"`           // LLM provider settings.
	MCPServers   []MCPServer        `yaml:"mcp_servers"`   // MCP servers to connect to.
	Capabilities CapabilitiesConfig `yaml:"capabilities"`  // Capability definitions derived from MCP servers.
	Interests    InterestsConfig    `yaml:"interests"`     // Interest directories to load.
	Product      ProductConfig      `yaml:"product"`       // Product identity for the system prompt.
	Server       ServerConfig       `yaml:"server"`        // HTTP server settings.
}

// MCPServer defines an MCP server connection.
type MCPServer struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	Capability string `yaml:"capability"`
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

	return &cfg, nil
}

// ServerConfigs converts MCP server config to mcpclient.ServerConfig slice.
func (c *Config) ServerConfigs() []mcpclient.ServerConfig {
	configs := make([]mcpclient.ServerConfig, len(c.MCPServers))
	for i, s := range c.MCPServers {
		configs[i] = mcpclient.ServerConfig{
			Name:     s.Name,
			Endpoint: s.URL,
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
		caps = append(caps, capability.Capability{
			ID:         s.Capability,
			Name:       s.Capability,
			State:      state,
			ServerName: s.Name,
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
