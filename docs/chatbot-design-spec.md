# host application Chatbot Design Specification

**Status:** Draft  
**Author:** Auto-generated from design session  
**Date:** 2026-03-01  
**Reference:** [NetApp Console Agentic Features PRD - 26H1](https://netapp.atlassian.net/wiki/spaces/UMF/pages/396463414)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [MCP Server Deployment](#3-mcp-server-deployment)
4. [BYO LLM Configuration](#4-byo-llm-configuration)
5. [Chatbot Backend API](#5-chatbot-backend-api)
6. [Chatbot Frontend UI](#6-chatbot-frontend-ui)
7. [Capability Controls](#7-capability-controls)
8. [Security & Auth](#8-security--auth)
9. [Testing Strategy](#9-testing-strategy)
10. [Phasing & Implementation Plan](#10-phasing--implementation-plan)
11. [Open Questions](#11-open-questions)

---

## 1. Overview

### 1.1 Goal

Add a natural language chatbot to host application that gives storage administrators AI-powered access to their infrastructure via MCP (Model Context Protocol) servers. The chatbot mirrors the capabilities defined in the Console Agentic PRD but is adapted for host application's on-premises appliance architecture.

### 1.2 Scope

| In Scope | Out of Scope |
|----------|-------------|
| Chatbot side panel UI in frontend | Autonomous agents (Provisioning, Search, Alert Resolution) |
| BYO LLM configuration (OpenAI, Anthropic, Bedrock) | Agent hosting infrastructure |
| Harvest MCP integration (already deployed) | KnowledgeBase MCP |
| ONTAP MCP deployment & integration | A2A protocol / external chatbot integration |
| Grafana MCP deployment & integration | Cross-session conversation persistence |
| Per-MCP capability controls (Off/Ask/Allow) | RBAC beyond host application's existing admin user model |
| Read-only / Read-write mode toggle | LLM proxy container (future) |
| Action confirmation for write operations | File/attachment uploads |
| Audit trail for all chatbot interactions | |

### 1.3 Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Chatbot UI location | Side panel (drawer) | Global access from any page, matches Console PRD pattern, fits Mantine AppShell |
| LLM connectivity | Direct from chat-service | Simpler architecture, fewer containers, sufficient for on-prem single-tenant |
| MCP transport | All HTTP containers | Consistent with existing Harvest MCP pattern, container isolation, Caddy routing |
| Permission model | Per-MCP token scopes | Extends existing host application scoped token system, 1:1 capability-to-MCP mapping |
| MCP client library | `modelcontextprotocol/go-sdk` (v1.4+) | Official SDK, same library Harvest MCP and ONTAP MCP are built on. Stable semver (v1.x), `slog` native, Go 1.24 |
| LLM client libraries | `openai/openai-go` + `anthropics/anthropic-sdk-go` | Two official SDKs behind a thin `Provider` interface. OpenAI SDK covers OpenAI + Azure. Anthropic SDK covers Anthropic + Bedrock (built-in). Both support streaming + tool calling. |
| ONTAP MCP config | Shared `harvest.yml` (read-only mount) | Same `Pollers:` format. No separate credential management needed. |
| Grafana SA provisioning | Runtime auto-provision via Grafana HTTP API | Grafana YAML provisioning doesn't support service accounts. chat-service already has the `pkg/grafana` code. Create `Viewer`-role SA on MCP enable. |
| Compose profile strategy | Keep single `mcp` profile, per-container up/down | No migration needed. All MCP containers share `profiles: ["mcp"]`. Individual enable/disable via `UpContainer`/`DownContainer`. |
| Context window | Sliding window (last N messages) | Simple, predictable. Oldest messages silently drop off. |
| Working state UX | Streaming text + collapsible tool status cards | User sees LLM tokens as they arrive + inline status cards for tool calls. |
| Chat rendering | Full GFM markdown + syntax highlighting + base64 images | Needed for Grafana panel images and readable LLM output. |
| Session persistence | In-memory only (no persistence) | Cleared on refresh. Simplest approach for single-admin appliance. |

### 1.4 Relationship to Console PRD

host application's chatbot is a **subset** of the Console Agentic PRD, adapted for an on-prem appliance:

| Console PRD Feature | host application Equivalent |
|--------------------|-----------------|
| LLM Gateway/Proxy (§3.2.1) | Direct LLM connection from chat-service |
| BYO LLM Configuration (§3.2.2) | Settings → AI Configuration page |
| Harvest MCP Deployment (§3.3.2) | Already deployed (v4.1.0) |
| ONTAP MCP Deployment (§3.3.1) | New: container in compose stack |
| Console Chatbot (§3.3.7) | New: side panel chatbot in frontend |
| Per-capability Off/Ask/Allow (§3.3.7) | Per-MCP Off/Ask/Allow controls |

---

## 2. Architecture

### 2.1 System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  host application Appliance                                                │
│                                                                 │
│  ┌──────────────┐     ┌──────────────────────────────────────┐ │
│  │  frontend     │     │  chat-service                              │ │
│  │  (React)      │────▶│  /chat/*    (new)            │ │
│  │               │     │  /config (host-integration)*      (new)            │ │
│  │  Chatbot      │     │  /preferences (existing)     │ │
│  │  Side Panel   │     │                                      │ │
│  └──────────────┘     │  ┌──────────────────────────────┐    │ │
│                        │  │  MCP Client (Go)              │    │ │
│                        │  │  • Connects to MCP servers    │    │ │
│                        │  │  • Routes tool calls          │    │ │
│                        │  │  • Enforces capabilities      │    │ │
│                        │  └──────────┬───────────────────┘    │ │
│                        │             │                         │ │
│                        │  ┌──────────┴───────────────────┐    │ │
│                        │  │  LLM Client (Go)              │    │ │
│                        │  │  • OpenAI / Anthropic / Bedrock│   │ │
│                        │  │  • Streaming responses         │    │ │
│                        │  │  • Tool call orchestration     │    │ │
│                        │  └──────────┬───────────────────┘    │ │
│                        └─────────────┼────────────────────────┘ │
│                                      │                          │
│         ┌────────────────────────────┼────────────────────┐     │
│         │                            │                    │     │
│         ▼                            ▼                    ▼     │
│  ┌─────────────┐  ┌──────────────────────┐  ┌──────────────┐  │
│  │ harvest-mcp │  │    ontap-mcp         │  │ grafana-mcp  │  │
│  │ :8082       │  │    :8084             │  │ :8085        │  │
│  │ (existing)  │  │    (new)             │  │ (new)        │  │
│  └──────┬──────┘  └──────────┬───────────┘  └──────┬───────┘  │
│         │                    │                      │          │
│         ▼                    ▼                      ▼          │
│  ┌─────────────┐   ┌──────────────┐        ┌─────────────┐   │
│  │ Victoria    │   │ ONTAP        │        │ Grafana     │   │
│  │ Metrics     │   │ Clusters     │        │ :3000       │   │
│  │ :8428       │   │ (customer)   │        │             │   │
│  └─────────────┘   └──────────────┘        └─────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS (outbound)
                              ▼
                    ┌──────────────────┐
                    │ Customer LLM     │
                    │ (OpenAI /        │
                    │  Anthropic /     │
                    │  Bedrock)        │
                    └──────────────────┘
```

### 2.2 Agent Loop Architecture

The chat-service chatbot backend is an **agentic tool-use loop** — not a simple prompt-response proxy. It discovers tools from MCP servers, presents them to the LLM, executes tool calls, and iterates until the LLM produces a final response.

**Lifecycle:**

```
                              ┌──────────────────────┐
                              │ User sends message    │
                              └──────────┬───────────┘
                                         ▼
                              ┌──────────────────────┐
                              │ Build conversation    │
                              │ context:              │
                              │ • System prompt       │
                              │ • Tool definitions    │
                              │   (from MCP discovery)│
                              │ • Message history     │
                              │   (sliding window)    │
                              └──────────┬───────────┘
                                         ▼
                          ┌─────────────────────────────┐
                     ┌───▶│ Send to LLM (streaming)     │
                     │    └──────────┬──────────────────┘
                     │               ▼
                     │    ┌──────────────────────────┐
                     │    │ LLM response chunk?       │
                     │    └──────────┬───────────────┘
                     │               │
                     │       ┌───────┴───────┐
                     │       ▼               ▼
                     │  ┌─────────┐   ┌────────────┐
                     │  │ Text    │   │ Tool call  │
                     │  │ tokens  │   │ request    │
                     │  └────┬────┘   └─────┬──────┘
                     │       │              │
                     │       ▼              ▼
                     │  Stream to     ┌─────────────┐
                     │  frontend      │ Check caps:  │
                     │  via SSE       │ Off → block  │
                     │                │ Ask → pause  │
                     │                │ Allow → exec │
                     │                └──────┬──────┘
                     │                       ▼
                     │                ┌─────────────┐
                     │                │ Execute tool │
                     │                │ via MCP HTTP │
                     │                └──────┬──────┘
                     │                       │
                     └───────────────────────┘
                     (append tool result, loop)
                                         │
                              ┌──────────▼───────────┐
                              │ LLM returns final     │
                              │ text (no more calls)  │
                              └──────────┬───────────┘
                                         ▼
                              ┌──────────────────────┐
                              │ SSE: event: done      │
                              └──────────────────────┘
```

**Key decisions:**

| Aspect | Choice |
|--------|--------|
| Tool registry | MCP `tools/list` protocol — dynamic discovery, cached in memory |
| Tool refresh | On MCP enable/disable, on chatbot open, periodic (5 min) |
| Max iterations | Hard limit per turn (e.g. 10 tool calls) to prevent runaway loops |
| Parallel tools | If LLM returns multiple tool calls in one response, execute sequentially (simpler) or concurrently (faster). Start with sequential. |
| Error handling | Tool execution failure → error result sent back to LLM → LLM explains the failure to user |
| MCP down | If an MCP server is unreachable, its tools are removed from the tool list; LLM informed via system prompt |

**There is no separate agent framework** — it's a straightforward Go implementation of the tool-use loop. The MCP protocol *is* the tool registry interface. Each MCP server registers itself by responding to `tools/list`.

### 2.3 Request Flow (Detailed)

1. User types message in chatbot side panel
2. Frontend sends `POST /chat/message` to chat-service
3. chat-service builds context: system prompt + filtered tool definitions + conversation history (sliding window)
4. chat-service's **LLM Client** streams the request to the customer's LLM provider
5. LLM streams back text tokens → chat-service forwards as `event: message` SSE events
6. LLM emits a tool call → chat-service sends `event: tool_call` SSE, then:
   a. Checks capability state (Off → block + tell LLM; Ask → pause + send `event: tool_approval_required` SSE; Allow → execute)
   b. **MCP Client** invokes the tool via HTTP on the appropriate MCP container
   c. Sends `event: tool_result` SSE to frontend
   d. Appends tool result to conversation, loops back to step 4
7. LLM returns final text (no tool calls) → chat-service sends `event: done` SSE
8. Conversation history updated in memory (sliding window manages old messages)

### 2.4 Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **frontend** (Chatbot Panel) | Conversation UI, mode toggles, capability controls, action confirmation dialogs, SSE streaming display |
| **chat-service** (Chat API) | Session management, LLM orchestration, MCP tool routing, capability enforcement, audit logging |
| **chat-service** (AI Config API) | LLM provider CRUD, connection test, credential storage |
| **chat-service** (MCP Client) | HTTP connections to MCP containers, tool discovery, tool invocation |
| **MCP containers** | Domain-specific tool execution (metrics queries, ONTAP operations, Grafana dashboards) |

---

## 3. MCP Server Deployment

### 3.1 Current State: Harvest MCP

Already deployed in host application v4.1.0:

| Aspect | Value |
|--------|-------|
| Image | `ghcr.io/netapp/harvest-mcp:nightly` |
| Container name | `harvest-mcp` |
| Port | 8082 (HTTP, internal) |
| Compose profile | `mcp` |
| Command | `start --http --host 0.0.0.0 --port 8082` |
| Backend | VictoriaMetrics via `HARVEST_TSDB_URL` env var |
| External URL | `http://host application/mcp/` (plain HTTP, Caddy reverse proxy) |
| Auth | Scoped token with `Harvest-MCP` scope via Caddy `forward_auth` |
| Enable/disable | Admin UI Preferences toggle → `COMPOSE_PROFILES` + `UpContainer`/`DownContainer` |

### 3.2 New: ONTAP MCP

| Aspect | Value |
|--------|-------|
| Image | `ghcr.io/netapp/ontap-mcp:latest` |
| Container name | `ontap-mcp` |
| Port | 8084 (HTTP, internal) |
| Compose profile | `mcp` (shared with harvest-mcp) |
| Command | `start --port 8084 --host 0.0.0.0` |
| Backend | Customer ONTAP clusters (credentials from shared `harvest.yml`) |
| External URL | `http://host application/mcp/ontap/` (plain HTTP, Caddy reverse proxy) |
| Auth | Scoped token with `ONTAP-MCP` scope |

**Shared Configuration with Harvest:**

The ONTAP MCP config format is intentionally identical to Harvest's `harvest.yml`. Both use a top-level `Pollers:` map with the same field names:

```yaml
Pollers:
  cluster1:
    addr: 10.0.0.1
    username: admin
    password: password
    use_insecure_tls: true
```

| Field | harvest.yml | ontap.yaml | Shared? |
|-------|-------------|------------|---------|
| `Pollers:` (top-level map) | ✅ | ✅ | Identical |
| `addr` | ✅ | ✅ | Identical |
| `username` | ✅ | ✅ | Identical |
| `password` | ✅ | ✅ | Identical |
| `use_insecure_tls` | ✅ | ✅ | Identical |
| `collectors`, `exporters`, `datacenter`, ... | ✅ | ❌ | Harvest-only (ignored by ONTAP MCP) |

**Design: Mount `harvest.yml` directly into the ONTAP MCP container.** No separate config file or credential management UI needed — ONTAP MCP reads the `Pollers:` section and ignores Harvest-specific fields. Cluster credentials are already managed via the existing harvest-proxy admin UI.

> **Note:** `harvest.yml` may contain non-ONTAP pollers (StorageGRID, CiscoSwitch). The ONTAP MCP server will attempt to connect to them and fail gracefully, or can be configured to filter by type if it supports that.

**Compose definition (new):**
```yaml
ontap-mcp:
  image: ontap-mcp:latest
  container_name: ontap-mcp
  hostname: ontap-mcp
  profiles:
    - "mcp"
  volumes:
    - harvest-conf:/opt/mcp:ro
  env_file:
    - .env
    - .env.custom
  command: ["start", "--port", "8084", "--host", "0.0.0.0"]
```

The `harvest-conf` volume (which contains `harvest.yml`) is already shared with the `harvest-proxy` container. Mounting it read-only into `ontap-mcp` at `/opt/mcp` (the default config path) means zero additional configuration.

### 3.3 New: Grafana MCP

| Aspect | Value |
|--------|-------|
| Image | `grafana/mcp-grafana:latest` |
| Container name | `grafana-mcp` |
| Port | 8085 (HTTP, internal) |
| Compose profile | `mcp` (shared) |
| Command | `-t streamable-http --address 0.0.0.0:8085 --disable-write` (default read-only) |
| Backend | Local Grafana instance at `http://grafana:3000` |
| External URL | `http://host application/mcp/grafana/` (plain HTTP, Caddy reverse proxy) |
| Auth | Scoped token with `Grafana-MCP` scope |

**Grafana MCP Specifics:**
- Connects to host application's bundled Grafana instance (internal network, no external auth needed)
- Requires a Grafana service account token (`GRAFANA_SERVICE_ACCOUNT_TOKEN`)
- Has built-in `--disable-write` flag for read-only mode
- Extensive tool categories with per-category `--disable-<category>` flags
- In host application context, primary value is dashboard search, Prometheus querying, alert rule viewing, and panel image rendering

**Compose definition (new):**
```yaml
grafana-mcp:
  image: grafana-mcp:latest
  container_name: grafana-mcp
  hostname: grafana-mcp
  profiles:
    - "mcp"
  depends_on:
    grafana:
      condition: service_healthy
  environment:
    - GRAFANA_URL=http://grafana:3000
    - GRAFANA_SERVICE_ACCOUNT_TOKEN=${GRAFANA_MCP_TOKEN}
  command: ["-t", "streamable-http", "--address", "0.0.0.0:8085", "--disable-write"]
```

### 3.4 MCP Lifecycle Management

All three MCP containers share the `mcp` compose profile. The existing toggle mechanism works for the group, but we need **per-MCP enable/disable**:

**Option A: Separate compose profiles** (recommended)
```
COMPOSE_PROFILES=mcp-harvest,mcp-ontap,mcp-grafana
```
Each MCP gets its own profile. The Preferences UI shows per-MCP toggles. Migration from `mcp` → individual profiles needed.

**Option B: Keep shared profile, control at chat-service level**
All MCPs start with `mcp` profile. chat-service selectively connects to only the enabled ones. Simpler compose but wastes resources on unused containers.

**Recommendation:** Option A — separate profiles. This is consistent with Docker Compose best practices and avoids running unnecessary containers.

### 3.5 Caddy Routing Updates

Add routes for the new MCP servers alongside the existing Harvest MCP route:

```
http:// {
    # Existing: Harvest MCP
    redir /mcp /mcp/
    handle /mcp/* {
        import auth "Harvest-MCP"
        handle_path /mcp/* {
            reverse_proxy harvest-mcp:8082
        }
    }

    # New: ONTAP MCP
    redir /mcp/ontap /mcp/ontap/
    handle /mcp/ontap/* {
        import auth "ONTAP-MCP"
        handle_path /mcp/ontap/* {
            reverse_proxy ontap-mcp:8084
        }
    }

    # New: Grafana MCP
    redir /mcp/grafana /mcp/grafana/
    handle /mcp/grafana/* {
        import auth "Grafana-MCP"
        handle_path /mcp/grafana/* {
            reverse_proxy grafana-mcp:8085
        }
    }
}
```

> **Note:** External MCP client access (via Caddy routes) is separate from the chatbot's internal access. The chatbot in chat-service connects to MCP containers directly on the Docker network (e.g., `http://harvest-mcp:8082`). The Caddy routes are for external MCP clients (e.g., Claude Desktop, VS Code).

### 3.6 Image Pull Strategy

| Image | Current Strategy | Notes |
|-------|-----------------|-------|
| `harvest-mcp` | Pulled at build time from `ghcr.io/netapp/harvest-mcp:nightly` | Version pinned in `build/Taskfile.yml` |
| `ontap-mcp` | **New:** Pull at build time from `ghcr.io/netapp/ontap-mcp:latest` | Pin to release tag when available |
| `grafana-mcp` | **New:** Pull at build time from `grafana/mcp-grafana:v0.11.2` | Pin to stable release |

Images are bundled into the OVA/QCOW2 during build. No runtime image pulls needed.

---

## 4. BYO LLM Configuration

### 4.1 AI Configuration Settings Page

New settings page at `/settings/ai` in the frontend navbar (under Settings).

**Mirrors Console PRD §3.2.2 — LLM Configuration Dialog:**

| Field | Required | Description |
|-------|----------|-------------|
| **LLM Provider** | Yes | Dropdown: OpenAI, Anthropic, AWS Bedrock, Custom (OpenAI-compatible) |
| **Endpoint URL** | Yes | Pre-populated for known providers, editable for custom |
| **API Key** | Yes | Masked after entry, stored encrypted in chat-service's credential store |
| **Model Name** | Yes | Dropdown for known providers (e.g., `gpt-4.1`, `claude-sonnet-4`), free-text for custom |
| **AWS Region** | Conditional | Required for AWS Bedrock only |
| **AWS Access Key ID** | Conditional | Required for AWS Bedrock (access key auth) |
| **AWS Secret Access Key** | Conditional | Required for AWS Bedrock (access key auth) |

**Connection Status States:**
- ⚠️ **Not Configured** — Agentic features disabled
- 🔄 **Testing...** — Validating connection
- ✅ **Connected** — Last verified: [date], Model: [model]
- ❌ **Connection Failed** — Error details + [Retry]

**Test Connection** button validates:
1. Endpoint reachability
2. API key validity
3. Model availability
4. Basic inference capability (simple prompt/response round-trip)

### 4.2 Backend API

**New endpoints under `/config (host-integration)`:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/ai/config` | Get current LLM configuration (credentials masked) |
| `POST` | `/ai/config` | Save LLM configuration |
| `POST` | `/ai/test` | Test LLM connection |
| `DELETE` | `/ai/config` | Remove LLM configuration, disable agentic features |

**Configuration storage:** YAML file at `/etc/host application/ai.yaml` (or embedded in existing `.env.custom`), with API key encrypted using chat-service's existing credential approach.

### 4.3 LLM Client Implementation (chat-service)

New Go package `internal/llm` in chat-service:

- **Provider abstraction:** Interface supporting OpenAI, Anthropic, and Bedrock APIs
- **Streaming:** Server-Sent Events for streaming LLM responses to the frontend
- **Tool calling:** Translate MCP tool definitions into LLM function/tool call format
- **Error handling:** Graceful rate limit handling (HTTP 429) with backoff/retry and user-facing messaging
- **No proxy:** Direct HTTPS connection to the customer's LLM endpoint from chat-service

---

## 5. Chatbot Backend API

### 5.1 Chat Endpoints

**New endpoints under `/chat/`:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/chat/message` | Send a message, receive streaming SSE response |
| `GET` | `/chat/capabilities` | Get available MCP capabilities and their states |
| `POST` | `/chat/capabilities` | Update capability states (Off/Ask/Allow per MCP) |
| `POST` | `/chat/approve` | Approve a pending tool call (Ask mode) |
| `POST` | `/chat/deny` | Deny a pending tool call (Ask mode) |
| `POST` | `/chat/stop` | Stop an in-progress multi-step operation |
| `DELETE` | `/chat/session` | Clear current conversation session |

### 5.2 Message Request/Response

**Request (`POST /chat/message`):**
```json
{
  "message": "What volumes are running low on space?",
  "mode": "read-only",
  "session_id": "optional-session-id"
}
```

**Response (SSE stream):**
```
event: message
data: {"type": "text", "content": "Let me check your volume capacity..."}

event: tool_call
data: {"type": "tool_call", "capability": "harvest", "tool": "metrics_query", "params": {...}, "status": "executing"}

event: tool_result  
data: {"type": "tool_result", "capability": "harvest", "tool": "metrics_query", "result": {...}}

event: message
data: {"type": "text", "content": "Here are the volumes with less than 20% free space:\n\n| Volume | ..."}

event: done
data: {"type": "done", "session_id": "abc123"}
```

**Ask-mode approval flow:**
```
event: tool_approval_required
data: {"type": "tool_approval_required", "approval_id": "xyz", "capability": "ontap", "tool": "volume_resize", "params": {"volume": "vol1", "size": "100GB"}, "description": "Resize volume vol1 to 100GB"}
```
Frontend shows approval dialog → user clicks Allow → `POST /chat/approve {"approval_id": "xyz"}` → execution continues.

### 5.3 Session Management

- **In-memory:** Conversation history held in chat-service process memory, keyed by session ID
- **Session-scoped:** Cleared on browser refresh, explicit clear, or chat-service restart
- **No persistence:** No database or file storage for conversation history
- **Sliding window:** chat-service keeps the last N messages (e.g. 20-40 user/assistant turns) in context. Older messages silently drop off. Tool call/result pairs count as one turn. The window size is tuned per LLM provider based on their context window size.
- **Token counting:** Approximate token count tracked per message. When approaching the provider's context limit, oldest messages are dropped first. System prompt + tool definitions are always included (they take priority over history).

### 5.4 MCP Client Implementation (chat-service)

New Go package `internal/mcp` in chat-service:

- **HTTP transport:** Connects to MCP containers on internal Docker network
- **Tool discovery:** On startup (and periodically), queries each enabled MCP server for its tool list
- **Tool schema:** Caches tool names, descriptions, and JSON schemas for LLM function calling
- **Tool invocation:** Executes tool calls and returns results
- **Health checks:** Monitors MCP container availability

### 5.5 System Prompt

chat-service constructs a system prompt that includes:
1. host application context (appliance role, storage monitoring purpose)
2. Available capabilities and their tools (dynamically built from MCP tool discovery)
3. Read-only vs read-write mode context
4. Response formatting guidelines (Markdown, tables)
5. Safety instructions (confirm destructive operations, explain reasoning)

---

## 6. Chatbot Frontend UI

### 6.1 Side Panel (Drawer)

**Implementation:** Mantine `Drawer` component, anchored to the left side of the AppShell.

**Panel Structure:**
```
┌─────────────────────────────────────────────┐
│ 🤖 host application Assistant        [⚙️] [─] [×]     │
│ Model: gpt-4.1 ✅                           │
├─────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────┐ │
│ │ 🔒 Read-Only  ◉───○  ✏️ Read/Write      │ │
│ └─────────────────────────────────────────┘ │
├─────────────────────────────────────────────┤
│                                             │
│  Suggested prompts:                         │
│  ┌─────────────────────────────────────┐   │
│  │ 📊 What's the health of my fleet?   │   │
│  │ 💾 Show volumes over 80% capacity   │   │
│  │ 📈 Show me my Grafana dashboards    │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  ─── Conversation ───                       │
│                                             │
│  🧑 What volumes are running low?           │
│                                             │
│  🤖 Let me check...                        │
│     ⚡ harvest → metrics_query              │
│                                             │
│     Here are volumes < 20% free:            │
│     ┌────────┬──────┬───────┐              │
│     │ Volume │ Used │ Free  │              │
│     ├────────┼──────┼───────┤              │
│     │ vol1   │ 95%  │ 5%   │              │
│     │ vol2   │ 88%  │ 12%  │              │
│     └────────┴──────┴───────┘              │
│                                             │
│     Follow-ups:                             │
│     [Resize vol1] [Show growth trend]       │
│                                             │
├─────────────────────────────────────────────┤
│ [Type a message...                    ] [▶] │
└─────────────────────────────────────────────┘
```

### 6.2 Panel Controls

| Control | Behavior |
|---------|----------|
| **Open/Close** | Global button in AppShell header + keyboard shortcut (Cmd+Shift+L) |
| **Resize** | Draggable left edge (min 350px, max 60% viewport) |
| **Minimize** | Collapse to icon in header, preserving conversation |
| **Settings (⚙️)** | Opens capability controls popover |
| **Clear** | Clears conversation, resets session |

### 6.3 Display & Rendering

**Markdown rendering (full):** Use `react-markdown` with `remark-gfm` plugin for GitHub-Flavored Markdown:
- **Tables:** Rendered via GFM tables → styled with Mantine `Table` CSS
- **Code blocks:** Syntax-highlighted (e.g. `react-syntax-highlighter` or Mantine `Code`)
- **Inline images:** Base64 images (from Grafana MCP `get_panel_image`) rendered as `<img>` tags with click-to-expand
- **Lists, headings, bold/italic, links:** Standard GFM rendering
- **Copy button:** Per-response copy to clipboard (copies raw markdown)

**Streaming & working state:**
- **Text tokens:** Appear incrementally as `event: message` SSE events arrive. Markdown is re-rendered progressively.
- **Tool call status:** Displayed as collapsible inline status cards during execution:
  ```
  ⚡ harvest → metrics_query  [executing...]
  └─ Queried: volume_capacity_used_percent > 80
  ```
  Cards show: capability badge → tool name → status (executing/completed/failed). On completion, the card collapses to a single line. Expandable to show parameters and result preview.
- **Typing indicator:** Shown when waiting for LLM response before any tokens arrive
- **STOP button:** Appears during execution, sends `POST /chat/stop`

**Suggested prompts:** Clickable chips, shown on first panel open and as contextual follow-ups after responses.

### 6.4 Mode Toggle

**Read-Only Mode (default):**
- Information retrieval only
- Write tools filtered out of LLM context
- Toggle shown prominently at top of panel

**Read/Write Mode (explicit enable):**
- All tools available
- Write operations require confirmation dialog
- **Auto-disable timer:** Reverts to read-only after 10 minutes of inactivity
- Timer display visible when active

### 6.5 Action Confirmation Dialog

When a write operation is invoked (in Read/Write + Ask mode or always for destructive ops):

```
┌─────────────────────────────────────────────┐
│ ⚠️  Confirm Action                          │
├─────────────────────────────────────────────┤
│                                             │
│  Operation: Resize Volume                   │
│  Capability: ONTAP MCP                      │
│  Tool: volume_resize                        │
│                                             │
│  Parameters:                                │
│  ┌─────────────────────────────────────┐   │
│  │ Volume:  vol1                        │   │
│  │ SVM:     svm1                        │   │
│  │ Cluster: cluster1                    │   │
│  │ Size:    100GB  [editable]           │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  CLI equivalent:                            │
│  volume size -vserver svm1 -volume vol1     │
│    -size 100GB                  [📋 Copy]   │
│                                             │
│                    [Cancel]  [Execute]       │
└─────────────────────────────────────────────┘
```

### 6.6 Stop Control

During multi-step or long-running operations:
- **STOP button** becomes prominent in the input area
- Sends `POST /chat/stop` to halt further tool calls
- Current atomic operation may complete
- Displays summary: completed vs. pending steps

### 6.7 Error States

| State | Display |
|-------|---------|
| LLM not configured | Banner: "Configure AI provider in Settings → AI to enable the assistant" + link |
| LLM connection error | Banner: "Unable to reach LLM. Check Settings → AI." + [Retry] |
| Rate limited | "Rate limited. Retrying in {n} seconds..." + [Cancel] + [Retry Now] |
| MCP unavailable | Inline: "⚠️ {Capability} is currently unavailable" — chatbot continues with remaining capabilities |
| Context length exceeded | "Response exceeded maximum length. [Start New Chat]" |

---

## 7. Capability Controls

### 7.1 Capability-to-MCP Mapping

Each capability maps 1:1 to an MCP server and has an independent autonomy state:

| Capability | MCP Server | Default State | Description |
|------------|-----------|---------------|-------------|
| **Harvest** | harvest-mcp | Ask | Infrastructure metrics, health, capacity analysis |
| **ONTAP** | ontap-mcp | Ask | Volume lifecycle, snapshots, data protection, multi-cluster mgmt |
| **Grafana** | grafana-mcp | Ask | Dashboard search, Prometheus queries, alert rules, panel images |

### 7.2 Per-Capability Autonomy States

Matches Console PRD §3.3.7:

| State | Behavior |
|-------|----------|
| **Off** | Capability disabled entirely. Tools not included in LLM context. If the LLM would benefit from this capability, chatbot informs user: "I could help with that if the {Capability} capability were enabled." |
| **Ask** (default) | Every tool invocation prompts user for permission before executing. Shows capability name, tool name, parameters, and [Allow] / [Deny] buttons. |
| **Allow** | Tools execute autonomously. Each invocation logged inline in conversation: `⚡ harvest → metrics_query`. |

### 7.3 Controls UI

Accessed via the ⚙️ button in the chatbot panel header. Renders as a Mantine `Popover` or collapsible section:

```
┌─────────────────────────────────────────┐
│ Capabilities                            │
├─────────────────────────────────────────┤
│ Harvest (Metrics & Health)              │
│   [Off] [Ask ●] [Allow]                │
│                                         │
│ ONTAP (Storage Management)              │
│   [Off] [Ask ●] [Allow]                │
│                                         │
│ Grafana (Dashboards & Alerts)           │
│   [Off] [Ask ●] [Allow]                │
└─────────────────────────────────────────┘
```

Disabled MCPs (not enabled in Preferences) are shown grayed out with a note: "Enable in Settings → Preferences".

### 7.4 Backend: Capability State API

```
GET  /chat/capabilities
POST /chat/capabilities
```

**Response:**
```json
{
  "capabilities": [
    {
      "id": "harvest",
      "name": "Harvest",
      "description": "Infrastructure metrics, health monitoring, capacity analysis",
      "state": "ask",
      "available": true,
      "tools_count": 12
    },
    {
      "id": "ontap",
      "name": "ONTAP",
      "description": "Volume lifecycle, snapshots, data protection",
      "state": "ask",
      "available": true,
      "tools_count": 25
    },
    {
      "id": "grafana",
      "name": "Grafana",
      "description": "Dashboard search, Prometheus queries, alert rules",
      "state": "off",
      "available": false,
      "tools_count": 0
    }
  ]
}
```

`available: false` means the MCP container is not running (not enabled in Preferences).

---

## 8. Security & Auth

### 8.1 Token Scopes

Extend the existing `scopeValues` in Security-Tokens with new scopes:

| Scope | Purpose |
|-------|---------|
| `Harvest-MCP` | **Existing** — external MCP client access to Harvest MCP |
| `ONTAP-MCP` | **New** — external MCP client access to ONTAP MCP |
| `Grafana-MCP` | **New** — external MCP client access to Grafana MCP |

> **Note:** The chatbot itself is authenticated via the user's existing JWT session (same as all frontend API calls). The scoped tokens above are for external MCP client access (e.g., Claude Desktop connecting to `http://host application/mcp/ontap/`). The chatbot does not use scoped tokens — it connects to MCP containers on the internal Docker network.

### 8.2 Read-Only Enforcement and Tool Budget

When the chatbot is in read-only mode:
1. chat-service filters the tool list sent to the LLM, excluding tools with write/destructive annotations
2. If an MCP provides tool annotations (`readOnlyHint`, `destructiveHint`), chat-service uses those — the values are propagated through `mcpclient.convertTool` into `llm.ToolDef.ReadOnlyHint` / `DestructiveHint`
3. If not annotated, chat-service maintains a per-server allowlist via `mcp_servers[].read_only_tools` in `config.yaml`. Tools listed there are treated as read-only even if the MCP doesn't publish annotations (e.g. Grafana, third-party MCPs)
4. Tools that are neither annotated nor on the allowlist default to **write-capable** and are filtered out in read-only mode. Each unannotated tool produces a debug log entry so operators can extend the allowlist
5. Internal tools registered via `agent.InternalTool` use the explicit `ReadWriteOnly` flag instead of annotations
6. Grafana MCP has native `--disable-write` support — we run it in read-only mode by default

#### Tool Budget (Hard Cap)

Azure OpenAI rejects requests with more than 128 tools (HTTP 400), and OpenAI accuracy degrades sharply above ~20 tools per turn. To prevent runtime failures and improve LLM quality, chat-service enforces a hard cap of `agent.MaxToolsPerRequest` (= 128) tools per request:

- **Pre-LLM gate:** `(*Agent).filteredTools()` returns `agent.ErrTooManyTools` when the assembled list exceeds the cap. The agent loop emits an `EventError` (mentioning the 128 limit and how to fix) followed by `EventDone` so the SSE stream closes cleanly. The LLM is never called.
- **API gate:** `POST /chat/capabilities` validates the proposed capability set against the cap before mutating state. If the change would push usage over the budget, it returns **HTTP 409 Conflict** with the structured payload `{message, tool_budget: {used, max, mode, per_capability}}` and leaves state untouched.
- **Mode gate:** the frontend pre-checks `tool_budgets.read_write` before allowing a mode switch into read-write. If the switch would exceed the cap, the UI displays a blocker error instead of issuing the change.
- **API surface:** `GET /chat/capabilities` returns per-capability `tools_count` + `read_only_tools_count`, plus a top-level `tool_budget` (for the requested `mode`) and `tool_budgets` (a dual `{read_only, read_write}` preview) so the UI can render the budget bar and predict the impact of toggles without extra round-trips.
- **UI:** `CapabilityControls` shows a tool-budget progress bar (blue → yellow at 80% → red over budget), per-capability badges (`N tools (M ro)`), and disables Ask/Allow on toggles whose tools alone would exceed the cap.

### 8.3 ONTAP Cluster Credentials

ONTAP MCP needs cluster credentials. Storage model:
- Config file (`/etc/host application/ontap-mcp/ontap.yaml`) mounted into the container
- Contains cluster list with hostnames, usernames, passwords
- Managed via a new frontend settings page or the existing ONTAP page
- Passwords encrypted at rest using chat-service's credential store

### 8.4 Grafana Service Account Token

Grafana MCP needs a service account token. Provisioning strategy:
- On first MCP enable, chat-service auto-creates a Grafana service account with appropriate role
- Token stored in `.env.custom` as `GRAFANA_MCP_TOKEN`
- Alternatively, let user configure via Settings

### 8.5 Audit Trail

All chatbot interactions are logged:
- User query text
- LLM tool calls (capability, tool name, parameters)
- Tool results (success/failure)
- User approvals/denials
- Mode changes (read-only ↔ read-write)

Log destination: chat-service's structured `slog` output (same as existing audit logging). Future: export via the log infrastructure.

## 9. Testing Strategy

host application's current test coverage is thin. The chatbot feature introduces significant new complexity (LLM integration, MCP orchestration, streaming UI, agent loop) that demands a proper test harness across all layers.

### 9.1 Current State

| Layer | What Exists | Gap |
|-------|------------|-----|
| Go backend | 23 `_test.go` files (harvest-proxy conf/netapp/server, chat-service ssl/netconfig/vmware, packages auth, faas) | No tests for most chat-service routes (login, preferences, upgrade, health, system, logs). No test patterns for HTTP streaming or SSE. |
| Frontend unit | Vitest + React Testing Library configured. 1 placeholder test (`Welcome.test.tsx`). | Essentially zero component coverage. |
| E2E | Robot Framework + Browser library (Playwright-based) in `robot/`. Login, Network, ONTAP, Password, StorageGRID suites. | Requires deployed appliance + real ONTAP credentials. Not dev-loop friendly. No chatbot coverage. |
| CI | None visible in repo. | No automated test runs on push/PR. |

### 9.2 Test Architecture

Three layers, each runnable independently:

```
┌──────────────────────────────────────────────────────────┐
│  E2E Tests (Playwright)                                  │
│  Full browser → chat-service → MCP containers                  │
│  • Chatbot conversation flows                            │
│  • LLM config setup                                      │
│  • MCP enable/disable                                    │
│  • Auth flows                                            │
├──────────────────────────────────────────────────────────┤
│  Frontend Component Tests (Vitest + RTL)                 │
│  jsdom, no backend, mocked API                           │
│  • Chatbot panel open/close/resize                       │
│  • Message rendering (markdown, tables, images)          │
│  • Streaming SSE display                                 │
│  • Tool status cards                                     │
│  • Capability controls UI state                          │
│  • Action confirmation dialog                            │
│  • Mode toggle behavior                                  │
├──────────────────────────────────────────────────────────┤
│  Backend Unit Tests (Go testing)                         │
│  No external deps, httptest, mocked MCP/LLM              │
│  • Agent loop (tool-use cycle)                           │
│  • LLM provider abstraction (each provider)              │
│  • MCP client (tool discovery, tool call)                │
│  • Chat API handlers (SSE streaming)                     │
│  • Capability filtering logic                            │
│  • Session management (sliding window)                   │
│  • AI config CRUD                                        │
│  • System prompt construction                            │
└──────────────────────────────────────────────────────────┘
```

### 9.3 Go Backend Tests

**Conventions** (match existing host application patterns):
- Standard `testing` package, table-driven tests with `t.Run`
- `httptest.NewServer` / `httptest.NewRecorder` for HTTP handler tests
- `reflect.DeepEqual` for comparisons (no testify)
- `t.Cleanup()` for teardown

**New test packages:**

| Package | Key Test Scenarios |
|---------|-------------------|
| `internal/llm` | Provider interface contract tests. Mock HTTP server simulating OpenAI/Anthropic streaming responses + tool call responses. Test token counting, error handling (429 retry, invalid key), streaming chunk assembly. |
| `internal/mcp` | Mock MCP server (using `go-sdk`'s `InMemoryTransport` or `httptest`). Test `tools/list` discovery, `tools/call` invocation, connection failure handling, tool cache refresh. |
| `internal/agent` | Agent loop tests with mock LLM + mock MCP. Test: text-only response, single tool call cycle, multi-step tool chain, max iteration limit, tool error recovery, capability filtering (Off/Ask/Allow), read-only mode filtering. |
| `server/server.go` | SSE streaming tests — verify event format (`event: message`, `event: tool_call`, `event: done`), session creation/cleanup, stop endpoint. Uses `httptest.NewRecorder` with SSE flushing. |
| `routes/ai.go` | Config CRUD, connection test endpoint, credential masking in GET response. |

**Mock strategy:**
- **Mock LLM:** `httptest.Server` that returns canned streaming responses (text chunks, tool call JSON). Configurable per test to return different sequences.
- **Mock MCP:** Either `go-sdk`'s `InMemoryTransport` for unit tests, or `httptest.Server` implementing the MCP JSON-RPC protocol for integration tests.
- **No real LLM calls in CI.** Tests use deterministic mock responses.

### 9.4 Frontend Component Tests (Vitest + React Testing Library)

**Already configured:** Vitest, `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, jsdom environment, `@test-utils` path alias.

**Test file convention:** `ComponentName.test.tsx` colocated with the component.

**Key test suites for chatbot:**

| Component | Test Scenarios |
|-----------|---------------|
| `ChatPanel.test.tsx` | Opens/closes drawer, preserves conversation on minimize, clear button resets, keyboard shortcut (Cmd+Shift+L), resize drag |
| `ChatMessage.test.tsx` | Renders markdown (GFM tables, code blocks, bold/italic, links), renders base64 images, copy button copies raw markdown |
| `ChatInput.test.tsx` | Submit on Enter, Shift+Enter for newline, disabled while streaming, character limit |
| `ToolStatusCard.test.tsx` | Shows executing/completed/failed states, expand/collapse, displays tool name + capability badge + params |
| `StreamingMessage.test.tsx` | Progressive markdown rendering from SSE chunks, typing indicator |
| `CapabilityControls.test.tsx` | Off/Ask/Allow toggle per MCP, state changes call API, disabled MCPs greyed out |
| `ActionConfirmation.test.tsx` | Shows tool params, approve/deny buttons, timeout behavior |
| `ModeToggle.test.tsx` | Read-only ↔ Read-write switch, auto-disable timer display, confirmation on enable |
| `AIConfigPage.test.tsx` | Provider dropdown, endpoint/key inputs, test connection button states, save/delete |

**Mock strategy:**
- Mock `APIClient` (Axios) for all backend calls
- Mock SSE with custom `EventSource` polyfill or `msw` (Mock Service Worker)
- Use Mantine's test utilities for component rendering

### 9.5 E2E Tests (Playwright)

**Why Playwright over Robot Framework for new chatbot tests:**
- TypeScript-native — same language as the frontend codebase
- Better dev-loop experience (`npx playwright test --ui` for interactive debugging)
- Built-in network mocking for LLM responses (no real LLM needed)
- Component testing support for isolated Mantine components
- Trace viewer, screenshot comparison, video recording
- Robot Framework remains for existing appliance integration tests (Login, Network, ONTAP, StorageGRID)

**Setup:**

```
chat-service/frontend/
├── e2e/
│   ├── chatbot.spec.ts
│   ├── ai-config.spec.ts
│   ├── mcp-preferences.spec.ts
│   └── fixtures/
│       ├── mock-llm-responses.ts
│       └── mock-mcp-tools.ts
├── playwright.config.ts
└── package.json              (add @playwright/test devDep)
```

**Playwright config:**
```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'yarn run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
  },
});
```

**E2E test scenarios:**

| Test Suite | Scenarios |
|-----------|----------|
| `ai-config.spec.ts` | Configure OpenAI provider → test connection → save. Switch to Anthropic. Delete config. Invalid key error. |
| `chatbot.spec.ts` | Open panel → send message → see streaming response. Tool call status cards appear. Multi-turn conversation. Clear session. Stop mid-response. Mode toggle read-only → read-write. |
| `mcp-preferences.spec.ts` | Enable/disable individual MCPs. Capability controls Off/Ask/Allow. |
| `action-confirm.spec.ts` | Write operation → confirmation dialog → approve → result. Deny → cancelled message. |

**Mock strategy for E2E:**
- **Mock LLM:** Playwright's `page.route()` intercepts outbound LLM API calls from chat-service. Alternatively, chat-service runs with a `--mock-llm` flag (build tag `//go:build e2etest`) that uses a deterministic mock provider.
- **Mock MCP:** Use real MCP containers with test data, OR run chat-service with mock MCP clients.
- **Prefer mocking at the chat-service boundary** (mock LLM + mock MCP) so E2E tests exercise the full frontend → chat-service → SSE pipeline without external dependencies.

### 9.6 Task Runner Integration

**New Taskfile targets:**

```yaml
# Root Taskfile.yml additions
test:
  desc: Run all tests
  cmds:
    - task: test-go
    - task: test-ui
    - task: test-e2e

test-go:
  desc: Run all Go tests across the workspace
  cmds:
    - go test ./...

test-ui:
  desc: Run frontend unit tests
  dir: chat-service/frontend
  cmds:
    - yarn run vitest

test-e2e:
  desc: Run Playwright E2E tests
  dir: chat-service/frontend
  cmds:
    - yarn run playwright test

test-e2e-ui:
  desc: Run Playwright in interactive UI mode
  dir: chat-service/frontend
  cmds:
    - yarn run playwright test --ui
```

**package.json script additions:**
```json
{
  "scripts": {
    "playwright": "playwright test",
    "playwright:ui": "playwright test --ui",
    "playwright:install": "playwright install --with-deps chromium"
  }
}
```

### 9.7 Test Coverage Targets

| Layer | Target | Rationale |
|-------|--------|-----------|
| Go backend (new chatbot packages) | 80%+ | Agent loop, LLM provider, MCP client are critical paths |
| Frontend components (chatbot) | 70%+ | UI rendering, state management, streaming |
| E2E | Key user journeys covered | Not a coverage metric — scenario completeness |

### 9.8 Testing Phase in Implementation

Testing is **not a separate phase** — it's integrated into every phase. Each phase's work items include writing tests alongside the implementation:

- **Phase 1:** Go unit tests for LLM client, MCP client, agent loop, chat API. Vitest tests for ChatPanel, ChatMessage, StreamingMessage, AIConfigPage. Playwright setup + first E2E (config → chat → response).
- **Phase 2:** Tests for capability filtering, approval flow, mode toggle, action confirmation dialog.
- **Phase 3:** Tests for ONTAP MCP tool discovery, multi-MCP routing.
- **Phase 4:** Tests for Grafana SA provisioning, panel image rendering.
- **Phase 5:** Test hardening, coverage reporting, flaky test cleanup.

---

### Phase 1: Foundation (LLM + Harvest Chat)

**Goal:** Chatbot with BYO LLM configuration and Harvest MCP only (read-only).

| Work Item | Component | Description |
|-----------|-----------|-------------|
| AI Configuration page | frontend | Settings → AI page with provider dropdown, endpoint, credentials, test button |
| AI Config API | chat-service | `GET/POST/DELETE /ai/config`, `POST /ai/test` endpoints |
| LLM Client | chat-service | Go package for OpenAI/Anthropic/Bedrock with streaming |
| MCP Client (HTTP) | chat-service | Go MCP client connecting to harvest-mcp over HTTP |
| Chat API | chat-service | `POST /chat/message` with SSE streaming |
| Chatbot Side Panel | frontend | Drawer component, conversation display, streaming, markdown rendering |
| System Prompt | chat-service | Dynamic prompt construction from MCP tool definitions |
| Session Management | chat-service | In-memory conversation state |

### Phase 2: Capabilities & Controls

**Goal:** Per-MCP capability controls, read-write mode, action confirmations.

| Work Item | Component | Description |
|-----------|-----------|-------------|
| Capability Controls UI | frontend | Off/Ask/Allow per capability, popover in chatbot panel |
| Capability API | chat-service | `GET/POST /chat/capabilities` endpoints |
| Ask-mode approval flow | both | `tool_approval_required` SSE event, approval dialog, `POST /chat/approve` |
| Read/Write toggle | frontend | Mode switch with auto-disable timer |
| Write tool filtering | chat-service | Filter tool list based on mode + annotations |
| Action confirmation dialog | frontend | Preview with parameters, CLI equivalent, edit, confirm/cancel |
| Stop control | both | STOP button, `POST /chat/stop`, partial completion summary |
| Suggested prompts | frontend | Starter prompts + contextual follow-ups |

### Phase 3: ONTAP MCP Integration

**Goal:** Deploy ONTAP MCP container using shared Harvest config.

| Work Item | Component | Description |
|-----------|-----------|-------------|
| ONTAP MCP compose definition | build | Container definition, profile, mount `harvest-conf` volume at `/opt/mcp:ro` |
| ONTAP MCP Caddy route | build | `/mcp/ontap/` reverse proxy with `ONTAP-MCP` scope |
| ONTAP MCP image pull | build | Add to `Taskfile.yml` docker-pull |
| ONTAP capability registration | chat-service | Discover ONTAP MCP tools, add to chatbot |
| ONTAP-MCP token scope | chat-service | Add scope for external access |
| Per-MCP compose profiles | build | Migrate from single `mcp` profile to `mcp-harvest`, `mcp-ontap`, `mcp-grafana` |
| Per-MCP Preferences toggles | frontend | Individual enable/disable per MCP server |

> **Simplification:** No separate cluster credential UI or config generation needed — ONTAP MCP reads the existing `harvest.yml` `Pollers:` section directly (same format). Clusters are already managed via the harvest-proxy admin UI.

### Phase 4: Grafana MCP Integration

**Goal:** Deploy Grafana MCP container, enable dashboard/metrics capabilities.

| Work Item | Component | Description |
|-----------|-----------|-------------|
| Grafana MCP compose definition | build | Container definition, profile, Grafana service account |
| Grafana MCP Caddy route | build | `/mcp/grafana/` reverse proxy with `Grafana-MCP` scope |
| Grafana MCP image pull | build | Add to `Taskfile.yml` docker-pull |
| Grafana service account provisioning | chat-service | Auto-create Grafana SA + token on enable |
| Grafana capability registration | chat-service | Discover Grafana MCP tools, add to chatbot |
| Grafana-MCP token scope | chat-service | Add scope for external access |
| Panel image rendering | frontend | Display base64 panel images returned by Grafana MCP in chat |

### Phase 5: Polish & Hardening

| Work Item | Component | Description |
|-----------|-----------|-------------|
| Audit trail | chat-service | Structured logging of all chatbot interactions |
| Error handling | both | Rate limiting, connection errors, context overflow |
| Keyboard shortcuts | frontend | Cmd+Shift+L to toggle panel |
| Responsive design | frontend | Panel behavior on smaller screens |
| Documentation | www | MkDocs pages for chatbot setup, LLM configuration, MCP access |
| Upgrade migration | chat-service | No profile migration needed — `mcp` profile unchanged |

---

## 11. Open Questions

All questions resolved.

| # | Question | Resolution | Status |
|---|----------|------------|--------|
| 1 | Go MCP client library | `modelcontextprotocol/go-sdk` (v1.4+). Official SDK — same library Harvest MCP (v1.3.1) and ONTAP MCP (v1.4.0) are built on. Stable semver, `slog` native, Go 1.24. | **Resolved** |
| 2 | Go LLM client libraries | `openai/openai-go` (v3.24) + `anthropics/anthropic-sdk-go` (v1.26) behind a thin `Provider` interface. Both official, Stainless-generated. Anthropic SDK has built-in Bedrock support (`bedrock` sub-package). | **Resolved** |
| 3 | ONTAP MCP transport flag | CLI: `start --port 8084 --host 0.0.0.0`. HTTP transport is the default. | **Resolved** |
| 4 | ONTAP MCP credential format | Same `Pollers:` format as `harvest.yml`. Mount `${host application_ETC}harvest` at `/opt/mcp:ro`. No separate config needed. | **Resolved** |
| 5 | Grafana SA provisioning | Runtime auto-provision via Grafana HTTP API. chat-service creates a `Viewer`-role SA + token on MCP enable (`POST /api/serviceaccounts`, `POST /api/serviceaccounts/:id/tokens`). Grafana YAML provisioning doesn't support SAs. chat-service already has `pkg/grafana.CreateServiceAccount()` using basic auth. Create a dedicated MCP SA (separate from existing admin SA). Idempotent: search by name before creating. Token stored as `GRAFANA_MCP_TOKEN` env var. | **Resolved** |
| 6 | Per-MCP profile migration | **No migration needed.** Keep single `mcp` profile — all MCP containers share `profiles: ["mcp"]`. Individual enable/disable via `UpContainer`/`DownContainer` targeting specific containers (Docker Compose auto-activates profiles when targeting a service directly). Preferences UI gets per-MCP toggles, each calling `UpContainer("ontap-mcp")` / `DownContainer("ontap-mcp")` etc. Existing `COMPOSE_PROFILES=mcp` continues to work unchanged. | **Resolved** |
| 7 | Grafana MCP images | `get_panel_image` returns base64 PNG. Render inline as `<img>` with click-to-expand. Full GFM markdown rendering via `react-markdown` + `remark-gfm`. | **Resolved** |
| 8 | Capability state persistence | Persist in `/etc/host application/ai.yaml` alongside LLM config. Single admin user, so no per-user distinction needed. Loaded on chat-service startup, saved on change via `POST /chat/capabilities`. | **Resolved** |
| 9 | Context window strategy | Sliding window — keep last N messages (20-40 turns). Oldest drop off silently. Approximate token counting per provider. System prompt + tool definitions always included (priority over history). | **Resolved** |
| 10 | Grafana MCP tool categories | Enable all tools but pass `--disable-write` flag to the Grafana MCP container. This disables all write/mutation tools at the MCP server level. Read tools (dashboards, queries, alerts, rendering) are all useful. The capability control (Off/Ask/Allow) provides the user-facing filter layer. | **Resolved** |
| 11 | Conversation persistence | Session-only (in-memory). Cleared on browser refresh, explicit clear, or chat-service restart. | **Resolved** |
| 12 | Agent loop: max iterations | Default 10 tool calls per user message. Configurable in `ai.yaml`. If limit reached, LLM receives a system message saying "tool call limit reached, please summarize what you have so far" and must produce a text response. | **Resolved** |
| 13 | Agent loop: parallel vs sequential | Start sequential. The agentic loop executes one tool call at a time, feeds result back to LLM, and loops. Simpler to implement, debug, and stream status to the user. Parallel execution can be added later as an optimization if needed. | **Resolved** |
| 14 | Non-ONTAP pollers in harvest.yml | Low risk. ONTAP MCP connects lazily (when user asks about a cluster). Attempting ONTAP REST calls against a StorageGRID address will fail with a connection/API error, which the MCP server handles as a standard error response. The LLM will see the error and exclude that cluster. If this proves problematic, chat-service can generate a filtered config at enable time. | **Resolved** |
