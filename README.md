# netapp-chat-service

A standalone, host-agnostic chat service that provides an agentic tool-use loop
powered by LLMs and MCP (Model Context Protocol) tool servers. It ships no
product-specific behavior and is not aware of the application that embeds it — a
host application injects the LLM provider, MCP servers, capabilities, system-prompt
text, interests, and any custom render tools via config (standalone) or
`server.ChatDeps` (embedded library).

## Features

- **Multi-provider LLM support** — OpenAI, Anthropic, AWS Bedrock, and any
  OpenAI-compatible custom/proxy endpoint, all behind a single streaming `Provider`
  interface.
- **MCP tool routing** — connect to multiple MCP servers (static list or dynamic
  Docker discovery); each server maps 1:1 to a capability.
- **Capability gating** — per-capability `off` / `ask` / `allow` states; ask-mode
  tools pause for explicit user approval before executing.
- **Read-only / read-write modes** — write-capable tools are filtered out in
  read-only mode (using MCP `ReadOnlyHint` annotations or a per-server
  `read_only_tools` allowlist).
- **High tool-count scaling (tool routing supervisor)** — optional in-band
  supervisor keeps the per-turn tool list under the provider's 128-tool cap
  as MCP servers grow: the model self-selects capability groups via an internal
  `load_tools` tool (no extra LLM round trip). Oversized servers can be offered
  tool-by-tool (`group_expand_threshold`). Off by default and byte-for-byte
  identical to legacy behavior when unset. See
  [docs/high-tool-count-scaling.md](docs/high-tool-count-scaling.md).
- **Interest system** — host-supplied, pattern-matched response templates that scope
  tools to relevant capabilities and teach the LLM to produce rich layouts. Users
  can create/update/delete their own interests via chat.
- **Declarative rendering** — the LLM emits typed JSON in fenced code blocks
  (`chart`, `dashboard`, `object-detail`) that the UI renders as React components;
  no executable code crosses the wire.
- **Bespoke render tools** — hosts can register Go `InternalTool`s (built on the
  `render/` toolkit) that deterministically build `object-detail` output instead of
  relying on the LLM to assemble JSON.
- **Canvas rendering** — `canvas-object-detail` / `canvas-dashboard` fences are
  converted to `canvas_open` SSE events so content opens in a pinned side panel.
- **SSE streaming** — real-time token/tool-call/tool-result streaming for chat
  responses.
- **Embeddable** — run as a standalone binary or embed as a Go library
  (`server.New(deps).Handler()`); optionally serve a built-in chat UI.

## Quick Start (standalone)

```bash
# Build
go build -o chat-service ./cmd/chat-service

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your LLM provider and MCP server details

# Run
./chat-service -config config.yaml
```

By default the service listens on `:8090` and exposes the JSON/SSE API only. The
chat UI is a separate React package (`@edjbarron/netapp-chat-component`) that a host
application embeds; the standalone binary does not bundle a UI.

## Embedding as a Go library

Hosts that embed the service construct `server.ChatDeps` directly instead of loading
a config file, then mount the returned `http.Handler`:

```go
import "github.com/ebarron/netapp-chat-service/server"

srv := server.New(&server.ChatDeps{
    Sessions:     sessions,     // *session.Manager
    Provider:     provider,     // llm.Provider
    Router:       router,       // mcpclient.ToolRouter
    Capabilities: capabilities, // []capability.Capability
    Catalog:      catalog,      // *interest.Catalog (optional)
    ExtraTools:   extraTools,   // map[string]agent.InternalTool — bespoke render tools, etc.
    PromptConfig: promptConfig, // agent.SystemPromptConfig (ProductName/Description/Guidelines/Vocabulary)
    Logger:       logger,

    // Optional tool-routing supervisor (`group_expand_threshold`); zero value == off.
    ToolRoutingMode:            "in-band",
    ToolRoutingExpandThreshold: 40,
})

// srv.ServeUI(myUIDistFS) // optional: serve a prebuilt UI dist the host embeds
mux.Handle("/", srv.Handler())
```

## Configuration

See [config.example.yaml](config.example.yaml) for a complete, commented example.
Environment variables are expanded in the config file (e.g. `$ANTHROPIC_API_KEY`).

Top-level config blocks:

| Block | Purpose |
|-------|---------|
| `llm` | Provider, endpoint, API key, model |
| `mcp_servers` | MCP servers to connect to (name, url, capability, optional labels/headers/`read_only_tools`/`forward_headers`) |
| `mcp_discovery` | Dynamic MCP discovery (`static` default, or `docker`) |
| `capabilities.defaults` | Initial `off`/`ask`/`allow` state per capability |
| `interests.dirs` | Directories of interest `.md` files to load |
| `product` | System-prompt identity: `name`, `description`, `guidelines` |
| `server.addr` | Listen address (default `:8090`) |
| `ui.enabled` | Hint for hosts that bundle a UI dist via `Server.ServeUI` (not bundled by the standalone binary) |
| `tool_routing` | High-tool-count supervisor: `mode`, `max_tools`, `always_on`, `group_expand_threshold` |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/chat/message` | Send a message, receive an SSE event stream |
| DELETE | `/chat/session` | Clear session history |
| GET | `/chat/capabilities` | List capabilities and tool counts (`?mode=read-write` for write budget) |
| POST | `/chat/capabilities` | Update capability states |
| POST | `/chat/approve` | Approve a pending ask-mode tool call |
| POST | `/chat/deny` | Deny a pending ask-mode tool call |
| POST | `/chat/stop` | Cancel an in-progress chat |
| GET | `/health` | Health check |

`POST /chat/message` returns `text/event-stream`. Event names include `message`
(text / tool_call / tool_result / tool_error), `tool_approval_required`,
`canvas_open`, `error`, and `done`.

## Docker

```bash
docker build -t chat-service .
docker run -p 8090:8090 -v ./config.yaml:/etc/chat-service/config.yaml chat-service
```

## Architecture

Go packages are top-level (module `github.com/ebarron/netapp-chat-service`):

```
cmd/chat-service/   Standalone entrypoint (config → ChatDeps → server)
agent/              Agentic tool-use loop, system prompt, tool routing, canvas interceptor
capability/         Capability state model (off/ask/allow) + tool-routing group registry
config/             YAML configuration loading and validation
eval/               Routing-eval harness (offline, opt-in)
interest/           Interest types, catalog, get/save/delete tools, example interests
llm/                LLM provider abstraction (OpenAI, Anthropic, Bedrock, custom)
mcpclient/          MCP server connections and tool routing
render/             Generic object-detail render toolkit (Go structs + fence marshalers)
server/             HTTP server, SSE chat handlers, ask-mode, embedding entrypoint
session/            In-memory conversation sessions
packages/chat-component/   Embeddable React chat UI (@edjbarron/netapp-chat-component)
```

## Documentation

- [docs/chatbot-architecture.md](docs/chatbot-architecture.md) — full architecture:
  agent loop, prompt, tool routing, interests, rendering, capabilities, security.
