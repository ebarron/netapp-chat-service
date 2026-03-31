# host application Chatbot Architecture

> **Status:** Living document — updated as features ship  
> **Audience:** Engineers working on host application  
> **Scope:** Everything built in the chatbot system: backend, frontend, protocols, type system, interest system, capability controls

---

## 1. Overview

The host application chatbot is an AI-powered storage infrastructure assistant embedded in the host application admin UI. It connects to NetApp monitoring data via MCP (Model Context Protocol) servers, uses an LLM for reasoning and data interpretation, and renders rich visual responses — charts, dashboards, status grids, action buttons — inline within the chat conversation.

The system has three main layers:

```
┌─────────────────────────────────────────────────────────┐
│                      Admin UI (React)                   │
│  ChatPanel │ Charts │ DashboardBlock │ CapabilityControls│
├─────────────────────────────────────────────────────────┤
│                  chat-service Backend (Go)                    │
│  Agent Loop │ LLM Providers │ MCP Router │ Sessions     │
├─────────────────────────────────────────────────────────┤
│                  MCP Tool Servers                       │
│  harvest-mcp │ ontap-mcp │ grafana-mcp                  │
└─────────────────────────────────────────────────────────┘
```

**Key design principles:**

- **LLM as orchestrator** — the LLM decides which tools to call, how to interpret results, and what visualization format to use. For LLM-generated dashboards, the backend doesn't pre-process or shape data. For bespoke render tools (§5.6), the Go handler fetches time-series chart data server-side from VictoriaMetrics so the LLM only needs to pass scalar properties.
- **Declarative rendering** — the LLM emits typed JSON; the frontend dispatches to React components by type. No executable code crosses the wire.
- **Capability-gated tool access** — each MCP server maps to a capability with an Off/Ask/Allow state. Users control what the LLM can do.
- **Interest-driven responses** — predefined "interests" teach the LLM how to produce rich dashboard layouts for common questions, without hardcoding behavior in the backend or frontend.

---

## 2. Backend Architecture

### 2.1 Startup & Initialization

`cmd/chat-service/main.go` — `initChatbot()` runs at server start:

1. Loads AI configuration from `/etc/host application/ai.yaml` (provider, API key, model, capability states)
2. Creates the LLM provider (OpenAI, Anthropic, Bedrock, or OpenAI-compatible custom endpoint)
3. Connects to MCP servers (harvest-mcp, ontap-mcp, grafana-mcp) — discovers available tools from each
4. Loads the interest catalog (embedded built-in interests + user-defined from `/etc/host application/interests/`)
5. Builds the default capability list and merges saved states

If no AI configuration exists, the chatbot is disabled — the frontend shows a setup prompt instead of the chat interface.

### 2.2 Agent Loop

`agent/agent.go` — the core orchestration engine.

The `Agent.Run()` function implements an agentic tool-use loop:

```
User message
    │
    ▼
┌──────────────────────────────────────────────┐
│ Send messages + filtered tools to LLM        │
│                                              │
│   LLM streams response:                     │
│   ├── Text tokens → emit EventText           │
│   └── Tool calls → collect in pending list    │
│                                              │
│ If no tool calls → emit EventDone, return     │
│                                              │
│ For each tool call (in parallel):            │
│   ├── Internal tool? → execute locally        │
│   └── MCP tool? → route via Router.CallTool   │
│       ├── Ask-mode? → emit approval request   │
│       │   └── Wait for user approve/deny      │
│       └── Execute and emit result             │
│                                              │
│ Append tool results to messages               │
│ Loop (max 10 iterations)                      │
└──────────────────────────────────────────────┘
```

**Key behaviors:**

- **Parallel tool execution**: When the LLM requests multiple tools in one response, they execute concurrently.
- **Rate-limit retry**: OpenAI 429 errors trigger automatic retry with parsed delay (up to 2 retries).
- **Safety limit**: After 10 iterations, the agent asks the LLM to summarize with available information rather than looping indefinitely.
- **Ask-mode approval**: For capabilities in Ask state, the agent pauses and emits an `EventToolApprovalRequired` event. The SSE handler holds the connection open until the user approves or denies via a separate API call.

### 2.3 LLM Provider Layer

`llm/` — multi-provider abstraction.

```go
type Provider interface {
    ChatStream(ctx context.Context, req ChatRequest) iter.Seq2[StreamEvent, error]
    ValidateConfig(ctx context.Context) error
    ListModels(ctx context.Context) ([]string, error)
}
```

Supported providers:

| Provider | Implementation | SDK |
|----------|---------------|-----|
| OpenAI | `openai.go` | Official OpenAI Go SDK |
| Anthropic | `anthropic.go` | Official Anthropic Go SDK |
| AWS Bedrock | `bedrock.go` | AWS SDK v2 |
| Custom | `openai.go` (with custom endpoint) | OpenAI-compatible |
| LLM Proxy | `openai.go` (with auth header) | OpenAI-compatible |

All providers implement streaming via `iter.Seq2[StreamEvent, error]` — a Go 1.23 iterator that yields text deltas and tool calls as they arrive from the upstream API.

Configuration is stored in `/etc/host application/ai.yaml`:

```yaml
provider: openai
endpoint: https://api.openai.com/v1
api_key: sk-...
model: gpt-4-turbo
capabilities:
  harvest: allow
  ontap: ask
  grafana: off
```

### 2.4 MCP Client Router

`mcpclient/router.go` — manages connections to MCP servers and routes tool calls.

```go
type Router struct {
    servers map[string]*serverConn  // name → connection
    toolIndex map[string]string     // tool name → server name
}
```

Each MCP server exposes a set of tools. The router:

1. **Connects** to each server, opens an MCP session, and discovers its tools
2. **Merges** all tools into a single list for the LLM (the LLM sees a flat tool namespace)
3. **Routes** tool calls to the correct server based on the tool→server index
4. **Handles** disconnection gracefully — tools from disconnected servers are removed from the list

MCP server URLs default to Docker-internal addresses and are overridable via environment variables:

