# netapp-chat-service

A standalone, product-agnostic chat service that provides an agentic tool-use loop powered by LLMs and MCP (Model Context Protocol) tool servers.

## Features

- **Multi-provider LLM support**: OpenAI, Anthropic, AWS Bedrock, custom endpoints
- **MCP tool routing**: Connect to multiple MCP servers with capability-based tool filtering
- **Interest system**: Pattern-matched prompts that scope tools to relevant capabilities
- **Autonomy modes**: Read-only, read-write, and per-capability ask/allow/off states
- **SSE streaming**: Real-time event streaming for chat responses
- **Tool approval workflow**: Ask-mode tools require user approval before execution

## Quick Start

The fastest path depends on what you need:

### Frontend only (React) — `npm install`

```bash
npm install @edjbarron/netapp-chat-component
```

Drop the `ChatPanel` component into a Mantine-based React app and point it at any chat-service backend. See [Consuming the React component via npm](#consuming-the-react-component-via-npm) for full usage.

### Backend only (standalone server) — download a prebuilt binary

No Go toolchain required. Each tagged release publishes binaries for linux/macOS × amd64/arm64 to [GitHub Releases](https://github.com/ebarron/netapp-chat-service/releases).

```bash
# macOS arm64 example — adjust OS/arch and version
VERSION=v0.1.1
ARCH=darwin_arm64
curl -L https://github.com/ebarron/netapp-chat-service/releases/download/${VERSION}/chat-service_${VERSION#v}_${ARCH}.tar.gz | tar xz

cp config.example.yaml config.yaml
# Edit config.yaml with your LLM provider and MCP server details
./chat-service -config config.yaml
```

Verify integrity using `checksums.txt` from the same release.

### Backend embedded in your Go service — `go get`

```bash
go get github.com/ebarron/netapp-chat-service@latest
```

Import the packages and wire them into your existing HTTP server. See [Go Library (Embedded)](#go-library-embedded) for the full pattern.

### Build from source

```bash
go build -o chat-service ./cmd/chat-service
./chat-service -config config.yaml
```

## Configuration

See [config.example.yaml](config.example.yaml) for a complete example with comments.

Environment variables are expanded in the config file (e.g. `$ANTHROPIC_API_KEY`).

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/chat/message` | Send a message, receive SSE event stream |
| DELETE | `/chat/session` | Clear session history |
| GET | `/chat/capabilities` | List capabilities and tool counts |
| POST | `/chat/capabilities` | Update capability states |
| POST | `/chat/approve` | Approve a pending tool call |
| POST | `/chat/deny` | Deny a pending tool call |
| POST | `/chat/stop` | Cancel an in-progress chat |
| GET | `/health` | Health check |

## Docker

```bash
docker build -t chat-service .
docker run -p 8090:8090 -v ./config.yaml:/etc/chat-service/config.yaml chat-service
```

## Architecture

```
cmd/chat-service/       Main entrypoint (standalone server mode)
agent/                  Agentic tool-use loop orchestration
capability/             Capability state model (off/ask/allow)
config/                 YAML configuration loading
interest/               Interest pattern matching and catalog
llm/                    LLM provider abstraction (OpenAI, Anthropic, Bedrock)
mcpclient/              MCP server connection and tool routing
render/                 Output rendering helpers
server/                 HTTP server and chat handlers
session/                Conversation session management
ui/                     Embedded React chat interface
packages/               Reusable frontend components (chat-component)
```

## Integration Guide

The chat service is designed to be consumed in two ways: as a **standalone server** or as a **Go library** embedded into your own application. All packages are top-level and importable — there are no `internal/` restrictions.

### Pulling the Service into Your Project

This repo is a standalone Go module (`github.com/ebarron/netapp-chat-service`). Integrating products reference it as a normal Go module dependency — there is no vendoring, submodule, or monorepo coupling.

**Add it to your `go.mod`:**

```bash
cd your-service/
go get github.com/ebarron/netapp-chat-service@latest
```

This pulls the published module from GitHub. Your `go.mod` will contain a versioned reference like:

```
require github.com/ebarron/netapp-chat-service v0.0.0-20260407155442-5a64c1cc57c4
```

**Local development workflow:**

For active development across both repos, clone the chat service alongside your project and use a Go workspace or `replace` directive to point at the local copy:

```bash
# Clone alongside your project
git clone https://github.com/ebarron/netapp-chat-service.git

# Option A: Go workspace (preferred — no go.mod edits)
# Add to your go.work:
#   use ./netapp-chat-service
go work use ./netapp-chat-service

# Option B: replace directive (per-module, remember to remove before committing)
# In your go.mod:
#   replace github.com/ebarron/netapp-chat-service => ../netapp-chat-service
```

With either approach, `go build` uses your local source instead of the published module, so you can iterate on both repos simultaneously.

**Publishing changes:**

When you're done with chat service changes:

1. Commit and push to `netapp-chat-service`
2. From your consuming module, run `go get github.com/ebarron/netapp-chat-service@latest` (or `@<commit>`) to update `go.mod` to the new version
3. Remove any `replace` directive or `go.work use` entry for the local clone
4. Commit the updated `go.mod` and `go.sum`

The local clone is not tracked by the consuming project's git — it's purely a development convenience.

### Standalone Server

Run the chat service as its own process, configured entirely via `config.yaml`. This is the simplest option when your product doesn't need to inject custom tools or deeply control the agent lifecycle.

#### Option A: Download a prebuilt binary

Each tagged Go release publishes prebuilt binaries to the [GitHub Releases](https://github.com/ebarron/netapp-chat-service/releases) page (linux/macOS, amd64/arm64). No Go toolchain required on the consumer's machine.

```bash
# macOS arm64 example — adjust OS/arch and version as needed
VERSION=v0.1.1
ARCH=darwin_arm64
curl -L https://github.com/ebarron/netapp-chat-service/releases/download/${VERSION}/chat-service_${VERSION#v}_${ARCH}.tar.gz | tar xz
./chat-service -config config.yaml
```

Verify integrity using `checksums.txt` from the same release.

#### Option B: Build from source

```bash
go build -o chat-service ./cmd/chat-service
./chat-service -config config.yaml
```

Your product talks to it over HTTP (SSE for streaming). See [API Endpoints](#api-endpoints) above.

### Go Library (Embedded)

Import the chat service packages into your own Go application and wire them into your existing HTTP server. This gives you full control over authentication, custom tools, MCP server discovery, and product-specific behavior.

#### 1. Add the dependency

```bash
go get github.com/ebarron/netapp-chat-service@latest
```

#### 2. Core packages to import

```go
import (
    "github.com/ebarron/netapp-chat-service/agent"
    "github.com/ebarron/netapp-chat-service/capability"
    "github.com/ebarron/netapp-chat-service/interest"
    "github.com/ebarron/netapp-chat-service/llm"
    "github.com/ebarron/netapp-chat-service/mcpclient"
    "github.com/ebarron/netapp-chat-service/render"
    "github.com/ebarron/netapp-chat-service/session"
)
```

#### 3. Initialize the components

The integration follows a consistent pattern: create a provider, connect MCP servers, build capabilities, load interests, then wire into your routes.

```go
// Create LLM provider from your product's config (persisted however you like).
provider, err := llm.NewProvider(llm.ProviderConfig{
    Provider: "anthropic",
    Endpoint: "https://api.anthropic.com",
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "claude-sonnet-4-20250514",
})

// Create MCP router and connect to tool servers.
router := mcpclient.NewRouter(slog.Default())
defer router.Close()
router.ConnectAll([]mcpclient.ServerConfig{
    {Name: "my-mcp-server", Endpoint: "http://localhost:8082"},
}, 10, 2*time.Second)

// Define capabilities (map MCP servers to user-facing permission groups).
caps := []capability.Capability{
    {ID: "metrics", Name: "Metrics", ServerName: "my-mcp-server", State: capability.StateAllow},
}

// Load interest catalog (pattern-matched prompt templates).
catalog := interest.NewCatalog(slog.Default())
catalog.Load([]string{"./interests"}, map[string]bool{"metrics": true})

// Create session manager (in-memory, sliding window).
sessions := session.NewManager(40)
```

#### 4. Mount chat handlers

You have two options for serving the chat API:

**Option A: Use the built-in `server.Server`** — mounts all chat endpoints for you.

```go
import "github.com/ebarron/netapp-chat-service/server"

chatServer := server.New(&server.ChatDeps{
    Sessions:     sessions,
    Provider:     provider,
    Router:       router,
    Model:        "claude-sonnet-4-20250514",
    Capabilities: caps,
    Catalog:      catalog,
    PromptConfig: agent.SystemPromptConfig{
        ProductName:        "My Product",
        ProductDescription: "what my product does",
    },
})

// Mount under your existing mux.
mux.Handle("/chat/", chatServer.Handler())
```

**Option B: Write your own handlers** — import the packages directly and build custom route handlers that call the agent loop yourself. This is what NAbox does — it wraps the agent, session, and capability packages inside its own `chi` router with product-specific auth middleware, SSE streaming, and config persistence. This approach is more work but gives you full control over the request lifecycle.

#### 5. Product customization points

| Customization | How |
|---|---|
| **System prompt** | Set `agent.SystemPromptConfig` with your product name, description, and guidelines |
| **Custom tools** | Register `agent.InternalTool` entries — local tool handlers that don't require an MCP server |
| **MCP discovery** | Use `mcpclient.NewDockerDiscoverer()` to find MCP servers from Docker labels at runtime |
| **Capability states** | Persist user preferences (off/ask/allow per capability) and merge on init with `capability.Merge()` |
| **Interests** | Ship built-in interests via `//go:embed` and support user-created interests from a config directory |
| **UI** | Use the embedded UI (`ui.Dist`) via `server.ServeUI()`, or build your own frontend against the SSE API |
| **OnBeforeInit hook** | Run product-specific setup (e.g. provisioning service account tokens) before MCP connections |

#### 6. SSE event protocol

When using Option B (custom handlers), your SSE endpoint should emit these event types from the agent loop:

| Event | Payload | Description |
|---|---|---|
| `message` | `{text}` | LLM text output (streamed incrementally) |
| `tool_call` | `{id, name, status}` | Tool execution started |
| `tool_result` | `{id, name, result, error}` | Tool completed or errored |
| `tool_approval_required` | `{approval_id, tool, input}` | Ask-mode tool awaiting user approval |
| `canvas_open` | `{type, data}` | Object detail view for rich UI rendering |
| `done` | `{type, session_id}` | Agent finished processing |
| `error` | `{message}` | Unrecoverable error |

#### 7. Frontend integration

The `packages/chat-component/` directory contains a reusable React chat component that handles the SSE protocol, tool approval UI, and message rendering. You can either:

- Use it directly in your React app as an npm dependency (see [Consuming the React component via npm](#consuming-the-react-component-via-npm))
- Use the full embedded UI from `ui/` via `server.ServeUI()`
- Build your own frontend using the SSE event protocol above

### Consuming the React component via npm

The chat component is published to public npm as **[`@edjbarron/netapp-chat-component`](https://www.npmjs.com/package/@edjbarron/netapp-chat-component)**.

#### Install

```bash
npm install @edjbarron/netapp-chat-component
```

#### Peer dependencies

You must install these in your host application:

```bash
npm install react@^18 || ^19 \
            react-dom@^18 || ^19 \
            @mantine/core@^8 \
            @mantine/charts@^8 \
            @mantine/hooks@^8 \
            @tabler/icons-react@^3
```

| Peer | Required version |
|---|---|
| `react` | `^18.0.0 || ^19.0.0` |
| `react-dom` | `^18.0.0 || ^19.0.0` |
| `@mantine/core` | `^8.0.0` |
| `@mantine/charts` | `^8.0.0` |
| `@mantine/hooks` | `^8.0.0` |
| `@tabler/icons-react` | `^3.0.0` |

#### Minimum usage

```tsx
import { MantineProvider } from '@mantine/core';
import {
  ChatPanel,
  ChatAPIProvider,
  createChatAPI,
} from '@edjbarron/netapp-chat-component';

import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import '@edjbarron/netapp-chat-component/styles.css';

const api = createChatAPI({ baseUrl: 'https://your-chat-service.example.com' });

export function App() {
  return (
    <MantineProvider>
      <ChatAPIProvider value={api}>
        <ChatPanel />
      </ChatAPIProvider>
    </MantineProvider>
  );
}
```

#### Backend requirement

The component is purely a UI client — it talks to the `netapp-chat-service` Go backend over the SSE event protocol described in [SSE event protocol](#6-sse-event-protocol). To use the npm package you must also:

- Run the Go server (see [Standalone Server](#standalone-server)), **or**
- Embed the chat service in your own Go application (see [Go Library (Embedded)](#go-library-embedded)), **or**
- Implement the [API endpoints](#api-endpoints) yourself in another language using the same SSE contract.

The component does not bundle, vendor, or require the Go backend at install time.

#### Versioning and releases

The package follows [semver](https://semver.org/). Releases are tracked in [CHANGELOG.md](packages/chat-component/CHANGELOG.md) and tagged in git as `chat-component-vX.Y.Z`. See all releases at <https://github.com/ebarron/netapp-chat-service/releases>.

Publishing to npm is automated via GitHub Actions ([`.github/workflows/publish-chat-component.yml`](.github/workflows/publish-chat-component.yml)) using **npm trusted publisher (OIDC)** — no long-lived tokens are stored in the repo. To cut a release, maintainers bump the version in `packages/chat-component/package.json`, push a `chat-component-vX.Y.Z` tag, and the workflow publishes automatically.

### Example: NAbox integration

NAbox embeds the chat service as a Go library. Here's how the integration is structured:

- **`chatbot.go`** — Initialization: loads AI config from YAML, creates provider + router + capabilities, defines custom tools (volume monitoring, alert management), configures Docker-based MCP discovery, provisions Grafana service account tokens
- **`routes/ai.go`** — Config management API: CRUD for LLM provider settings, provider validation, model listing
- **`routes/chat.go`** — Chat API: SSE streaming endpoint, session management, capability state persistence, tool approval workflow — all using the chat service's `agent`, `session`, and `capability` packages within NAbox's own auth middleware stack
- **`interests/*.md`** — Built-in interests embedded via `//go:embed` for domain-specific prompt routing
- **`admin-ui/`** — Custom React frontend that consumes the SSE API and renders tool results, canvas views, and approval dialogs
