# netapp-chat-service: Reusable Chat Interface

> **Status:** Phase 0 complete. Phase 1-3 are in progress with partial implementation. See Current State Summary and phase sections for exact remaining work.
> **Audience:** Engineering teams integrating AI chat into NetApp products
> **Origin:** Extracted from host application chatbot (v4)
> **Last Updated:** March 30, 2026

---

## Current State Summary

The chatbot is fully functional within host application. The backend Go packages are portable (zero monolith imports — only stdlib, third-party libs, and peer chatbot packages). A separate `netapp-chat-service` repository now exists and is being consumed by host application, but host application has not yet fully cut over to an external sidecar deployment path.

| Area | Status |
|------|--------|
| Backend Go packages (agent, llm, mcpclient, session, capability, interest) | **Ready for extraction** — zero host application-specific imports |
| Backend render package | **Mostly portable** — volume-detail renderer has host application-specific Harvest integration |
| Product-specific tool injection (`ExtraTools` pattern) | **Implemented** — `ChatDeps.ExtraTools` map allows host products to register internal tools |
| Canvas system (object-detail views in dedicated tabs) | **Implemented** — `canvas_open` SSE event, `CanvasPanel.tsx`, `output_target` interest field |
| Interest pre-filtering (tool reduction by trigger match) | **Implemented** — `Catalog.Match()` narrows tool list per interest `requires` |
| Standalone `netapp-chat-service` repo and server | **In progress** — repo, main server entrypoint, and Dockerfile exist |
| Frontend `chat-component` package consumption in host application | **Implemented (dev wiring)** — host application frontend imports package via workspace portal dependency |
| Frontend `@netapp/chat-component` publish-ready package | **In progress** — still Mantine-coupled, not framework-agnostic yet |
| host application sidecar cutover (`/api/chat` path + removal of in-process handlers) | **Not started** |
| MCP discovery in host application | **In progress** — discovery code and compose labels added; runtime validation and socket portability still being finalized |
| Phase 0 in-place refactoring | **Complete** — SystemPromptConfig, ChatbotConfig, RunChat(), Catalog.Load(dirs), ExtraTools extraction |

## 1. Problem Statement

Multiple NetApp products need conversational AI interfaces that can query infrastructure, render rich visualizations, and take guided actions — but each product today must build this from scratch. The host application chatbot has proven the pattern: an LLM-powered agent that connects to MCP tool servers, renders interactive dashboards and object-detail views inline, and enforces capability-based access controls. Rather than duplicate this stack in every product, we should extract it as a reusable microservice + frontend component that any product can integrate.

## 2. Goals