| Server | Default | Env Var |
|--------|---------|---------|
| harvest-mcp | `http://harvest-mcp:8082` | `MCP_HARVEST_URL` |
| ontap-mcp | `http://ontap-mcp:8084` | `MCP_ONTAP_URL` |
| grafana-mcp | `http://grafana-mcp:8085/mcp` | `MCP_GRAFANA_URL` |

### 2.5 Session Management

`session/session.go` — in-memory conversation state.

```go
type Session struct {
    ID        string
    Messages  []llm.Message
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

Sessions use a sliding window to cap context size. When the message count exceeds the maximum, `trimWindow()` removes the oldest non-system messages. Sessions are keyed by a client-generated UUID stored in the frontend.

There is no persistence — sessions are lost on chat-service restart. This is intentional: host application is an appliance, and conversation history is ephemeral.

### 2.6 System Prompt

`BuildSystemPrompt()` in `agent.go` constructs a dynamic system prompt from runtime state:

```
┌─────────────────────────────────────────────┐
│ 1. Role & guidelines                        │
│    - Storage infrastructure expert           │
│    - Markdown formatting rules               │
│    - Grafana URL rewriting                   │
│    - Destructive operation confirmations      │
├─────────────────────────────────────────────┤
│ 2. Connected data sources                   │
│    - List of MCP server names                │
│    - Tool count                              │
├─────────────────────────────────────────────┤
│ 3. Chart format spec (if interests loaded)  │
│    - 12 chart types with JSON schemas        │
│    - Dashboard panel layout rules             │
│    - Data point limits                        │
├─────────────────────────────────────────────┤
│ 4. Interest catalog (if interests loaded)   │
│    - Compact index table (ID │ Name │ Triggers)│
│    - Critical instruction: match triggers →  │
│      call get_interest before anything else   │
├─────────────────────────────────────────────┤
│ 5. Interest management spec (if save/delete │
│    tools available)                          │
│    - Create/update/delete workflow instructions│
│    - Confirmation requirements                │
└─────────────────────────────────────────────┘
```

The chart format spec is a string constant (`chartFormatSpec`) that documents all 12 panel types with their JSON schemas. The interest catalog is a dynamic markdown table built from loaded interests. Together they give the LLM the vocabulary and instructions to produce structured visual responses.

---

## 3. Frontend Architecture

### 3.1 Chat Panel

`chat-service/frontend/src/components/ChatPanel/ChatPanel.tsx` — the main chat interface, implemented as a Mantine `Drawer` that slides in from the left side of the admin UI.

**Structure:**

```
ChatPanel (Drawer)
├── Header
│   ├── Title
│   ├── CapabilityControls (popover)
│   ├── ModeToggle (read-only ↔ read-write)
│   └── Clear / Close buttons
├── Message List (ScrollArea)
│   ├── User messages (right-aligned)
│   ├── Assistant messages (left-aligned, markdown-rendered)
│   │   ├── Inline text (ReactMarkdown + remarkGfm)
│   │   ├── code[language=chart] → ChartBlock
│   │   ├── code[language=dashboard] → DashboardBlock
│   │   └── code[other] → syntax-highlighted <code>
│   └── Tool messages → ToolStatusCard
├── Suggested Prompts (when empty)
└── Input Area
    ├── Textarea
    ├── Send button
    └── Stop button (during streaming)
