package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"gopkg.in/yaml.v3"
)

// TestServerConfigsPropagatesReadOnlyTools exercises the read_only_tools
// allowlist plumbing from config.yaml all the way to mcpclient.ServerConfig.
// Without this propagation the read-only filter would drop tools from MCPs
// that don't publish ToolAnnotations.ReadOnlyHint (e.g. Grafana).
func TestServerConfigsPropagatesReadOnlyTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
llm:
  provider: openai
  model: gpt-4
mcp_servers:
  - name: grafana-mcp
    url: http://grafana:8086
    capability: grafana
    read_only_tools:
      - list_dashboards
      - get_panel_data
  - name: ontap-mcp
    url: http://ontap:8084
    capability: ontap
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	got := cfg.ServerConfigs()
	if len(got) != 2 {
		t.Fatalf("got %d configs, want 2", len(got))
	}

	wantGrafana := []string{"list_dashboards", "get_panel_data"}
	if !reflect.DeepEqual(got[0].ReadOnlyTools, wantGrafana) {
		t.Errorf("grafana ReadOnlyTools = %v, want %v", got[0].ReadOnlyTools, wantGrafana)
	}
	if len(got[1].ReadOnlyTools) != 0 {
		t.Errorf("ontap ReadOnlyTools = %v, want empty", got[1].ReadOnlyTools)
	}
}

// writeConfig writes a minimal config with the given tool_routing block and
// loads it, returning the parsed config or the load error.
func loadConfigYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlDoc := "llm:\n  provider: openai\n  model: gpt-4\n" + body
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

// TestToolRoutingDefaultOff verifies that omitting tool_routing yields mode
// "off" (today's behavior) with no error — the backward-compat guarantee.
func TestToolRoutingDefaultOff(t *testing.T) {
	cfg, err := loadConfigYAML(t, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ToolRouting.Mode != agent.ToolRoutingOff {
		t.Errorf("default mode = %q, want %q", cfg.ToolRouting.Mode, agent.ToolRoutingOff)
	}
	if cfg.ToolRouting.MaxTools != 0 {
		t.Errorf("default max_tools = %d, want 0", cfg.ToolRouting.MaxTools)
	}
	if len(cfg.ToolRouting.AlwaysOn) != 0 {
		t.Errorf("default always_on = %v, want empty", cfg.ToolRouting.AlwaysOn)
	}
}

// TestToolRoutingValidModes verifies each legal mode parses without error,
// including "router" (a legal mode that is rejected only at startup wiring).
func TestToolRoutingValidModes(t *testing.T) {
	for _, mode := range []string{"off", "in-band", "router"} {
		t.Run(mode, func(t *testing.T) {
			cfg, err := loadConfigYAML(t, "tool_routing:\n  mode: "+mode+"\n")
			if err != nil {
				t.Fatalf("Load(mode=%q) error = %v", mode, err)
			}
			if cfg.ToolRouting.Mode != mode {
				t.Errorf("mode = %q, want %q", cfg.ToolRouting.Mode, mode)
			}
		})
	}
}

// TestToolRoutingInvalidMode verifies an unrecognized mode is a clear config
// error.
func TestToolRoutingInvalidMode(t *testing.T) {
	_, err := loadConfigYAML(t, "tool_routing:\n  mode: bogus\n")
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "tool_routing.mode") {
		t.Errorf("error = %q, want it to mention tool_routing.mode", err)
	}
}

// TestToolRoutingNegativeMaxTools verifies a negative max_tools is rejected.
func TestToolRoutingNegativeMaxTools(t *testing.T) {
	_, err := loadConfigYAML(t, "tool_routing:\n  mode: in-band\n  max_tools: -1\n")
	if err == nil {
		t.Fatal("expected error for negative max_tools, got nil")
	}
}

// TestToolRoutingFields verifies max_tools and always_on parse correctly.
func TestToolRoutingFields(t *testing.T) {
	cfg, err := loadConfigYAML(t, "tool_routing:\n  mode: in-band\n  max_tools: 40\n  always_on:\n    - core\n    - search\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ToolRouting.MaxTools != 40 {
		t.Errorf("max_tools = %d, want 40", cfg.ToolRouting.MaxTools)
	}
	want := []string{"core", "search"}
	if !reflect.DeepEqual(cfg.ToolRouting.AlwaysOn, want) {
		t.Errorf("always_on = %v, want %v", cfg.ToolRouting.AlwaysOn, want)
	}
}

// TestToolRoutingGroupExpandThreshold verifies the S8 group_expand_threshold
// field parses, defaults to 0, and rejects negative values.
func TestToolRoutingGroupExpandThreshold(t *testing.T) {
	cfg, err := loadConfigYAML(t, "tool_routing:\n  mode: in-band\n  group_expand_threshold: 25\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ToolRouting.GroupExpandThreshold != 25 {
		t.Errorf("group_expand_threshold = %d, want 25", cfg.ToolRouting.GroupExpandThreshold)
	}

	def, err := loadConfigYAML(t, "tool_routing:\n  mode: in-band\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if def.ToolRouting.GroupExpandThreshold != 0 {
		t.Errorf("default group_expand_threshold = %d, want 0", def.ToolRouting.GroupExpandThreshold)
	}

	if _, err := loadConfigYAML(t, "tool_routing:\n  mode: in-band\n  group_expand_threshold: -1\n"); err == nil {
		t.Fatal("expected error for negative group_expand_threshold, got nil")
	}
}

// TestToolRoutingRoundTrip verifies marshal/unmarshal stability of the
// tool_routing block.
func TestToolRoutingRoundTrip(t *testing.T) {
	orig := ToolRoutingConfig{Mode: "in-band", MaxTools: 64, AlwaysOn: []string{"a", "b"}, GroupExpandThreshold: 25}
	data, err := yaml.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ToolRoutingConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

// TestBuildCapabilitiesNameDescription verifies the optional capability_name
// and capability_description fields flow into the capability set, falling back
// to the capability ID for the name when unset.
func TestBuildCapabilitiesNameDescription(t *testing.T) {
	cfg, err := loadConfigYAML(t, "mcp_servers:\n"+
		"  - name: jira-mcp\n    url: http://jira:8080\n    capability: jira\n"+
		"    capability_name: Jira\n    capability_description: Issue tracking and backlog\n"+
		"  - name: zoom-mcp\n    url: http://zoom:8080\n    capability: zoom\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	caps := cfg.BuildCapabilities()
	byID := map[string]struct{ name, desc string }{}
	for _, c := range caps {
		byID[c.ID] = struct{ name, desc string }{c.Name, c.Description}
	}
	if byID["jira"].name != "Jira" || byID["jira"].desc != "Issue tracking and backlog" {
		t.Errorf("jira cap = %+v, want name=Jira desc set", byID["jira"])
	}
	if byID["zoom"].name != "zoom" {
		t.Errorf("zoom cap name = %q, want fallback to ID %q", byID["zoom"].name, "zoom")
	}
	if byID["zoom"].desc != "" {
		t.Errorf("zoom cap desc = %q, want empty", byID["zoom"].desc)
	}
}