1. **Ship once, integrate everywhere** — A single Go microservice (`netapp-chat-service`) that any product deploys as a container alongside its existing services.
2. **Frontend flexibility** — A reference React component for products using React; a well-documented SSE API contract for products using other frameworks.
3. **Per-product interests, shared MCPs** — Each product provides its own interest catalog (mounted volume). MCP backends (Harvest, ONTAP, Grafana) are shared infrastructure that any product can connect to.
4. **Auth-agnostic** — The service assumes a token is passed on every request. Each product's auth stack (Keycloak, JWT, OAuth2) sits in front.
5. **Self-contained config** — The microservice owns its own configuration (LLM provider, MCP endpoints, interests). No dependency on host product config systems.

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│                   Host Product                       │
│  ┌────────────┐  ┌──────────────────────────────┐   │
│  │            │  │      Product Frontend         │   │
│  │   Auth     │  │  ┌────────────────────────┐   │   │
│  │   Proxy    │  │  │  Chat Component        │   │   │
│  │            │  │  │  (React or custom)      │   │   │
│  │  (Keycloak,│  │  └───────────┬────────────┘   │   │
│  │   OAuth2,  │  │              │ SSE             │   │
│  │   etc.)    │  └──────────────┼────────────────┘   │
│  │            │                 │                     │
│  └─────┬──────┘                 │                     │
│        │ token                  │                     │
│  ┌─────▼────────────────────────▼──────────────────┐ │
│  │          netapp-chat-service                     │ │
│  │  ┌─────────┐ ┌───────┐ ┌──────────┐ ┌────────┐ │ │
│  │  │ Agent   │ │ LLM   │ │ Interest │ │ Render │ │ │
│  │  │ Loop    │ │Client │ │ Catalog  │ │ Engine │ │ │
│  │  └────┬────┘ └───────┘ └──────────┘ └────────┘ │ │
│  │       │                                          │ │
│  │  ┌────▼────────────────────────────────────────┐ │ │
│  │  │           MCP Client / Router               │ │ │
│  │  └────┬──────────┬──────────┬──────────────────┘ │ │
│  └───────┼──────────┼──────────┼────────────────────┘ │
│          │          │          │                       │
│  ┌───────▼───┐ ┌────▼────┐ ┌──▼──────┐               │
│  │ Harvest   │ │ ONTAP   │ │ Grafana │               │
│  │ MCP       │ │ MCP     │ │ MCP     │               │
│  └───────────┘ └─────────┘ └─────────┘               │
└─────────────────────────────────────────────────────┘
```

## 4. Target Consumers

### 4.1 host application (Current)

- **Deployment:** Flatcar Container Linux appliance (OVA/QCOW2)
- **Frontend:** React 19 + Mantine 8
- **Current state:** Chatbot is embedded in `chat-service` monolith. All backend packages (agent, llm, mcpclient, session, capability, interest) are portable with zero monolith imports. Canvas system, interest pre-filtering, and ExtraTools injection pattern are implemented.
- **Migration:** Extract chat backend from chat-service into netapp-chat-service container; chat-service proxies or frontend calls directly
- **Auth:** host application JWT/BasicAuth stack; chat-service sits behind Caddy reverse proxy

### 4.2 Other Products

Any product with MCP servers can consume `netapp-chat-service`. The service is product-agnostic — it accepts MCP server URLs, interest directories, and a `SystemPromptConfig` via `config.yaml`. Products deploy it as a sidecar container alongside their existing stack.

## 5. Microservice Design

### 5.1 Container: `netapp-chat-service`

A single Go binary in a distroless container image. No sidecar dependencies.

**Responsibilities:**
- Accept chat messages via HTTP (SSE streaming responses)
- Connect to configured MCP servers and route tool calls (with `ConnectAll` retry/backoff)
- Manage LLM provider connections (OpenAI, Anthropic, Bedrock, llm-proxy)
- Load and serve interest catalog (built-in + user-defined)
- Pre-filter tools by matched interest triggers (reduces LLM context size)
- Execute bespoke render functions for high-value views
- Enforce capability states and read-only/read-write mode
- Manage chat sessions (in-memory, per-user)
- Support canvas output (object-detail and dashboard views in dedicated tabs)
- Accept product-specific internal tools via `ExtraTools` injection

**Does NOT do:**
- Authentication (expects pre-authenticated requests with token)
- User management
- Host product routing or UI serving
- Persistent storage (sessions are ephemeral)

### 5.2 Configuration

The service owns its config via environment variables and a mounted config file.

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CHAT_PORT` | HTTP listen port | `8090` |
| `CHAT_CONFIG_PATH` | Path to config file | `/etc/chat-service/config.yaml` |
| `CHAT_LOG_LEVEL` | Log level | `info` |
| `DEBUG` | Enable debug logging | `false` |

#### Config File (`config.yaml`)

```yaml
llm:
  provider: llm-proxy          # openai | anthropic | bedrock | llm-proxy
  endpoint: https://llm-proxy-api.ai.openeng.netapp.com
  api_key: ${CHAT_LLM_API_KEY} # or from K8s Secret mount
  model: gpt-5.4

mcp_servers:
  - name: harvest
    url: http://harvest-mcp:8082
    capability: metrics
  - name: ontap
    url: http://ontap-mcp:8084
    capability: storage
  - name: grafana
    url: http://grafana-mcp:8086
    capability: dashboards

capabilities:
  defaults:
    metrics: allow
    storage: allow
    dashboards: allow

interests:
  product_dir: /etc/chat-service/interests   # product-provided interests (mounted volume)
  user_dir: /data/interests                  # user-created interests (persistent volume)
  max_user_interests: 10

canvas:
  max_tabs: 5                                # FIFO eviction when exceeded

extra_tools: []                              # product-specific internal tools (injected at startup)

session:
  timeout: 30m
  read_write_timeout: 10m
```