```

### 3.2 State Management

`useChatPanel.ts` — a custom React hook that manages all chat state:

| State | Type | Purpose |
|-------|------|---------|
| `messages` | `ChatMessage[]` | Conversation history |
| `streaming` | `boolean` | Active LLM stream |
| `sessionId` | `string` | Client-generated session UUID |
| `configured` | `boolean` | Whether AI provider is set up |
| `mode` | `"read-only" \| "read-write"` | Current execution mode |
| `modeTimeLeft` | `number \| null` | Read-write auto-disable countdown (10 min) |
| `capabilities` | `Capability[]` | Capability definitions + states |
| `pendingApproval` | `PendingApproval \| null` | Active ask-mode approval |

**Mode system**: Read-write mode must be explicitly activated and auto-disables after 10 minutes. Write-capable tools (action-button execute, save_interest, delete_interest) are only available in read-write mode.

### 3.3 SSE Streaming

The frontend uses `fetch()` with a streaming body reader to consume SSE events from `POST /chat/message`. Events are parsed line-by-line and dispatched:

| SSE Event | Frontend Action |
|-----------|----------------|
| `message` (type: text) | Append text delta to current assistant message |
| `message` (type: tool_call) | Add ToolStatusCard with "executing" status |
| `message` (type: tool_result) | Update ToolStatusCard with result + optional auto-vis |
| `message` (type: tool_error) | Update ToolStatusCard with error |
| `tool_approval_required` | Show ActionConfirmation inline card |
| `error` | Display error message |
| `done` | Finalize message, save session ID |

Text tokens stream incrementally — the user sees the response building in real time. Dashboard blocks are buffered until the closing fence arrives, with an "Assembling dashboard..." placeholder shown during accumulation.

---

## 4. Type System — Chart & Dashboard Panels

The LLM produces structured visual responses by emitting fenced code blocks containing typed JSON. The frontend dispatches each JSON object to a specific React component based on its `type` field.

### 4.1 Panel Types

There are 12 panel types, organized into three categories:

**Data visualization** (wrap Mantine/recharts components):

| Type | Component | Source | Purpose |
|------|-----------|--------|---------|
| `area` | `AreaChartBlock` | `@mantine/charts` AreaChart | Time-series trends |
| `bar` | `BarChartBlock` | `@mantine/charts` BarChart | Comparisons |
| `gauge` | `GaugeBlock` | `@mantine/core` RingProgress | Single utilization value |
| `sparkline` | `SparklineBlock` | `@mantine/charts` Sparkline | Compact inline trend |
| `status-grid` | `StatusGridBlock` | Custom (SimpleGrid + Badge) | Multi-resource health |
| `stat` | `StatBlock` | Custom (Text + Group) | Single prominent value |

**Interest-specific** (custom Mantine compositions):

| Type | Component | Purpose |
|------|-----------|---------|
| `alert-summary` | `AlertSummaryBlock` | Severity count badges (clickable) |
| `resource-table` | `ResourceTableBlock` | Clickable resource list |
| `alert-list` | `AlertListBlock` | Active alerts with severity + time |
| `callout` | `CalloutBlock` | Highlighted recommendation card |
| `proposal` | `ProposalBlock` | Proposed CLI command |
| `action-button` | `ActionButtonBlock` | Execute or conversational buttons |

### 4.2 Rendering Dispatch

Two entry points handle chart JSON:

**Standalone charts** — `ChartBlock.tsx` handles `language-chart` code fences:

```
```chart
{ "type": "area", "title": "...", ... }
```​
```

**Multi-panel dashboards** — `DashboardBlock.tsx` handles `language-dashboard` code fences:

```
```dashboard
{ "title": "Fleet Health", "panels": [ { "type": "area", "width": "half", ... }, ... ] }
```​
```

**Object detail views** (planned) — `ObjectDetailBlock.tsx` will handle `language-object-detail` code fences:

```
```object-detail
{ "type": "object-detail", "kind": "volume", "name": "vol_prod_01", "sections": [...] }
```​
```

All three components:
1. Parse and validate the JSON using type-specific parsers from `chartTypes.ts`
2. Dispatch each panel/section to the correct renderer component by `type` / `layout`
3. Fall back gracefully — unknown types or malformed JSON render as a plain code block

`DashboardBlock` also manages a responsive CSS grid layout where panels declare their width as `"full"` (100%), `"half"` (50%), or `"third"` (33%).

`ObjectDetailBlock` renders a single-entity detail page: identity header, then a sequence of sections (properties grid, embedded charts, timeline, alert list, actions, text, tables). See `docs/chatbot-object-detail-design.md` for the full schema and navigation paradigm.

### 4.3 Type Inference

`inferChartType()` in `chartTypes.ts` provides shape-based type inference when the LLM omits the `type` field. This handles edge cases where the LLM returns valid panel JSON without an explicit type — common with alert-list, gauge, and status-grid shapes.

The inference logic examines the JSON structure (presence of specific keys, array shapes, value types) and maps it to one of the 12 known types. It is used as a fallback in both `parseChart()` and the inline chart detector.

### 4.4 Inline Chart Detection

`inlineChartDetector.ts` — `wrapInlineChartJson()` handles a common LLM behavior: emitting bare JSON objects in the response text without wrapping them in a fenced code block.

The detector:
1. Scans the assistant message for bare `{...}` JSON objects outside code fences
2. Sanitizes the JSON (strips JS-style comments, trailing commas)
3. Classifies the object: `chart`, `dashboard`, or neither (using `inferChartType()` as fallback)
4. Wraps detected chart/dashboard JSON in the appropriate code fence so ReactMarkdown routes it to the correct renderer

### 4.5 Data Safety

`downsample()` in `chartTypes.ts` is a safety net for large data arrays. If a chart's data array exceeds 200 points, it is downsampled by picking every Nth point. The system prompt also instructs the LLM to limit data to ~50–100 rows.

---

## 5. Interest System

Interests are predefined response patterns that teach the LLM how to produce rich, structured responses for common questions. They bridge the gap between "the LLM knows the chart vocabulary" and "the LLM consistently produces the exact dashboard layout we want."

### 5.1 Concept

An interest is a markdown file with YAML frontmatter:

```markdown
---
id: morning-coffee
name: Fleet Health Overview
source: builtin
triggers:
  - how's everything
  - any issues
  - summary
  - good morning
requires:
  - harvest
---

When the user wants an overall health check, produce a dashboard with:
1. alert-summary (full width) — Call get_active_alerts...
2. area (half width) — Cluster Performance (7d)...
...
```

**Frontmatter** provides metadata for matching and filtering. **Body** provides instructions the LLM follows when producing the response.

### 5.2 Two Tiers

**Built-in interests** are authored by the host application team, embedded in the binary via `//go:embed`, and prescriptive — they specify exact panel types, widths, tool calls, and layout order. Six ship today:

| Interest | Triggers | Requires | Output type | Render |
|----------|----------|----------|-------------|--------|
| `morning-coffee` | "how's everything", "any issues", "summary", "fleet summary" | harvest | `dashboard` | LLM-generated |
| `morning-coffee-v2` | "per cluster view", "cluster breakdown", "detailed fleet view" | harvest | `dashboard` | LLM-generated |
| `resource-status` | "tell me about cluster/SVM/aggregate", "how is", "status of" | harvest | `object-detail` or `dashboard` | LLM-generated |
| `object-list` | "show me volumes", "list aggregates", "top clusters" | harvest | `dashboard` | LLM-generated |
| `volume-detail` | "tell me about volume", "volume details", "monitor this volume" | harvest | `object-detail` | **Bespoke** (`render_volume_detail`) |
| `volume-provision` | "provision a volume", "new volume", "need storage" | harvest, ontap | `dashboard` (with `action-form`) | LLM-generated |

Most interests are **LLM-generated** — the interest body tells the LLM exactly what tools to call and what output structure to produce, and the LLM assembles the final JSON (`dashboard` or `object-detail` block) itself.

One interest — `volume-detail` — is **bespoke**: the interest body tells the LLM what data to gather, but the final output is built by a Go render tool (`render_volume_detail`) rather than by the LLM. See §5.6 for the bespoke render tool architecture.

The two fleet health interests (`morning-coffee` and `morning-coffee-v2`) form a summary/detailed pair. Each dashboard includes a `toggle` field that renders a clickable badge next to the title, switching between the two views by injecting a trigger message for the other interest.

**User-defined interests** are created by users (via chat or by dropping files in `/etc/host application/interests/`), have `source: user`, and are typically descriptive — prose that the LLM interprets to choose panel types and layout.

### 5.3 How It Works

The interest catalog flows through the system in three stages:

**Stage 1 — Index in system prompt**: At session start, `BuildSystemPrompt()` includes a compact table of interest IDs, names, and trigger phrases. This costs ~200-600 tokens regardless of catalog size.

