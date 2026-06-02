package mcpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// roundTrip is a tiny helper that drives headerRoundTripper against a stub
// server and returns the headers the server actually received.
func roundTrip(t *testing.T, rt *headerRoundTripper, ctx context.Context, url string) http.Header {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	resp.Body.Close()
	return req.Header
}

func TestRoundTripForwardsAllowlistedHeaderFromContext(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	rt := &headerRoundTripper{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-Static": "s"},
		forward: []string{"X-User-Token"},
	}
	ctx := WithForwardedHeaders(context.Background(), map[string]string{
		http.CanonicalHeaderKey("X-User-Token"): "user-abc",
	})

	roundTrip(t, rt, ctx, srv.URL)

	if got.Get("X-User-Token") != "user-abc" {
		t.Errorf("forwarded header = %q, want %q", got.Get("X-User-Token"), "user-abc")
	}
	if got.Get("X-Static") != "s" {
		t.Errorf("static header = %q, want %q", got.Get("X-Static"), "s")
	}
}

func TestRoundTripForwardedValueWinsOverStatic(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	rt := &headerRoundTripper{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-User-Token": "static-default"},
		forward: []string{"X-User-Token"},
	}
	ctx := WithForwardedHeaders(context.Background(), map[string]string{
		http.CanonicalHeaderKey("X-User-Token"): "per-request",
	})

	roundTrip(t, rt, ctx, srv.URL)

	if got.Get("X-User-Token") != "per-request" {
		t.Errorf("header = %q, want per-request value to win", got.Get("X-User-Token"))
	}
}

func TestRoundTripIgnoresNonAllowlistedHeader(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	rt := &headerRoundTripper{
		base:    http.DefaultTransport,
		forward: []string{"X-User-Token"}, // does not allowlist X-Secret
	}
	ctx := WithForwardedHeaders(context.Background(), map[string]string{
		http.CanonicalHeaderKey("X-Secret"): "leak",
	})

	roundTrip(t, rt, ctx, srv.URL)

	if got.Get("X-Secret") != "" {
		t.Errorf("non-allowlisted header leaked: %q", got.Get("X-Secret"))
	}
}

func TestRoundTripNoForwardConfigIsParityWithStatic(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	rt := &headerRoundTripper{
		base:    http.DefaultTransport,
		headers: map[string]string{"Authorization": "Bearer x"},
	}
	// Context carries a value, but no forward allowlist → must be ignored.
	ctx := WithForwardedHeaders(context.Background(), map[string]string{
		http.CanonicalHeaderKey("X-User-Token"): "ignored",
	})

	roundTrip(t, rt, ctx, srv.URL)

	if got.Get("Authorization") != "Bearer x" {
		t.Errorf("static auth = %q, want Bearer x", got.Get("Authorization"))
	}
	if got.Get("X-User-Token") != "" {
		t.Errorf("header forwarded without allowlist: %q", got.Get("X-User-Token"))
	}
}

func TestCollectForwardableHeadersUnionAndCanonicalization(t *testing.T) {
	r := NewRouter(nil)
	// Two servers, overlapping + distinct allowlists.
	r.servers["a"] = &serverConn{cfg: ServerConfig{Name: "a", ForwardHeaders: []string{"X-User-Token"}}}
	r.servers["b"] = &serverConn{cfg: ServerConfig{Name: "b", ForwardHeaders: []string{"x-user-token", "X-Trace"}}}

	in := http.Header{}
	in.Set("x-user-token", "abc")
	in.Set("X-Trace", "t1")
	in.Set("X-Other", "nope")

	got := r.CollectForwardableHeaders(in)

	if got[http.CanonicalHeaderKey("X-User-Token")] != "abc" {
		t.Errorf("X-User-Token = %q, want abc", got[http.CanonicalHeaderKey("X-User-Token")])
	}
	if got[http.CanonicalHeaderKey("X-Trace")] != "t1" {
		t.Errorf("X-Trace = %q, want t1", got[http.CanonicalHeaderKey("X-Trace")])
	}
	if _, ok := got["X-Other"]; ok {
		t.Errorf("X-Other should not be collected")
	}
}

func TestCollectForwardableHeadersEmptyWhenNoServerOptsIn(t *testing.T) {
	r := NewRouter(nil)
	r.servers["a"] = &serverConn{cfg: ServerConfig{Name: "a"}} // no ForwardHeaders

	in := http.Header{}
	in.Set("X-User-Token", "abc")

	if got := r.CollectForwardableHeaders(in); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
