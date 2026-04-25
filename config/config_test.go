package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