**Stage 1.5 — Pre-filter tools by interest** (optional): Before the agent is created, the chat handler attempts a fast substring match of the user message against all interest triggers using `Catalog.Match()`. If a trigger matches, the handler narrows the tool set to only the matched interest's required capabilities (e.g. morning-coffee requires only `harvest`, so ontap and grafana tools are excluded). This reduces the tool schema sent to the LLM on every iteration, improving TTFT. If no trigger matches, the full tool set is sent. See §6.2 for details.

**Stage 2 — LLM matches and retrieves**: When the user sends a message, the LLM checks if it semantically matches any trigger phrase. If so, it calls `get_interest(id)` as its first tool call to retrieve the full interest body. This is a local lookup — no network call.

**Stage 3 — LLM follows instructions**: The LLM reads the interest body, calls the specified tools to gather data, then produces the output. The final step depends on whether the interest is LLM-generated or bespoke:

- **LLM-generated** (5 of 6 built-in interests): The LLM assembles the output JSON itself — a `dashboard` or `object-detail` code block following the layout instructions. For built-in interests, the instructions are precise (specific panel types, widths, queries). For user-defined interests, the LLM exercises judgment.

- **Bespoke** (currently `volume-detail` only): The LLM gathers scalar data (properties, status, monitoring state) and then calls a **render tool** (`render_volume_detail`). The render tool — a Go `InternalTool` — deterministically builds the `object-detail` JSON. The LLM does not assemble the output; Go code does. See §5.6 for details.

```
User: "How's everything looking?"         User: "Tell me about volume proxmox1"
  │                                         │
  ▼                                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│ Stage 1 — System prompt includes interest index                    │
│ Stage 2 — LLM matches trigger → calls get_interest(id)            │
│           Returns: full interest body with instructions            │
│ Stage 3 — LLM calls data-gathering tools (metrics, alerts, etc.)  │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
     LLM-Generated                  Bespoke
  (5 of 6 interests)           (volume-detail)
              │                         │
  LLM assembles dashboard      LLM calls render_*() tool
  or object-detail JSON        with scalar properties
              │                         │
              │                 Go handler builds
              │                 object-detail JSON +
              │                 fetches chart data from
              │                 VictoriaMetrics
              │                         │
              │                 Result emitted directly
              │                 to frontend (EmitResult)
              │                         │
              ▼                         ▼
  LLM adds narrative text      LLM adds brief summary
              │                         │
              ▼                         ▼
  Frontend renders              Frontend renders
  DashboardBlock or             ObjectDetail +
  ObjectDetail + text           markdown text
```

### 5.4 Interest Management

Users manage interests through three tools (all require read-write mode):

| Tool | Purpose | Guardrails |
|------|---------|------------|
| `get_interest(id)` | Retrieve interest body | Read-only, any mode |
| `save_interest(...)` | Create or update user interest | Max 10 user interests, no built-in ID shadowing, valid capability refs |
| `delete_interest(id)` | Remove user interest | Cannot delete built-in interests |

The LLM mediates all management actions — when a user says "save a new interest," the LLM infers metadata, drafts the body, shows a preview for confirmation, and only saves after explicit approval.

### 5.5 Implementation

| File | Purpose |
|------|---------|
| `interest/interest.go` | `InterestMeta`, `Interest` types; YAML frontmatter parser |
| `interest/catalog.go` | `Catalog` — load, filter, index, save, delete |
| `interest/embed.go` | `//go:embed interests/*.md` |
| `interest/tool.go` | Tool definitions and handlers for get/save/delete |
| `interest/interests/*.md` | Built-in interest files |

### 5.6 Bespoke Render Tools

Built-in interests instruct the LLM to gather data and produce structured UI
blocks. In the original design the LLM also assembled the final JSON output
(the `object-detail` or `dashboard` block). This works well for dashboards,
but for single-object detail views the LLM occasionally omits sections,
forgets buttons, or varies the layout across sessions.

**Bespoke render tools** solve this by splitting the pipeline:

1. The **interest** tells the LLM *what data to gather* (metrics, alerts,
   monitoring status) and *which render tool to call*.
2. The **render tool** (a Go `InternalTool` in `render/`)
   deterministically builds the `object-detail` JSON from the gathered data.

This keeps the flexible, natural-language data-gathering step (which the LLM
excels at) while guaranteeing the final UI is always correct and complete.

**Server-side chart data**: Render tools fetch time-series data (IOPS,
latency, capacity trends) directly from VictoriaMetrics via `MetricsFetcher`,
rather than relying on the LLM to pass large arrays through tool arguments.
The LLM passes only scalar properties (name, size, status, etc.); the Go
handler queries VictoriaMetrics for chart data and populates the chart
sections deterministically.

```
┌─────────────────────────────────────────────────────────┐
│                  LLM-Generated Flow                     │
│                                                         │
│  Interest ──→ LLM gathers data ──→ LLM builds JSON     │
│                                     ↑ inconsistent      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│                  Render Tool Flow                        │
│                                                         │
│  Interest ──→ LLM gathers data ──→ render_*() tool      │
│                                     ↑ deterministic     │
│                                     Go builds JSON      │
└─────────────────────────────────────────────────────────┘
```

#### Bespoke Interest Inventory

Currently one interest uses the bespoke pattern:

| Interest | Render tool | Output type | Server-side data |
|----------|-------------|-------------|------------------|
| `volume-detail` | `render_volume_detail` | `object-detail` (6 sections) | IOPS, latency (24h/5m), capacity trend (30d/1d) via `MetricsFetcher` |

All other built-in interests (`morning-coffee`, `morning-coffee-v2`,
`resource-status`, `object-list`, `volume-provision`) are LLM-generated —
the LLM assembles the final JSON output.

#### Enforcement Mechanisms

Because LLMs are unreliable at following mandatory tool-call instructions,
bespoke interests use two enforcement flags on the `InternalTool` registration:

| Flag | Purpose |
|------|---------|
| `RequiredAfterInterest` | Names the interest that makes this tool mandatory. When the named interest has been loaded (via `get_interest`), the agent ensures the tool is called before the turn ends. If the LLM finishes with text instead, the agent clears the text, injects a system message ("You MUST call {tool} now"), and retries. |
| `EmitResult` | When `true`, the tool's return value is emitted directly as an `EventText` SSE event — injected into the assistant message stream so the frontend renders it inline. Without this, the tool result would only appear inside a collapsed tool-result card. |

