package mcpclient

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestStaticDiscoverer(t *testing.T) {
	servers := []ServerConfig{
		{Name: "harvest-mcp", Endpoint: "http://harvest:8082"},
		{Name: "ontap-mcp", Endpoint: "http://ontap:8084"},
	}
	d := &StaticDiscoverer{Servers: servers}

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, servers) {
		t.Errorf("got %v, want %v", got, servers)
	}
}

func TestDockerDiscoverer_Discover(t *testing.T) {
	containers := []dockerContainer{
		{
			ID:    "abc123def456",
			Names: []string{"/harvest-mcp"},
			Labels: map[string]string{
				"mcp.discover":   "true",
				"mcp.name":       "harvest-mcp",
				"mcp.endpoint":   "http://harvest-mcp:8082",
				"mcp.capability": "harvest",
			},
			State: "running",
		},
		{
			ID:    "def456ghi789",
			Names: []string{"/ontap-mcp"},
			Labels: map[string]string{
				"mcp.discover":   "true",
				"mcp.name":       "ontap-mcp",
				"mcp.endpoint":   "http://ontap-mcp:8084",
				"mcp.capability": "ontap",
			},
			State: "running",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.41/containers/json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(containers)
	}))
	defer srv.Close()

	// Create a discoverer that talks to our test server instead of Docker socket.
	d := &DockerDiscoverer{
		LabelPrefix: "mcp.",
		logger:      testLogger(t),
		client:      srv.Client(),
	}
	// Override the endpoint to point at our test server.
	d.client.Transport = &rewriteTransport{
		base:    d.client.Transport,
		baseURL: srv.URL,
	}

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []ServerConfig{
		{Name: "harvest-mcp", Endpoint: "http://harvest-mcp:8082"},
		{Name: "ontap-mcp", Endpoint: "http://ontap-mcp:8084"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDockerDiscoverer_SkipsMissingLabels(t *testing.T) {
	containers := []dockerContainer{
		{
			ID:    "abc123def456",
			Names: []string{"/good-mcp"},
			Labels: map[string]string{
				"mcp.discover": "true",
				"mcp.name":     "good-mcp",
				"mcp.endpoint": "http://good-mcp:8082",
			},
			State: "running",
		},
		{
			// Missing mcp.endpoint label
			ID:    "bad123bad456",
			Names: []string{"/bad-mcp"},
			Labels: map[string]string{
				"mcp.discover": "true",
				"mcp.name":     "bad-mcp",
			},
			State: "running",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(containers)
	}))
	defer srv.Close()

	d := &DockerDiscoverer{
		LabelPrefix: "mcp.",
		logger:      testLogger(t),
		client:      srv.Client(),
	}
	d.client.Transport = &rewriteTransport{base: d.client.Transport, baseURL: srv.URL}

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d servers, want 1", len(got))
	}
	if got[0].Name != "good-mcp" {
		t.Errorf("got name %q, want %q", got[0].Name, "good-mcp")
	}
}

