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