**History pre-scan**: The `RequiredAfterInterest` check works across turns.
At the start of each `Run()` call, the agent scans the message history for
any prior `get_interest` tool calls and seeds the `loadedInterests` map.
This prevents a gap where the LLM reuses interest instructions from a
previous turn without re-calling `get_interest`, which would otherwise
bypass enforcement.

#### When to use a render tool vs. LLM-generated output

| Scenario | Approach |
|----------|----------|
| Single-object detail views with guaranteed sections (volume, aggregate, alert) | **Render tool** — consistency is critical |
| Multi-panel dashboards (morning-coffee, resource-status) | **LLM-generated** — layout is simple, variation is acceptable |
| User-defined interests | **LLM-generated** — user controls the output shape |

#### Volume Detail Wireframe

The `render_volume_detail` tool produces the following guaranteed layout:

```
┌──────────────────────────────────────────────────────────┐
│  📦 vol_docs                                    [ok]     │
│  Volume on SVM vdbench, cluster cls1                     │
├──────────────────────────────────────────────────────────┤
│  Properties                                              │
│  ┌────────────────────┬────────────────────┐             │
│  │ State      online  │ Total Size  51.4GB │             │
│  │ Used       89%     │ Aggregate   aggr1→ │             │
│  │ SVM        vdbench→│ Cluster     cls1→  │             │
│  │ Style      FlexVol │ Protocol    NFS    │             │
│  │ Monitoring Active (3 capacity, 3 dp)    │             │
│  └────────────────────┴────────────────────┘             │
├──────────────────────────────────────────────────────────┤
│  Performance (last 24h)                                  │
│  ┌──────────────────────────────────────────┐            │
│  │  📈 IOPS & Latency area chart            │            │
│  │  Series: Read IOPS, Write IOPS, Latency  │            │
│  └──────────────────────────────────────────┘            │
│  (Falls back to "No I/O activity" text when no data)     │
├──────────────────────────────────────────────────────────┤
│  Capacity Trend (30 days)                                │
│  ┌──────────────────────────────────────────┐            │
│  │  📈 Used % area chart                    │            │
│  │  ─ ─ Warning (85%)  ─ ─ Critical (95%)  │            │
│  └──────────────────────────────────────────┘            │
│  (Falls back to "No capacity trend data" text)           │
├──────────────────────────────────────────────────────────┤
│  Active Alerts                                           │
│  ⚠ Volume vol_docs used 89%         2024-01-15 10:30    │
│  (Empty state: no items, section still rendered)         │
├──────────────────────────────────────────────────────────┤
│  Health Analysis                                         │
│  Free-text paragraph written by the LLM describing       │
│  volume health, risks, and recommendations.              │
├──────────────────────────────────────────────────────────┤
│  Actions                                                 │
│  [Stop Monitoring🔒] [Show Snapshots] [Resize Volume]   │
│                                                          │
│  🔒 = requiresReadWrite (disabled in read-only mode)    │
│  Toggle label changes: "Monitor this Volume" when off    │
└──────────────────────────────────────────────────────────┘
```

**Key guarantees:**
- Always exactly 6 sections in this order
- Monitoring button always present with correct toggle label
- `requiresReadWrite` always set on monitoring buttons
- Empty data gracefully falls back to text sections
- Property links (→) inject follow-up chat messages

#### Implementation Files

| File | Purpose |
|------|---------|
| `render/render.go` | Shared Go types mirroring TypeScript `ObjectDetailData` |
| `render/volume.go` | `render_volume_detail` tool: `VolumeInput` → `ObjectDetail` |
| `render/volume_test.go` | 17 tests covering all sections, edge cases, round-trip |
| `interest/interests/volume-detail.md` | Interest body instructs LLM to call `render_volume_detail` |
| `server/server.go` | Registers `render_volume_detail` as `InternalTool` |

---

## 6. Capability System

Capabilities gate LLM access to MCP tool servers. Each MCP server maps to one capability with three states:

| State | Behavior |
|-------|----------|
| **Off** | Tools from this server are excluded from the LLM's tool list. The LLM doesn't know they exist. |
| **Ask** | Tools are visible to the LLM, but each call pauses for user approval before executing. |
| **Allow** | Tools execute autonomously — no user intervention required. |

### 6.1 Default Capabilities

| Capability ID | Server | Description | Default State |
|---------------|--------|-------------|---------------|
| `harvest` | harvest-mcp | Infrastructure metrics, health monitoring, capacity analysis | Ask |
| `ontap` | ontap-mcp | Volume lifecycle, snapshots, data protection, multi-cluster management | Ask |
| `grafana` | grafana-mcp | Dashboard search, Prometheus queries, alert rules, panel images | Ask |

### 6.2 How Filtering Works

Tool filtering happens in two stages: **pre-filtering** (before agent creation) and **capability filtering** (inside the agent loop).

#### Pre-filtering by Interest

Before the agent is created, the chat handler attempts to match the user's message against interest triggers using `Catalog.Match()`. If a trigger matches, the handler narrows `capStates` to only the capabilities the matched interest requires — all other capabilities are set to Off. This reduces the tool schema sent to the LLM on every iteration (e.g. ~42 tools → ~15 for harvest-only interests), improving time-to-first-token.

If no trigger matches, no pre-filtering is applied — the full tool set is available.

#### Capability Filtering in the Agent

When the agent prepares to call the LLM, `filteredTools()` builds the tool list:

1. Gets all tools from the MCP router
2. Maps each tool to its server, then to the corresponding capability
3. Excludes tools whose capability is Off (including any set Off by pre-filtering)
4. For Ask-state tools, the agent's `ApprovalFunc` gates execution at call time
5. Internal tools (get_interest, save_interest, delete_interest) are appended after filtering
6. Tools marked `ReadWriteOnly` (save_interest, delete_interest) are excluded unless mode is read-write

### 6.3 Ask-Mode Approval Flow

