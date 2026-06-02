# Per-Request MCP Header Forwarding

> **Status:** Implemented (v0.1.15)
> **Audience:** Engineers working on netapp-chat-service
> **Scope:** A generic mechanism for propagating named inbound HTTP headers
> from a `/chat/*` request onto the outbound HTTP requests the service makes
> to its MCP servers. No host-application semantics live in this service.

---

## 1. Overview

Today the chat-service attaches a **static**, per-server set of HTTP headers to
every MCP request. They are configured once (`mcp_servers[].headers`) and
injected by `headerRoundTripper` at connect time. This is sufficient when the
MCP servers authenticate the *service* (one shared identity for all chat
users).

It is **not** sufficient when the MCP servers — or a proxy in front of them —
need to authorize **the end user who issued the current chat turn**. Because
the chat-service multiplexes every user over MCP sessions opened at startup
with one static header set, all tool calls present the same identity
regardless of who is chatting.

This document specifies a **generic** capability: *forward a configured list of
inbound request headers onto the outbound MCP request for the duration of that
chat turn.* The service treats the header **name** as configuration and the
header **value** as an opaque string. It assigns no meaning to the value, does
not parse or verify it, and has no knowledge of the host application's identity
model. A host that wants per-user authorization mints whatever token it likes,
sends it as a request header to `/chat/message`, and configures the
chat-service to relay it to the relevant MCP servers. The host's MCP endpoint
(or its proxy) is responsible for interpreting it.

This is the MCP analogue of standard reverse-proxy header propagation
(`Authorization`, `X-Forwarded-*`, `traceparent`): the service is a conduit,
not an interpreter.

---

## 2. Goals & non-goals

### Goals

- Forward a **configurable allowlist** of inbound headers from a `/chat/*`
  request onto the outbound MCP HTTP requests made while serving that request.
- Per-server opt-in: a given MCP server only receives the headers its config
  names — no accidental fan-out of a caller token to unrelated servers.
- Per-request value: the forwarded value is taken from the **current** inbound
  request, not from static config, and never leaks across requests or users.
- Zero domain knowledge: header names are config; values are opaque; no
  signing, parsing, identity, or host-application concepts enter this repo.
- Backward compatible: absent configuration, behavior is byte-for-byte
  identical to today.

### Non-goals

- Minting, signing, or verifying tokens (the host application does this).
- Per-user MCP **sessions** or connection pooling. We keep the existing shared,
  startup-established sessions and vary only the per-request header on the
  outbound call. (See §7 for why this is safe with the current SDK.)
- Any change to capability gating, tool discovery, or the agent loop.

---

## 3. Design

### 3.1 Request flow

```
inbound  POST /chat/message
   │      headers: { X-User-Token: <opaque>, … }
   ▼
server.PostChatMessage
   │   capture configured forwardable headers from r.Header
   │   ctx = WithForwardedHeaders(r.Context(), captured)
   ▼
RunChat(ctx, …) ─► agent loop ─► Router.CallTool(ctx, tc)
   │
   ▼
mcp StreamableClientTransport issues HTTP POST carrying ctx
   │
   ▼
headerRoundTripper.RoundTrip(req)
   │   static cfg.Headers           (unchanged)
   │   + forwarded headers from req.Context(), filtered by this
   │     server's forward_headers allowlist   (new; per-request wins)
   ▼
outbound MCP request to the server / proxy
```

The inbound request's `context.Context` already flows unbroken from
`PostChatMessage` → `RunChat` → `Router.CallTool(ctx, …)`, and the MCP Go SDK
attaches that `ctx` to the HTTP request it issues. So a value placed in the
context at the HTTP boundary is readable inside `RoundTrip` via
`req.Context()`. No new plumbing through the agent loop is required.

### 3.2 Configuration

Add an optional `forward_headers` list to each MCP server. It is an allowlist
of inbound header **names** to relay to that server.

```yaml
mcp_servers:
  - name: zoom-mcp
    url: http://127.0.0.1:3001/api/mcp/zoom/streamable
    capability: zoom
    forward_headers:
      - X-User-Token        # opaque; defined and interpreted by the host
  - name: ontap-mcp
    url: http://localhost:8083
    capability: ontap
    # no forward_headers → behaves exactly as today
```

Notes:

- Matching is case-insensitive (HTTP header semantics).
- If the inbound request does not carry a listed header, nothing is added for
  it — the static `headers` (if any) still apply.
- A forwarded header **overrides** a same-named static `headers` entry for that
  request only.

### 3.3 Context carrier

A typed, unexported context key in `mcpclient` carrying an immutable
`map[string]string` (canonicalized header name → value):

```go
// mcpclient/forward.go
type forwardedHeadersKey struct{}

// WithForwardedHeaders returns a child ctx carrying headers to be relayed to
// MCP servers that opt in via ServerConfig.ForwardHeaders. Values are opaque.
func WithForwardedHeaders(ctx context.Context, h map[string]string) context.Context

func forwardedHeadersFrom(ctx context.Context) map[string]string
```

### 3.4 RoundTripper change

`headerRoundTripper` gains the per-server allowlist and reads per-request values
from the request context:

```go
type headerRoundTripper struct {
    base    http.RoundTripper
    headers map[string]string // static, from cfg.Headers (unchanged)
    forward []string          // allowlist of inbound header names (new)
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    for k, v := range h.headers {
        req.Header.Set(k, v)
    }
    if len(h.forward) > 0 {
        if fwd := forwardedHeadersFrom(req.Context()); len(fwd) > 0 {
            for _, name := range h.forward {
                if v, ok := fwd[http.CanonicalHeaderKey(name)]; ok {
                    req.Header.Set(name, v) // per-request value wins
                }
            }
        }
    }
    return h.base.RoundTrip(req)
}
```

Because a `forward_headers` server now needs a custom transport even when
`cfg.Headers` is empty, `Connect` builds the custom `http.Client` when
`len(cfg.Headers) > 0 || len(cfg.ForwardHeaders) > 0`.

### 3.5 Server boundary capture

In `server.PostChatMessage`, before `RunChat`, capture the union of all header
names any connected server is willing to forward, then attach them:

```go
fwd := s.deps.Router.CollectForwardableHeaders(r.Header) // map[canonical]value
ctx = mcpclient.WithForwardedHeaders(ctx, fwd)
```

`Router.CollectForwardableHeaders` computes (once, cached) the set-union of all
servers' `ForwardHeaders` and returns only those present on the inbound
request. This keeps the server package free of per-server knowledge and ensures
we never stash a header no server asked for.

The same capture is applied to any other handler that reaches the router on a
user's behalf (e.g. `/chat/approve` resuming a tool call) so resumed calls
carry the originating identity.

---

## 4. Changes by file

| File | Change |
|---|---|
| `config/config.go` | Add `ForwardHeaders []string \`yaml:"forward_headers"\`` to `MCPServer`; map it through to `mcpclient.ServerConfig`. |
| `mcpclient/router.go` | Add `ForwardHeaders []string` to `ServerConfig`; extend `headerRoundTripper`; build a custom client when forwarding is configured; add `CollectForwardableHeaders`. |
| `mcpclient/forward.go` (new) | Context key + `WithForwardedHeaders` / `forwardedHeadersFrom`. |
| `server/server.go` | Capture forwardable headers in `PostChatMessage` (and other router-invoking handlers) and attach to ctx before `RunChat`. |
| `config.example.yaml` | Document `forward_headers` with a generic example + comment. |
| `docs/this file` | This design. |
| `CHANGELOG.md` | Note the new generic capability. |

---

## 5. Security considerations

- **Allowlist only.** Only explicitly configured header names are ever relayed,
  and only to the servers that name them. There is no wildcard and no implicit
  forwarding. A caller cannot cause an arbitrary header to be relayed.
- **No interpretation.** The service never parses, logs the value of, or makes
  decisions based on a forwarded header. Treat values as secrets-in-transit.
- **No value logging.** Do not log forwarded header values. If logging the act
  of forwarding for diagnostics, log the header **name** and a boolean
  presence, never the value.
- **Trust boundary is the deployment's.** Forwarding is only meaningful when the
  chat-service trusts its inbound caller (typically a host application reverse
  proxy on loopback) and the MCP endpoint trusts the relayed value. This is the
  operator's responsibility, identical to standard proxy header trust.
- **No cross-request leakage.** Values live solely in the per-request context
  and are filtered per server; nothing is stored on the shared session or the
  static client.

---

## 6. Backward compatibility

With no `forward_headers` configured anywhere, `CollectForwardableHeaders`
returns an empty map, the context carrier is empty, and `RoundTrip` does
exactly what it does today. No existing deployment changes behavior.

---

## 7. Why per-request headers work with shared sessions

MCP sessions (and their `http.Client`) are created once in `Router.Connect`.
The streamable HTTP transport, however, issues a **separate HTTP request per
tool call**, and the MCP Go SDK attaches the call's `context.Context` to that
request. The forwarded value is therefore resolved at `RoundTrip` time from the
*current* request's context — not baked into the session — so a single shared
session safely serves different forwarded values on consecutive calls. No
per-user session or pool is needed.

The long-lived server→client SSE GET stream is established at connect time and
does not carry per-call identity; only the per-call POSTs do, which is exactly
where authorization is needed.

---

## 8. Test plan

- **Unit (`mcpclient`):**
  - `RoundTrip` relays a forwarded header when the server allowlists it; value
    from context wins over a same-named static header.
  - `RoundTrip` ignores forwarded headers a server does not allowlist.
  - Empty context / empty allowlist → no added headers (parity with today).
  - `CollectForwardableHeaders` returns only allowlisted headers present on the
    request, canonicalized, union across servers.
- **Unit (`config`):** `forward_headers` parses and maps into
  `mcpclient.ServerConfig`.
- **Integration (`server`):** two requests carrying different
  `X-User-Token` values route to distinct outbound header values against a stub
  MCP server; a request with no token adds nothing.

---

## 9. Example configuration

```yaml
mcp_servers:
  # Server fronted by a host proxy that authorizes per end user.
  - name: zoom-mcp
    url: http://127.0.0.1:3001/api/mcp/zoom/streamable
    capability: zoom
    forward_headers: [X-User-Token]

  # Server authenticated by a static service token (unchanged pattern).
  - name: ontap-mcp
    url: http://localhost:8083
    capability: ontap
    headers:
      Authorization: Bearer $ONTAP_TOKEN
```

The header name `X-User-Token` is illustrative. Any name works; the
chat-service neither defines nor understands it.