### 5.3 API Contract

All endpoints are prefixed with a configurable base path (default `/api/chat/`).

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/chat/message` | Send message, receive SSE stream (body includes optional `canvas_tabs` for LLM context) |
| `GET` | `/api/chat/capabilities` | Returns capability states + tool counts |
| `PUT` | `/api/chat/capabilities` | Update capability states |
| `POST` | `/api/chat/approve` | Approve a pending tool execution |
| `POST` | `/api/chat/deny` | Deny a pending tool execution |
| `POST` | `/api/chat/stop` | Cancel ongoing stream |
| `DELETE` | `/api/chat/session` | Clear current session |

> **Note:** LLM provider config is managed separately by the host product. In host application, this is at `GET/POST/DELETE /config (host-integration)config`. The chat service itself does not expose a config endpoint — it reads config from file/env at startup.

#### Chat Message Request

```json
{
  "message": "What's my fleet health?",
  "mode": "read-only",
  "session_id": "abc123",
  "canvas_tabs": [
    {"tab_id": "vol-1", "title": "Volume: vol_data_01", "type": "object-detail"}
  ]
}
```

The `canvas_tabs` field is optional. When present, the LLM receives context about currently open canvas tabs so it can reference or update them.

#### SSE Event Types

| Event | Description |
|-------|-------------|
| `message` (text) | Streaming text token |
| `tool_call` | Tool execution started (includes tool name, params, capability) |
| `tool_result` | Tool execution completed |
| `tool_error` | Tool execution failed |
| `tool_approval_required` | Pause for user approval (ask-mode) |
| `text_clear` | Clear accumulated text buffer (emitted before canvas events) |
| `canvas_open` | Open content in canvas tab (object-detail or dashboard) |
| `error` | Fatal error, stream stops |
| `done` | Stream complete (includes session_id) |

### 5.4 Auth Integration

The service is **auth-agnostic**. It expects the host product's auth layer to:

1. Authenticate the user before requests reach the chat service
2. Pass a token (Bearer, cookie, or header) that the chat service can forward to MCP servers
3. Optionally pass user identity in a header (e.g., `X-Chat-User: ebarron`) for session scoping

```
User → [Host Auth Proxy] → netapp-chat-service
         (validates token,     (trusts the proxy,
          adds user header)     scopes session by user)
```

The chat service does NOT validate tokens. It trusts the upstream proxy.

In Kubernetes, this is typically handled by:
- **Traefik/Ingress** with auth middleware
- **Istio/Linkerd** service mesh with mTLS
- **OAuth2 Proxy** sidecar

## 6. Frontend Integration

### 6.1 Option A: Built-in Config UI (Zero Integration)

For products that want a completely self-contained chat widget, the service includes an optional config panel accessible via a gear icon in the chat header:

- LLM provider selection (OpenAI, Anthropic, llm-proxy)
- Endpoint and API key
- Model selection
- Capability toggles

Config changes are persisted to the service's config file or a ConfigMap.

This mode requires **zero host-app integration** beyond deploying the container and mounting the React component (or iframe).

### 6.2 Option B: External Config (Props/Hooks)

For products that own their settings experience, the chat component accepts config externally:

```typescript
interface ChatServiceConfig {
  apiBaseUrl: string;        // e.g., "https://myproduct.com/api/chat"
  authToken?: string;        // passed as Bearer header
  configMode: 'built-in' | 'external';
  position?: 'left' | 'right';
  onOpen?: () => void;
  onClose?: () => void;
}
```

The host app provides the API base URL and auth token. The chat component handles everything else.

### 6.3 Framework Strategy

host application is a React app. The extracted component must be framework-independent enough that other products can consume it.

**Implications:**

- **Component library:** The chat component must NOT depend on Mantine. Ship with self-contained styles (CSS modules).
- **Charts:** Bundle Recharts — it's already proven in host application and lightweight.
- **Language:** TypeScript with compiled JS output.

| Integration Path | Description |
|-----------------|-------------|
| npm package | `@netapp/chat-component` — bundled styles, bundled Recharts, React 18+ peer dep |
| Built-in UI | Chat service serves its own minimal web page for products without a frontend |
| SSE API only (fallback) | For non-React consumers: documented API contract + reference client |

### 6.4 Mounting the Chat Component

**Trigger button:** The library exports a `<ChatTrigger />` component (an icon button) that the host app places in its nav, header, or FAB position. Clicking it opens the chat drawer.

**Drawer rendering:** The chat drawer renders as a portal (full-viewport overlay), independent of the host app's layout. No AppShell integration required.

```tsx
import { ChatTrigger, ChatDrawer } from '@netapp/chat-component';