```
Agent encounters tool call → capability state is Ask
    │
    ├── Emit EventToolApprovalRequired (SSE)
    │     └── approval_id, capability, tool, params
    │
    ▼ (agent blocks, waiting)
    │
Frontend shows ActionConfirmation inline
    │
    ├── User clicks Approve → POST /chat/approve
    │     └── ApprovalFunc returns true → tool executes
    └── User clicks Deny → POST /chat/deny
          └── ApprovalFunc returns false → tool skipped
```

### 6.4 Frontend Controls

`CapabilityControls.tsx` renders a popover accessible from the chat header:

- Per-capability segmented control (Off | Ask | Allow)
- Availability indicator (gray when MCP server is disconnected)
- Tool count badge
- "Show tool traces" toggle — displays ToolStatusCards for tool execution visibility

---

## 7. API Surface

All endpoints are under `/`.

### 7.1 Chat Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/chat/message` | Send message, receive SSE stream |
| `DELETE` | `/chat/session` | Clear session history |
| `GET` | `/chat/capabilities` | Get capability definitions + states |
| `POST` | `/chat/capabilities` | Update capability states |
| `POST` | `/chat/approve` | Approve ask-mode tool call |
| `POST` | `/chat/deny` | Deny ask-mode tool call |

### 7.2 AI Configuration Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/ai/config` | Get current LLM config (API key masked) |
| `POST` | `/ai/config` | Save LLM config, reinitialize chatbot |
| `DELETE` | `/ai/config` | Remove config, disable chatbot |
| `POST` | `/ai/test` | Validate LLM connection |
| `POST` | `/ai/models` | List available models from provider |

### 7.3 SSE Protocol

`POST /chat/message` returns `text/event-stream`:

```
event: message
data: {"type":"text","content":"Let me check..."}

event: message
data: {"type":"tool_call","tool":"get_active_alerts","params":{},"capability":"harvest","status":"executing"}

event: message
data: {"type":"tool_result","tool":"get_active_alerts","result":"{...}"}

event: message
data: {"type":"text","content":"```dashboard\n{...}\n```"}

event: done
data: {"type":"done","session_id":"abc-123"}
```

Event types:

| Event | Data Fields | When |
|-------|-------------|------|
| `message` (text) | `content` | Streamed text tokens |
| `message` (tool_call) | `tool`, `params`, `capability`, `status` | Tool starts executing |
| `message` (tool_result) | `tool`, `result` | Tool succeeded |
| `message` (tool_error) | `tool`, `error` | Tool failed |
| `tool_approval_required` | `approval_id`, `capability`, `tool`, `params`, `description` | Ask-mode pause |
| `error` | `message` | Fatal error |
| `done` | `session_id` | Stream complete |

---

## 8. Tool Visualization

### 8.1 ToolStatusCard

Every tool execution renders a `ToolStatusCard` in the message list. It shows:
- Tool name and capability
- Execution status (executing → completed / failed)
- Auto-visualization of results when applicable (sparkline from time-series data, gauge from capacity data)
- Toggle between chart view and raw JSON

The "Show tool traces" toggle in CapabilityControls controls whether ToolStatusCards are visible. When hidden, tool calls execute silently and only the final assistant text (and charts) are shown.

### 8.2 Auto-Visualization Heuristics

`detectToolViz()` in `ToolStatusCard.tsx` examines tool result JSON:
- Array of objects with timestamp + numeric fields → inline sparkline
- Single object with value + max → mini gauge
- Otherwise → plain text (collapsed at 3 lines)

This provides lightweight visualization even when the LLM doesn't produce a formal chart block.

---

## 9. Rendering Pipeline — End to End

Three paths exist for rendering LLM output:

```
                    ┌─────────────────────────┐
                    │     ChatMessage          │
                    │  role: assistant | tool   │
                    └──────────┬──────────────┘
                               │
              ┌────────────────┴─────────────────┐
              │                                  │
     role = assistant                     role = tool
              │                                  │
     ReactMarkdown                      ToolStatusCard
     + remarkGfm                               │
              │                         toolResult has
     code block lang?                   known data shape?
     ┌────────┼──────────┐               │          │
  dashboard  chart      other          YES         NO
     │        │           │              │          │
 Dashboard  ChartBlock  <code>      Mini chart   Plain text
  Block     (single)                               (lineClamp 3)
  (multi-panel,
   clickable)
```

**Path 1 — Dashboard blocks** (interest-driven): Multi-panel grid with clickable elements that inject follow-up chat messages.

**Path 2 — Standalone chart blocks**: Single chart inline in the message.

**Path 3 — ToolStatusCard auto-visualization**: Automatic detection of chartable data in tool results — no LLM formatting needed.

Additionally, `wrapInlineChartJson()` preprocesses assistant messages to catch bare JSON that should have been in a code fence, wrapping it so Paths 1 and 2 can handle it.

---

## 10. Configuration & Environment

### 10.1 AI Configuration

Stored in `/etc/host application/ai.yaml` (or path from `AI_CONFIG_PATH` env):

```yaml
provider: openai          # openai | anthropic | bedrock | custom | llm-proxy
endpoint: https://api.openai.com/v1
api_key: sk-...
model: gpt-4-turbo
user: ""                  # optional, for llm-proxy
aws_region: ""            # bedrock only
aws_access_key: ""        # bedrock only
aws_secret_key: ""        # bedrock only
capabilities:
  harvest: allow
  ontap: ask
  grafana: off
```

### 10.2 MCP Server URLs

| Env Var | Default | Server |
|---------|---------|--------|
| `MCP_HARVEST_URL` | `http://harvest-mcp:8082` | harvest-mcp |
| `MCP_ONTAP_URL` | `http://ontap-mcp:8084` | ontap-mcp |
| `MCP_GRAFANA_URL` | `http://grafana-mcp:8085/mcp` | grafana-mcp |

### 10.3 Dev Environment

- `scripts/dev-start.sh` / `scripts/dev-stop.sh` — start/stop the full dev stack
- MCP servers in dev: harvest(8084), ontap(8085), grafana(8086)
- Vite dev proxy: `/api` → `localhost:8080` (chat-service), harvest-proxy routes → `localhost:8083`
- Dev-mode auth: `FakeAuthMiddleware` accepts `admin/Netapp01`