func TestDockerDiscoverer_ErrorOnBadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Docker daemon error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := &DockerDiscoverer{
		LabelPrefix: "mcp.",
		logger:      testLogger(t),
		client:      srv.Client(),
	}
	d.client.Transport = &rewriteTransport{base: d.client.Transport, baseURL: srv.URL}

	_, err := d.Discover(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// TestDockerDiscoverer_ReadOnlyToolsLabel verifies the mcp.read_only_tools
// label is parsed (whitespace trimmed, empty entries skipped) into the
// resulting ServerConfig.ReadOnlyTools allowlist. Regression test for the
// bug where Docker discovery silently dropped the allowlist, causing
// read-only mode to filter out every tool from MCP servers that don't
// publish ToolAnnotations.ReadOnlyHint.
func TestDockerDiscoverer_ReadOnlyToolsLabel(t *testing.T) {
	containers := []dockerContainer{
		{
			ID:    "aaa111",
			Names: []string{"/with-allowlist"},
			Labels: map[string]string{
				"mcp.discover":        "true",
				"mcp.name":            "harvest-mcp",
				"mcp.endpoint":        "http://harvest-mcp:8082",
				"mcp.read_only_tools": "tool_a,tool_b, tool_c ",
			},
			State: "running",
		},
		{
			ID:    "bbb222",
			Names: []string{"/no-label"},
			Labels: map[string]string{
				"mcp.discover": "true",
				"mcp.name":     "ontap-mcp",
				"mcp.endpoint": "http://ontap-mcp:8084",
			},
			State: "running",
		},
		{
			ID:    "ccc333",
			Names: []string{"/empty-label"},
			Labels: map[string]string{
				"mcp.discover":        "true",
				"mcp.name":            "grafana-mcp",
				"mcp.endpoint":        "http://grafana-mcp:8086",
				"mcp.read_only_tools": "",
			},
			State: "running",
		},
		{
			ID:    "ddd444",
			Names: []string{"/whitespace-only"},
			Labels: map[string]string{
				"mcp.discover":        "true",
				"mcp.name":            "extra-mcp",
				"mcp.endpoint":        "http://extra-mcp:8088",
				"mcp.read_only_tools": " , ,, ",
			},
			State: "running",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(containers)
	}))
	defer srv.Close()

	d := &DockerDiscoverer{
		LabelPrefix: "mcp.",
		logger:      testLogger(t),
		client:      srv.Client(),
	}
	d.client.Transport = &rewriteTransport{base: d.client.Transport, baseURL: srv.URL}

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]ServerConfig, len(got))
	for _, s := range got {
		byName[s.Name] = s
	}

	t.Run("populated and trimmed", func(t *testing.T) {
		want := []string{"tool_a", "tool_b", "tool_c"}
		if !reflect.DeepEqual(byName["harvest-mcp"].ReadOnlyTools, want) {
			t.Errorf("got %v, want %v", byName["harvest-mcp"].ReadOnlyTools, want)
		}
	})

	t.Run("absent label is nil", func(t *testing.T) {
		if byName["ontap-mcp"].ReadOnlyTools != nil {
			t.Errorf("expected nil, got %v", byName["ontap-mcp"].ReadOnlyTools)
		}
	})

	t.Run("empty label is nil", func(t *testing.T) {
		if byName["grafana-mcp"].ReadOnlyTools != nil {
			t.Errorf("expected nil, got %v", byName["grafana-mcp"].ReadOnlyTools)
		}
	})

	t.Run("whitespace-only entries skipped", func(t *testing.T) {
		if byName["extra-mcp"].ReadOnlyTools != nil {
			t.Errorf("expected nil, got %v", byName["extra-mcp"].ReadOnlyTools)
		}
	})
}

// TestServerConfigsEqual covers the drift-detection helper used by
// reconcile() to decide whether to reconnect an existing server when its
// discovered ServerConfig has changed (e.g. a label edit changed the
// ReadOnlyTools allowlist).
func TestServerConfigsEqual(t *testing.T) {
	base := ServerConfig{
		Name:          "harvest-mcp",
		Endpoint:      "http://harvest-mcp:8082",
		ReadOnlyTools: []string{"a", "b"},
		Headers:       map[string]string{"X-Auth": "tok"},
	}

	cases := []struct {
		name string
		a, b ServerConfig
		want bool
	}{
		{"identical", base, base, true},
		{"endpoint changed", base, ServerConfig{
			Name: base.Name, Endpoint: "http://other:8082",
			ReadOnlyTools: base.ReadOnlyTools, Headers: base.Headers,
		}, false},
		{"read_only_tools added", base, ServerConfig{
			Name: base.Name, Endpoint: base.Endpoint,
			ReadOnlyTools: []string{"a", "b", "c"}, Headers: base.Headers,
		}, false},
		{"read_only_tools removed", base, ServerConfig{
			Name: base.Name, Endpoint: base.Endpoint,
			ReadOnlyTools: nil, Headers: base.Headers,
		}, false},
		{"read_only_tools reordered", base, ServerConfig{
			Name: base.Name, Endpoint: base.Endpoint,
			ReadOnlyTools: []string{"b", "a"}, Headers: base.Headers,
		}, false},
		{"header changed", base, ServerConfig{
			Name: base.Name, Endpoint: base.Endpoint,
			ReadOnlyTools: base.ReadOnlyTools,
			Headers:       map[string]string{"X-Auth": "other"},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serverConfigsEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("serverConfigsEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	router := NewRouter(testLogger(t))

	// Mock discoverer that returns changing server lists.
	disc := &mockDiscoverer{
		servers: []ServerConfig{
			{Name: "server-a", Endpoint: "http://a:8080"},
		},
	}

	// We can't actually connect, but we can verify the reconcile logic
	// doesn't panic with empty server list changes.
	router.reconcile(context.Background(), disc)

	// Verify no servers connected (endpoints don't exist).
	if got := len(router.ConnectedServers()); got != 0 {
		t.Errorf("expected 0 connected servers, got %d", got)
	}
}

// -- test helpers --

type mockDiscoverer struct {
	servers []ServerConfig
}

func (m *mockDiscoverer) Discover(_ context.Context) ([]ServerConfig, error) {
	return m.servers, nil
}

// rewriteTransport redirects Docker API requests to a test server.
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.Default()
}