function MyApp() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <MyNavbar>
        <ChatTrigger onClick={() => setOpen(true)} />
      </MyNavbar>
      <ChatDrawer
        opened={open}
        onClose={() => setOpen(false)}
        apiBaseUrl="/api/chat"
      />
    </>
  );
}
```

## 7. Kubernetes Deployment

### 7.1 Helm Chart

`netapp-chat-service` ships its own Helm chart, deployable standalone or as part of an existing Docker Compose / Kubernetes stack.

```yaml
# values.yaml (key fields)
replicaCount: 1

image:
  repository: ghcr.io/netapp/netapp-chat-service
  tag: latest

config:
  llm:
    provider: llm-proxy
    endpoint: https://llm-proxy-api.ai.openeng.netapp.com
    model: gpt-5.4
  mcp_servers:
    - name: harvest
      url: http://harvest-mcp:8082
      capability: metrics
    - name: ontap
      url: http://ontap-mcp:8084
      capability: storage

secrets:
  llmApiKey:
    secretName: chat-service-secrets
    key: llm-api-key

service:
  port: 8090

ingress:
  enabled: true
  path: /api/chat
```

### 7.2 Deployment Topology

**host application deployment:**
```
Caddy (reverse proxy)
  ├── /     → chat-service:8080       (admin API)
  ├── /api/chat/    → chat-service:8090  (chat API)
  ├── /grafana/     → grafana:3000
  └── /             → chat-service:8080        (embedded UI)

chat-service:8090
  ├── harvest-mcp:8082
  ├── ontap-mcp:8084
  └── grafana-mcp:8086
```

**Harvest (Docker Compose) deployment:**
```
chat-service:8090
  ├── /              → built-in chat UI (served by chat-service)
  ├── /api/chat/     → chat API
  └── harvest-mcp:8082 (existing Harvest MCP container)

# Added to Harvest's existing docker-compose.yml:
services:
  chat-service:
    image: ghcr.io/netapp/netapp-chat-service:latest
    ports: ["8090:8090"]
    environment:
      CHAT_CONFIG_PATH: /etc/chat-service/config.yaml
    volumes:
      - ./chat-config.yaml:/etc/chat-service/config.yaml
```

### 7.3 MCP Server Discovery

Initially, MCP server URLs are explicit in config. Future iterations may adopt a registry/discovery model:

```yaml
# Future: registry-based discovery
mcp_discovery:
  mode: registry            # static | registry
  registry_url: http://mcp-registry:8080
  poll_interval: 30s