---

## 11. Security Model

host application uses a layered security model with scoped tokens, JWT sessions, and capability-gated tool access. The chatbot inherits the appliance-wide auth infrastructure and adds chatbot-specific controls on top.

### 11.1 Authentication Stack

All `/` routes — including all chat and AI configuration endpoints — pass through a middleware chain:

```
Request
  │
  ├── BasicAuthMiddleware  — checks username/password against /etc/shadow
  ├── JWTAuthMiddleware    — checks X-Token / X-Token-Refresh cookies (HMAC-signed)
  ├── tokens.AuthMiddleware — checks Bearer token against hashed token file
  ├── ConfirmAuthMiddleware — rejects if none of the above succeeded
  └── RequireScopeMiddleware("host application-API") — enforces scope on Bearer tokens
```

Three authentication methods, tried in order:

| Method | Mechanism | When Used |
|--------|-----------|-----------|
| **Basic Auth** | Username + password validated against `/etc/shadow` | Initial login, API scripts |
| **JWT** | HMAC-signed cookies (`X-Token` 5min, `X-Token-Refresh` 20min) | Web admin sessions (after initial Basic Auth login) |
| **Bearer Token** | SHA-256 hashed token checked against token file | Programmatic API access, MCP client access |

JWT tokens are issued after successful Basic Auth and auto-refresh via the `X-Token-Refresh` cookie. The web admin session is stateless — no server-side session store for the HTTP auth layer. (Chat sessions are separate — see §2.5.)

### 11.2 Scoped Tokens

host application issues API tokens that are **scoped** to specific services and optionally **restricted to specific clusters**. Token storage is a flat file of SHA-256 hashes with tab-separated metadata:

```
<sha256-hash>    <name>    <scopes>    <clusters>
```

**Scopes** control which services a token can access:

| Scope | Grants Access To |
|-------|-----------------|
| `host application-API` | chat-service admin API (all `/` routes) |
| `Harvest-MCP` | Harvest MCP server (via Caddy `forward_auth`) |
| `ONTAP-MCP` | ONTAP MCP server (via Caddy `forward_auth`) |
| `Grafana-MCP` | Grafana MCP server (via Caddy `forward_auth`) |
| `harvest-proxy` | harvest-proxy REST API |
| `harvest-proxy-Proxy` | harvest-proxy metrics proxy |
| `VictoriaMetrics` | VictoriaMetrics query API |
| `Node-Exporter` | Node exporter metrics |
| `*` | Wildcard — all scopes |

A token created with `scopes: ["ONTAP-MCP"]` can access ontap-mcp through Caddy but cannot call the chat-service admin API or any other MCP server.

**Cluster restrictions** further limit what ONTAP clusters a token can operate on:

```json
{ "name": "team-a", "scopes": ["ONTAP-MCP", "harvest-proxy"], "clusters": ["clusterA", "clusterB"] }
```

This token can only query data for `clusterA` and `clusterB` — requests targeting other clusters are rejected. Clusters default to `["*"]` (all clusters) when not specified. Cluster enforcement uses `IsValidWithCluster()` which checks both scope and cluster in one call.

### 11.3 Caddy Forward Auth

host application uses Caddy as its reverse proxy. Each backend service route is protected by Caddy's `forward_auth` directive, which sends a subrequest to chat-service's `/auth` endpoint with:

- `X-Forwarded-Uri` — the original request path
- `X-Required-Scope` — the scope tag assigned to that route

chat-service's `ForwardAuthHandler` checks the Bearer token against the required scope. This means **external MCP client access** (e.g., Claude Desktop connecting to `/mcp/ontap/`) is scope-gated at the Caddy layer — a `Harvest-MCP` token cannot reach the ONTAP MCP endpoint.

```
External client → Caddy → forward_auth → chat-service /auth → scope check
                    │
                    └── scope OK → reverse_proxy → MCP container
```

**Guest access bypass**: Monitoring scopes (`VictoriaMetrics`, `Node-Exporter`, `harvest-proxy`, `harvest-proxy-Proxy`) can be opened without tokens when `VM_GUEST_ACCESS=true` is set, allowing read-only monitoring integrations.

### 11.4 Chatbot Internal MCP Access

The chatbot in chat-service connects to MCP containers **directly on the Docker network** (e.g., `http://harvest-mcp:8082`) — not through Caddy. This is internal container-to-container communication with no token auth on the wire.

This is secure because:
- MCP containers are on an isolated Docker network — not exposed externally
- External access goes through Caddy → `forward_auth` → scoped token check (§11.3)
- The chatbot's tool access is further gated by the capability system (§11.5)

### 11.5 Capability Controls

On top of authentication, the chatbot has its own authorization layer via capabilities. Each MCP server maps to a capability with three states:

| State | Effect |
|-------|--------|
| **Off** | Tools from this server are excluded from the LLM's tool list entirely. The LLM doesn't know they exist. |
| **Ask** | Tools are visible but each call pauses for explicit user approval before executing. |
| **Allow** | Tools execute autonomously. |

All capabilities default to **Ask** — the user must explicitly opt into autonomous execution. Capability states are persisted in `/etc/host application/ai.yaml` under the `capabilities` map and survive restarts.

This gives users fine-grained control: they can allow Harvest (read-only metrics queries) to run freely while keeping ONTAP (which has write operations like volume creation) in Ask mode.

### 11.6 Read-Write Mode & Action Confirmation

The chatbot operates in **read-only mode by default**. Write-capable operations require:

1. **Explicit mode activation**: User toggles to read-write mode in the UI
2. **Auto-disable timer**: Read-write mode automatically reverts after 10 minutes
3. **Action confirmation**: Even in read-write mode, `action-button` execute commands and interest management tools (`save_interest`, `delete_interest`) go through a confirmation flow — the LLM shows what it intends to do and waits for approval

These layers stack: a destructive ONTAP operation requires (a) the ONTAP capability to be Ask or Allow, (b) read-write mode to be active, and (c) user approval of the specific action.

### 11.7 LLM API Key Security

