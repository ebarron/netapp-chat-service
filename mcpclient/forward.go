package mcpclient

import (
	"context"
	"net/http"
)

// forwardedHeadersKey is the unexported context key under which per-request
// forwardable headers are carried from the HTTP boundary to the RoundTripper.
type forwardedHeadersKey struct{}

// WithForwardedHeaders returns a child context carrying headers to be relayed
// onto outbound MCP requests for servers that opt in via
// ServerConfig.ForwardHeaders. Keys must be canonicalized
// (http.CanonicalHeaderKey). Values are treated as opaque strings — this
// package assigns them no meaning, never parses them, and never logs them.
//
// If h is empty, the parent context is returned unchanged.
func WithForwardedHeaders(ctx context.Context, h map[string]string) context.Context {
	if len(h) == 0 {
		return ctx
	}
	return context.WithValue(ctx, forwardedHeadersKey{}, h)
}

// forwardedHeadersFrom returns the forwardable headers carried by ctx, or nil.
func forwardedHeadersFrom(ctx context.Context) map[string]string {
	h, _ := ctx.Value(forwardedHeadersKey{}).(map[string]string)
	return h
}

// collectForwardable returns the subset of src whose canonicalized names are in
// the allow set, keyed by canonical header name. Returns nil if nothing matches.
func collectForwardable(src http.Header, allow map[string]struct{}) map[string]string {
	if len(src) == 0 || len(allow) == 0 {
		return nil
	}
	var out map[string]string
	for name := range allow {
		if v := src.Get(name); v != "" {
			if out == nil {
				out = make(map[string]string, len(allow))
			}
			out[http.CanonicalHeaderKey(name)] = v
		}
	}
	return out
}