```

This would allow the chat service to dynamically discover MCP servers as they are deployed, rather than requiring config changes.

## 8. Extraction Plan

### 8.1 What Moves Out of host application

| Current Location | Destination | Notes |
|-----------------|-------------|-------|
| `agent/` | `netapp-chat-service/internal/agent/` | Agent loop + canvas fence interception — already portable, zero host application imports |
| `llm/` | `netapp-chat-service/internal/llm/` | LLM client — already portable |
| `mcpclient/` | `netapp-chat-service/internal/mcpclient/` | MCP client/router — already portable |
| `session/` | `netapp-chat-service/internal/session/` | Session management — already portable |
| `capability/` | `netapp-chat-service/internal/capability/` | Capability states — already portable |
| `interest/` | `netapp-chat-service/internal/interest/` | Minor refactor: parameterize directory paths. Interest pre-filtering (`Catalog.Match`) and `output_target` field already implemented. |
| `render/` | `netapp-chat-service/internal/render/` | Minor refactor: configurable metric query templates |
| `server/server.go` | `netapp-chat-service/internal/server/` | Refactor: standalone HTTP server, remove chat-service auth deps. ExtraTools injection pattern already supports product-agnostic tool registration. |
| `cmd/chat-service/main.go` | `netapp-chat-service/cmd/chat-service/main.go` | Refactor: config from file/env instead of chat-service wiring. `ConnectAll()` retry logic already extracted to router. |
| `chat-service/frontend/.../ChatPanel/` | `@netapp/chat-component` npm package | Refactor: remove Mantine peer dep, bundle styles. Canvas panel, action-form, object-detail, and all 14 panel types must be included. |

### 8.2 What Stays in host application

| Component | Reason |
|-----------|--------|
| `chat-service/internal/alertmgr/` | Volume monitoring rules — host application-specific domain logic |
| `render/` | Volume-detail renderer — host application-specific Harvest metric integration (extract interface, keep implementation) |
| host application auth middleware | Product-specific auth stack |
| AI config page (`AIConfigPage.tsx`) | host application settings UI; replaced by built-in config or external props |
| `chat-service/main.go` app wiring | host application-specific initialization |

### 8.3 What host application Gains

After extraction, host application's `chat-service` becomes thinner:

- Deploys `netapp-chat-service` as a sidecar container (or systemd unit)
- Caddy routes `/api/chat/` to the chat service
- Admin UI imports `@netapp/chat-component` instead of local ChatPanel
- Alert manager registers as an internal tool via the chat service's tool registration API
- host application-specific interests remain embedded in host application, mounted into the chat service container

### 8.4 Phased Approach

#### Phase 0: Refactor in-place (inside host application) *(complete)*

Reduce coupling before extraction. All changes stay in the host application repo, all tests stay green. This phase had no external dependencies.

**0a. Parameterize `BuildSystemPrompt()`**
- File: `agent/agent.go` (~line 646)
- Currently hardcodes "host application Assistant" identity, ONTAP/StorageGRID service names, and `/grafana/` URL rewriting
- Create a `SystemPromptConfig` struct:
  ```go
  type SystemPromptConfig struct {
      ProductName        string   // "host application Assistant"
      ProductDescription string   // "AI-powered storage infrastructure expert..."
      Services           []string // ["ONTAP", "StorageGRID"]
      URLRewriteRules    []URLRewriteRule // [{From: "http://grafana:3000", To: "/grafana"}]
  }
  ```
- Pass config into `BuildSystemPrompt()` instead of hardcoding
- host application passes its current values — behavior unchanged, but now injectable
- **Tests:** Update `agent_test.go` to verify prompt contains injected product name

**0b. Refactor `initChatbot()` to accept dependencies**
- File: `cmd/chat-service/main.go`
- Currently imports: `internal/alertmgr`, `internal/render`, `pkg/grafana`, `pkg/prometheus`, `host application-go` — all host application-specific
- Create a `ChatbotConfig` struct:
  ```go
  type ChatbotConfig struct {
      ConfigPath   string                          // ai.yaml path
      InterestsDir string                          // product interest files
      MCPServers   []mcpclient.ServerConfig        // no more hardcoded URLs
      ExtraTools   map[string]agent.InternalTool   // product-specific tools
      Logger       *slog.Logger
  }
  ```
- Move `ensureGrafanaMCPToken()` out of `initChatbot()` — call it in `main.go` before calling `initChatbot()`, then pass the MCP configs with the token already set
- Move `ExtraTools` construction (alertmgr, render_volume) to `main.go`
- `initChatbot(cfg ChatbotConfig)` becomes host application-agnostic
- **Tests:** Verify `initChatbot()` works with mock config (no filesystem, no containers)

**0c. Extract chat handler logic from HTTP layer**
- File: `server/server.go`
- Currently `PostChatMessage()` mixes HTTP concerns (SSE headers, flushing) with agent orchestration (building agent, running loop, emitting events)
- Extract a `RunChat()` function that takes an event callback — no HTTP awareness:
  ```go
  func RunChat(ctx context.Context, deps ChatDeps, req ChatMessageRequest, emit func(event, data)) error
  ```
- `PostChatMessage()` becomes a thin HTTP wrapper that sets up SSE and calls `RunChat()`
- **Tests:** Test `RunChat()` directly without `httptest` — easier to validate agent behavior

**0d. Make interest catalog load from directories only**
- File: `interest/catalog.go`
- Currently `Load(embedded fs.FS, userDir string, enabled map[string]bool)` — takes an `fs.FS` for built-in interests
- Change to `Load(dirs []string, enabled map[string]bool)` — accepts a list of directories
- host application embeds interests to disk at startup (or mounts them), then passes the directory path like any other product
- **Tests:** Update `catalog_test.go` to use temp directories instead of embedded FS

**Testing checkpoint:** All host application tests pass. No behavioral changes. Chat works exactly as before.

> **Completed 2025-03-30** — All 5 tasks done in commit `6ceff81`. `go test ./...` and `go vet ./...` clean.

---

#### Phase 1: Extract backend *(in progress)* — REQUIRED for host application

Create the standalone `netapp-chat-service` repo and move Go packages. host application then consumes the chat service as a sidecar container.

**1a. Create private repo `github.com/NetApp/netapp-chat-service`** — **Done**
- Go module: `github.com/NetApp/netapp-chat-service`
- Copy packages: `internal/agent`, `internal/llm`, `internal/mcpclient`, `internal/session`, `internal/capability`, `internal/interest`, `internal/render`
- These are already portable after Phase 0 refactoring

**1b. Build standalone HTTP server** — **Done (initial version)**
- `cmd/chat-service/main.go` — reads `config.yaml`, calls `initChatbot(cfg)`, starts HTTP server
- `internal/server/` — HTTP handlers (adapted from `RunChat()` extracted in 0c)
- Serves SSE streams, capability management, session management
- No auth middleware — trusts upstream proxy

**1c. Container image** — **Partially done**
- Dockerfile (distroless base, single binary)
- GitHub Actions CI: build, test, push to `ghcr.io/netapp/netapp-chat-service` (private) — **pending**

**1d. host application integration** — **Not started**
- Add `netapp-chat-service` container to host application's Docker Compose / systemd
- Caddy routes `/api/chat/` → chat-service:8090
- `chat-service` removes chat handler code, keeps only ExtraTools registration + proxy/redirect
- host application mounts its interest files into the chat-service container

**Testing:**
- Chat service unit tests: agent loop, SSE streaming, capability management, interest loading
- Chat service integration test: spin up service with mock MCP server, send messages, verify SSE events
- host application regression: existing e2e chatbot tests (`chatbot.spec.ts`) pass against the external chat service
- Verify: canvas events, tool approval flow, read-write mode, interest retrieval all work end-to-end

---

#### Phase 2: Extract frontend *(in progress)* — REQUIRED for host application

Publish `@netapp/chat-component` npm package. host application's frontend then imports the package instead of its local ChatPanel.

**2a. Create chat API client library** — **Partially done**
- Extract SSE parsing, message state management, capability fetching from `useChatPanel.ts`
- Configurable `apiBaseUrl` — no hardcoded `/` paths
- Pure TypeScript, no React dependency in the client layer

**2b. Build framework-agnostic chat component** — **Not started**
- Remove Mantine dependencies (currently 13+ Mantine component imports in `ChatPanel.tsx`)
- Bundle self-contained styles (CSS modules)
- Bundle Recharts for chart rendering
- Include all 14 panel types, canvas panel, action-form

**2c. host application consumes the package** — **Done (workspace portal dependency)**
- `chat-service/frontend` imports `@netapp/chat-component` instead of local ChatPanel
- Passes `apiBaseUrl`, auth token, product-specific props

**2d. Built-in UI shell** — **Not started**
- Minimal HTML page served by chat-service itself for products without a frontend
- Just the chat component in a full-page layout

**Testing:**
- Component unit tests (Vitest + Testing Library) for all panel types
- Visual regression tests for chart rendering
- host application e2e tests pass with the npm package replacing local components
- Built-in UI: basic smoke test (loads, sends message, renders response)

---

#### Phase 3: MCP server discovery *(in progress)* — REQUIRED for host application

Registry-based dynamic MCP server discovery. Currently host application hardcodes MCP server URLs in `ChatbotConfig.MCPServers`. With discovery, MCP servers register themselves (e.g., via Docker labels or a simple registry endpoint), so adding or removing an MCP server from the stack requires no code changes.

Current implementation status:
- Discovery interface and Docker discoverer are implemented in `netapp-chat-service/mcpclient`.
- host application `ChatbotConfig` supports dynamic discovery via `Discoverer` and polling interval.
- Compose services include `mcp.discover=true` labels and endpoint metadata.
- Remaining: complete runtime validation across dev/appliance environments and finalize socket path portability defaults.

---

### 8.5 Pre-Extraction Refactoring Checklist

Work that can be done **now, inside host application**, to prepare for extraction:

| # | Task | File(s) | Coupling Removed | Status |
|---|------|---------|-----------------|--------|
| 1 | Parameterize `BuildSystemPrompt()` with `SystemPromptConfig` | `agent/agent.go`, `agent_test.go` | Hardcoded product identity, service names, URL rewriting | ✅ Done |
| 2 | Refactor `initChatbot()` to accept `ChatbotConfig` struct | `chatbot.go`, `main.go` | host application package imports, hardcoded MCP URLs, Grafana token provisioning | ✅ Done |
| 3 | Extract `RunChat()` from `PostChatMessage()` | `server/server.go`, `routes/chat_test.go` | HTTP layer mixed with agent orchestration | ✅ Done |
| 4 | Change `Catalog.Load()` to accept directory list instead of `fs.FS` | `interest/catalog.go`, `interest/catalog_test.go` | Embedded FS dependency | ✅ Done |
| 5 | Move ExtraTools construction to `main.go` | `chatbot.go`, `main.go` | alertmgr/render imports in chatbot.go | ✅ Done |

## 9. Features Built Since Initial Design

These features were implemented in host application after the initial design draft and should be carried into the extracted service.

### 9.1 Canvas System

The canvas provides dedicated tabs for rich object-detail and dashboard views, separate from the chat message stream.

**Backend:**
- `agent/canvas.go` — Fence interceptor detects ` ```canvas-object-detail ` and ` ```canvas-dashboard ` fences in LLM output, emits `canvas_open` agent events
- `render/volume.go` — `MarshalCanvasBlock()` returns canvas-fenced JSON for object-detail views
- `CanvasTabSummary` struct passed to `BuildSystemPrompt()` so the LLM knows what tabs are pinned

**Frontend:**
- `CanvasPanel.tsx` — Renders pinned tabs (max 5, FIFO eviction) using `ObjectDetailBlock` or `DashboardBlock`
- `useChatPanel.ts` — Manages `canvasTabs` state, sends `canvas_tabs` summaries in chat requests
- SSE events: `text_clear` (clears buffer before canvas), `canvas_open` (opens tab with JSON payload)

**Interest integration:**
- `output_target` field in interest YAML frontmatter (`"canvas"` or `"chat"`, default `"chat"`)
- Interests with `output_target: canvas` direct their output to a canvas tab instead of inline

### 9.2 Interest Pre-Filtering

When a user message matches an interest's triggers, the tool list sent to the LLM is narrowed to only the capabilities listed in that interest's `requires` field. This reduces context size (e.g., ~150 tools → ~40) and improves response quality.

- `Catalog.Match(userMessage)` — Checks message against interest trigger phrases
- Matched interest's `requires` field determines which MCP servers' tools are included
- Unmatched messages get the full tool set

### 9.3 Product-Specific Tool Injection

The `ChatDeps.ExtraTools` map allows host products to register internal tools without modifying chatbot core code:

```go
type ChatDeps struct {
    ExtraTools map[string]agent.InternalTool
    // ...
}
```

host application uses this to inject `render_volume` and alertmanager tools. Other products can inject their own domain-specific tools.

### 9.4 New Panel Types (14 total, up from 12)

**Added since initial design:**
- `action-form` — Interactive form with input fields, dropdowns, and submit button that triggers a tool call
- `object-detail` — Multi-section object view with layouts: properties, chart, alert-list, timeline, actions, text, table

**Supporting components:**
- `ActionFormBlock.tsx` — Renders form fields with validation and submit handling
- `ObjectDetailBlock.tsx` — Renders sectioned object views
- `PropertiesSection.tsx` — Key-value property tables within object-detail
- `TimelineSection.tsx` — Event timeline within object-detail
- `AutoJsonBlock.tsx` — Fallback JSON renderer for unknown types

### 9.5 Built-in Interests (6 total, up from 3)

| Interest | Description | Output Target |
|----------|-------------|---------------|
| `morning-coffee` | Fleet health check | chat |
| `morning-coffee-v2` | Updated fleet health check | chat |
| `volume-detail` | Volume deep-dive with metrics and alerts | canvas |
| `volume-provision` | Volume lifecycle workflow | canvas |
| `resource-status` | Resource utilization overview | canvas |
| `object-list` | Multi-resource listings | chat |

### 9.6 MCP Router Improvements

- `ConnectAll(servers, maxAttempts, retryDelay)` — Batch-connect with configurable retry/backoff
- `Close()` — Graceful shutdown of all MCP sessions
- `rebuildToolIndex()` — Internal tool cache rebuild after connect/disconnect

## 10. Open Questions

| # | Question | Owner | Status |
|---|----------|-------|--------|
| 1 | ~~NACL frontend framework and component library?~~ | — | **Removed:** NACL is no longer a target consumer |
| 2 | ~~Auth model for consumers?~~ | — | **Resolved:** Chat service is auth-agnostic. Trusts upstream proxy. If host has tokens, they're forwarded in headers. If host has no auth (e.g., Harvest), service works without it. |
| 3 | ~~MCP service registry?~~ | — | **Resolved:** Not needed for initial consumers. Harvest already ships `harvest-mcp`. host application has hardcoded MCP endpoints. Registry is a future optimization. |
| 4 | ~~Chart rendering library?~~ | — | **Resolved:** Recharts bundled in chat component |
| 5 | ~~Open source or inner-source?~~ | Ed | **Resolved:** Inner-source initially (private repo on `github.com/NetApp/netapp-chat-service`). Must be open-sourced before Harvest integration (Harvest is Apache-2.0 on public GitHub). |
| 6 | ~~Interest catalog — shared repo or per-product?~~ | — | **Resolved:** Per-product. Chat service ships with zero built-in interests. Each product provides its own via mounted volume. host application keeps its current interests embedded; Harvest could reuse them by copying/mounting the same files. |
| 7 | ~~Image registry?~~ | — | **Resolved:** Private repo on GitHub (`github.com/NetApp/netapp-chat-service`, private). Container images via GitHub Container Registry (`ghcr.io/netapp/netapp-chat-service`, private). Goes public when open-sourced. |
| 8 | ~~Helm chart — standalone or sub-chart?~~ | — | **Resolved:** Standalone Helm chart + Docker Compose service definition for Harvest |
| 9 | Built-in UI for Harvest — minimal shell or full SPA? | Ed | Deferred until Harvest integration phase |
| 10 | Action-form standardization across products? | Ed | Deferred — use current implementation for extraction. May align to NetApp UX standards later. |