- API keys are stored in `/etc/host application/ai.yaml` on the appliance filesystem (root-owned)
- `GET /ai/config` masks the key before returning it to the frontend — the full key is never sent to the browser after initial configuration
- Keys are never logged (structured logging deliberately excludes credential fields)
- Keys are sent only to the configured LLM endpoint over HTTPS

### 11.8 Grafana Service Account Provisioning

The Grafana MCP needs a service account token to query Grafana. chat-service auto-provisions this at startup via the Grafana HTTP API:

1. Creates a `Viewer`-role service account (read-only — cannot modify dashboards or settings)
2. Generates a token for the service account
3. Writes the token to `.env.custom` so Docker Compose injects it into the `grafana-mcp` container
4. Restarts `grafana-mcp` to pick up the token

The Viewer role is deliberately minimal — the MCP can query dashboards and metrics but cannot create, modify, or delete anything in Grafana.

### 11.9 Declarative Rendering

The LLM emits typed JSON, not executable code. The frontend renders it through type-dispatched React components. There is no `eval()`, no dynamic script injection, no HTML rendering from LLM output. Malformed JSON falls back to a plain code block — the worst case is unrendered text, not code execution.

### 11.10 Security Summary

```
Layer                     What It Protects              How
─────────────────────     ──────────────────────────    ────────────────────────────
Caddy forward_auth        External MCP access           Scoped Bearer tokens
chat-service auth middleware    Admin API + chat endpoints    Basic Auth / JWT / Bearer token
RequireScopeMiddleware    API route access               host application-API scope check
Cluster restrictions      Multi-tenant data isolation   Token-level cluster list
Capability Off/Ask/Allow  LLM tool access               User-controlled per-MCP
Read-write mode           Destructive operations        Manual toggle + 10min timer
Action confirmation       Individual write actions      Inline approval flow
Grafana SA provisioning   Grafana data access           Viewer-role (read-only)
Declarative rendering     Frontend code execution       Type-dispatched JSON, no eval
```

---

## 12. File Index

### Backend (Go)

| File | Lines | Purpose |
|------|-------|---------|
| `cmd/chat-service/main.go` | ~250 | Startup, MCP connections, capability init |
| `server/server.go` | ~400 | SSE streaming, session management, ask-mode |
| `config/config.go` | ~300 | LLM config CRUD, model discovery, validation |
| `agent/agent.go` | ~750 | Agentic tool-use loop, system prompt, tool filtering |
| `llm/provider.go` | ~150 | Provider interface, config types |
| `llm/openai.go` | ~250 | OpenAI/custom provider |
| `llm/anthropic.go` | ~250 | Anthropic provider |
| `llm/bedrock.go` | ~200 | AWS Bedrock provider |
| `mcpclient/router.go` | ~350 | Multi-server MCP routing |
| `session/session.go` | ~150 | In-memory sessions, sliding window |
| `capability/capability.go` | ~120 | Off/Ask/Allow state model |
| `interest/interest.go` | ~80 | Interest types, frontmatter parser |
| `interest/catalog.go` | ~280 | Catalog loading, filtering, indexing |
| `interest/tool.go` | ~280 | get/save/delete tool handlers |
| `interest/embed.go` | ~5 | `//go:embed` for built-in interests |
| `interest/interests/*.md` | — | Built-in interest files |
| `render/render.go` | ~120 | Shared Go types for deterministic object-detail rendering |
| `render/volume.go` | ~340 | `render_volume_detail` tool handler |
| `chat-service/internal/alertmgr/alertmgr.go` | ~250 | Volume monitoring rule builder (enable/disable/status) |

### Frontend (TypeScript/React)

| File | Lines | Purpose |
|------|-------|---------|
| `ChatPanel.tsx` | ~300 | Main chat drawer, message rendering, markdown integration |
| `useChatPanel.ts` | ~400 | State management, SSE streaming, mode/approval/capability state |
| `CapabilityControls.tsx` | ~90 | Off/Ask/Allow toggles, tool traces toggle |
| `ModeToggle.tsx` | ~40 | Read-only ↔ read-write with countdown |
| `ActionConfirmation.tsx` | ~80 | Ask-mode approval inline card |
| `ToolStatusCard.tsx` | ~120 | Tool execution status + auto-vis |
| `inlineChartDetector.ts` | ~200 | Bare JSON detection + code fence wrapping |
| `charts/chartTypes.ts` | ~300 | TypeScript interfaces, parsers, type inference |
| `charts/ChartBlock.tsx` | ~100 | Single chart dispatcher |
| `charts/DashboardBlock.tsx` | ~150 | Multi-panel grid layout |
| `charts/AreaChartBlock.tsx` | — | Mantine AreaChart wrapper |
| `charts/BarChartBlock.tsx` | — | Mantine BarChart wrapper |
| `charts/GaugeBlock.tsx` | — | Mantine RingProgress wrapper |
| `charts/SparklineBlock.tsx` | — | Mantine Sparkline wrapper |
| `charts/StatusGridBlock.tsx` | — | Custom SimpleGrid + Badge |
| `charts/StatBlock.tsx` | — | Big number display |
| `charts/AlertSummaryBlock.tsx` | — | Clickable severity badges |
| `charts/ResourceTableBlock.tsx` | — | Clickable resource table |
| `charts/AlertListBlock.tsx` | — | Alert detail list |
| `charts/CalloutBlock.tsx` | — | Recommendation card |
| `charts/ProposalBlock.tsx` | — | Proposed command display |
| `charts/ActionButtonBlock.tsx` | — | Execute/message action buttons |

---

## 13. Related Documents

- **Design Spec**: `docs/chatbot-design-spec.md` — original design covering MCP deployment, BYO LLM, backend API, frontend UI, capability controls, security, and phasing
- **Graphical UI Enhancements**: `docs/chatbot-graphical-ui-enhancements.md` — interest system design, chart type catalog, rendering architecture, implementation plan with milestones
- **Object-Detail Design**: `docs/chatbot-object-detail-design.md` — interest/type layering, the `object-detail` code fence type, navigation paradigm (dashboard → drill-down → detail), and the alerts lighthouse use case
