# Chatbot Graphical UI Enhancements

**Status:** Draft — Planning  
**Date:** 2026-03-05  
**Companion:** [chatbot-design-spec.md](chatbot-design-spec.md)

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Current State](#2-current-state)
3. [Design Goals](#3-design-goals)
4. [Interest Response System](#4-interest-response-system)
5. [Visualization Data Contract](#5-visualization-data-contract)
6. [Chart Components](#6-chart-components)
7. [Rendering Architecture](#7-rendering-architecture)
8. [Implementation Phases](#8-implementation-phases)
9. [Related Work: A2UI](#9-related-work-a2ui)
10. [Open Questions](#10-open-questions)
11. [Implementation Plan](#11-implementation-plan)

---

## 1. Problem Statement

The chatbot currently returns text-only responses. When it queries MCP servers for metrics, capacity, health, or trends, the results are:

- **Assistant messages**: Plain markdown text (tables, lists)
- **Tool cards**: Plain-text `toolResult` truncated to 3 lines

Storage administrators need **visual context** — a capacity trend line, a health status grid, a utilization gauge — to quickly understand what the data means. Text tables of 50 time-series data points are not actionable at a glance.

### Examples of what we want

| User asks | LLM today | LLM with visualization |
|-----------|-----------|----------------------|
| "Show me storage consumption for the last week" | Text table of daily values | Area chart trending over 7 days |
| "How healthy are my volumes?" | Bulleted list of status per volume | Status grid with green/yellow/red indicators per volume, capacity bars |
| "What's my aggregate utilization?" | "Aggregate X is at 82%" | Ring gauge at 82% with threshold coloring |
| "Compare throughput across nodes" | Text table | Grouped bar chart |
| "Any alerts trending up?" | Text list of alerts | Sparkline of alert count over time |

---

## 2. Current State

### 2.1 Frontend Rendering

- **Assistant messages**: `ReactMarkdown` with `remarkGfm` — supports tables, code blocks, links
- **Tool messages**: `ToolStatusCard` shows tool name, capability badge, status badge, and `toolResult` as plain text (`lineClamp={3}`)
- **No chart rendering** in the chat panel today

### 2.2 Available Libraries (already in package.json)

| Library | Version | Current Usage |
|---------|---------|--------------|
| `@mantine/charts` | 8.3.12 | `AreaChart` (Home.page), `Sparkline` (ONTAP.page, Switches.page) |
| `recharts` | 3.6.0 | `ComposedChart`, `Area`, `Line`, `XAxis`, `YAxis` (Home.page) |

No new dependencies needed for basic charting.

### 2.3 System Prompt

The system prompt (`agent.go:BuildSystemPrompt`) currently instructs the LLM to:
- Use markdown formatting (tables, code blocks)
- Be concise but thorough
- Explain metrics and interpret results

It says **nothing** about when or how to produce visual/structured data suitable for charting.

### 2.4 MCP Data Sources

| MCP | Tool | Returns |
|-----|------|---------|
| Harvest | `metrics_range_query` | Time-series data points (timestamp + value arrays) |
| Harvest | `metrics_query` | Point-in-time metric values |
| Harvest | `infrastructure_health` | Structured health summary |
| ONTAP | various `volume_*`, `aggregate_*` | Structured JSON (capacity, status, performance) |
| Grafana | `search_dashboards`, `query_prometheus` | Dashboard links, metric series |

---

## 3. Design Goals

1. **Inline charts in chat** — time-series trends, capacity bars, health grids render directly in the conversation
2. **Zero extra clicks** — charts appear automatically when the data warrants it (no "click to visualize")
3. **LLM-narrated** — the LLM explains what the chart shows, the chart supports the narrative
4. **Graceful degradation** — if the LLM doesn't produce chart markup, you still get readable markdown text
5. **Interest-aware** — the system recognizes *what the user is interested in* and shapes the response with appropriate visualizations

---

## 4. Interest Response System

### 4.1 The Core Idea

Capabilities define **what tools are available**. Interests define **what the user cares about and how to show it**.

An interest describes a topic or concern the user has — fleet health, a volume's status, provisioning a resource. Each interest has an associated response layout: a mini-dashboard of coordinated panels that the LLM assembles when it recognizes the user is interested in that topic. Each interest specifies:

- **What the user cares about** (the topic or workflow)
- **What data to gather** (which tools to call, what queries to run)
- **How to present it** (a composite layout of multiple charts, grids, stats, and lists)
- **What's interactive** (clickable elements that drill down or navigate)

The LLM doesn't just pick "a chart type." It recognizes what the user is interested in and assembles a **structured multi-panel response** — like a purpose-built dashboard — shaped to that interest.

### 4.2 Interest vs. Capability

| Concept | Scope | Example |
|---------|-------|--------|
| **Capability** | What tools are available (harvest, ontap, grafana) | "You can query metrics and manage volumes" |
| **Interest** | What the user is interested in and how to present it | "I'm interested in my fleet's health this morning" |

An interest may span multiple capabilities. The "Morning Coffee" interest needs Harvest tools (for metrics/alerts) *and* possibly ONTAP tools (for capacity). The interest doesn't care which MCP the data comes from — it cares about the *shape of the answer*.

### 4.3 Defined Interests

#### Interest 1: `morning-coffee` — Fleet Health Overview

**Trigger patterns**: Opening question, "how's everything", "any issues", "give me a summary", "what should I look at", "good morning", "show me a summary of my fleet", "fleet summary"

**What the user wants**: A quick executive view of their entire infrastructure. Where are the problems? What needs attention today? Where should I start?

**Response layout** (a composite of panels):

```
┌─────────────────────────────────────────────────────┐
│  ☕ Infrastructure Health Summary                    │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Alert Summary                                      │
│  ┌──────────┬──────────┬──────────┬──────────┐      │
│  │ 🔴 2     │ 🟠 5     │ 🟡 12    │ 🟢 OK   │      │
│  │ Critical │ Warning  │ Info     │ Healthy  │      │
│  └──────────┴──────────┴──────────┴──────────┘      │
│  (each count is clickable → shows list of alerts)   │
│                                                     │
│  Performance Trend (7d)          Capacity Trend (7d)│
│  ┌─────────────────────┐  ┌─────────────────────┐   │
│  │   ╱╲    ╱╲          │  │          ╱──────────│   │
│  │  ╱  ╲──╱  ╲───      │  │     ╱───╱           │   │
│  │ ╱              ╲     │  │ ───╱                │   │
│  │ IOPS / Latency       │  │ Used % across fleet │   │
│  └─────────────────────┘  └─────────────────────┘   │
│                                                     │
│  ⚠️  Top 5 Resources Needing Attention              │
│  ┌──────────────────────────────────────────────┐   │
│  │ vol_prod_db01    │ 94% full  │ 🔴 3 alerts   │   │
│  │ aggr_node02      │ 89% full  │ 🟠 perf trend │   │
│  │ cluster-west     │ latency ↑ │ 🟠 2 alerts   │   │
│  │ vol_logs_archive  │ 91% full  │ 🟡 capacity   │   │
│  │ svm_prod_east    │ 87% full  │ 🟡 1 alert    │   │
│  └──────────────────────────────────────────────┘   │
│  (each row is clickable → triggers resource-status  │
│   interest for that resource)                         │
│                                                     │
│  Brief: "2 critical alerts on vol_prod_db01 (snap-  │
│  shot failure) and cluster-west (node offline).      │
│  Capacity trending toward exhaustion on 3 volumes   │
│  within 14 days. Recommend starting with the        │
│  critical alerts."                                  │
└─────────────────────────────────────────────────────┘
```

**Data sources**:
- `get_active_alerts` → alert counts by severity
- `metrics_range_query` → fleet-wide IOPS/latency trend (7d)
- `metrics_range_query` → fleet-wide capacity used % trend (7d)
- `metrics_query` → top volumes/aggregates by utilization + alert count

**Panels produced** (as `chart` blocks in the response):

| Panel | Chart Type | Data |
|-------|-----------|------|
| Alert Summary | `alert-summary` | Counts per severity, clickable |
| Performance Trend | `area` | IOPS + latency over 7 days |
| Capacity Trend | `area` | Used % over 7 days |
| Top 5 At-Risk | `resource-table` | Name, metric, status, alert count — clickable rows |

---

#### Interest 2: `resource-status` — Cluster or Volume Deep Dive

**Trigger patterns**: "tell me about volume X", "how is cluster Y", "status of [resource]", "what's going on with [name]", clicking a row in the morning-coffee top-5 list

**What the user wants**: A detailed operational view of a single resource — performance, capacity, and alerts all in one place. Like opening a Grafana dashboard for that one thing.

**Response layout**:

```
┌─────────────────────────────────────────────────────┐
│  📊 vol_prod_db01 — Status                          │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Performance (24h)                                  │
│  ┌─────────────────────────────────────────────┐    │
│  │ IOPS ──── R/W KB/s ─── Latency              │    │
│  │   ╱╲         ╱╲                              │    │
│  │  ╱  ╲──╱╲──╱  ╲───                          │    │
│  │ ╱              ╲                             │    │
│  │  (3 series on one area chart)                │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  Capacity (30d)                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │                        ╱──────── 94%         │    │
│  │               ╱───────╱                      │    │
│  │  ────────────╱                               │    │
│  │  Used %   ─── Projection: full in ~14 days   │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  Alerts (7d trend)           Active Alerts          │
│  ┌───────────────────┐  ┌───────────────────────┐   │
│  │ ╱╲                │  │ 🔴 Snapshot failed     │   │
│  │╱  ╲──╱╲───        │  │    2h ago              │   │
│  │    alert count/day │  │ 🔴 Space nearly full   │   │
│  └───────────────────┘  │    ongoing              │   │
│                         │ 🟠 High latency         │   │
│                         │    4h ago               │   │
│                         └───────────────────────┘   │
│                                                     │
│  Brief: "vol_prod_db01 is at 94% capacity with a   │
│  linear growth trend — projected full in ~14 days.  │
│  2 critical alerts: snapshot failure (likely caused  │
│  by low space) and space warning. Latency elevated  │
│  in the last 4 hours, correlating with high write   │
│  throughput. Recommend: expand volume or move data   │
│  to free space, then investigate snapshot policy."   │
└─────────────────────────────────────────────────────┘
```

**Data sources**:
- `metrics_range_query` → IOPS, read/write KB/s, latency over 24h
- `metrics_range_query` → capacity used % over 30d
- `metrics_range_query` → alert count per day over 7d
- `get_active_alerts` (filtered by resource) → current alert list

**Panels produced**:

| Panel | Chart Type | Data |
|-------|-----------|------|
| Performance | `area` (multi-series) | IOPS, R/W KB/s, latency — 24h |
| Capacity | `area` + projection line | Used % — 30d, with trend extrapolation |
| Alert Trend | `sparkline` or small `area` | Alert count/day — 7d |
| Active Alerts | `alert-list` | Severity, message, timestamp |

---

#### Interest 3: `volume-provision` — Smart Volume Placement

**Trigger patterns**: "provision a volume", "create a volume", "I need a new volume with", "allocate storage for"

**What the user wants**: Provision a new volume with specific characteristics (size, performance tier, protocol, etc.). Rather than blindly picking a cluster, the system should **justify its recommendation** with data — show the user *why* a particular cluster is a good fit, then let them execute with one click.

**Response layout**:

```
┌─────────────────────────────────────────────────────┐
│  📦 Volume Provisioning — 2 TB NFS, high-perf       │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Top Candidate Clusters                             │
│  ┌─────────────────────────────────────────────┐    │
│  │ cluster-east │ cluster-west │ cluster-south  │    │
│  │              │              │                │    │
│  │  Capacity (30d)                              │    │
│  │  ╱───── 62%  │  ╱──── 78%  │  ╱──── 84%    │    │
│  │ ╱            │ ╱           │╱               │    │
│  │ (headroom)   │ (moderate)  │ (tight)        │    │
│  │              │              │                │    │
│  │  Performance (7d)                            │    │
│  │  ── 12K IOPS │ ── 38K IOPS│ ── 45K IOPS    │    │
│  │  (low load)  │ (moderate)  │ (near limit)   │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  ★ Recommendation: cluster-east                     │
│  ┌─────────────────────────────────────────────┐    │
│  │ Best fit: 38% free capacity (7.6 TB avail-  │    │
│  │ able), low IOPS utilization (12K of 80K),   │    │
│  │ NFS enabled, same datacenter as requestor.   │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  Proposed Command                                   │
│  ┌─────────────────────────────────────────────┐    │
│  │ volume create -vserver svm_prod_east         │    │
│  │   -volume vol_app_data                       │    │
│  │   -aggregate aggr1_east                      │    │
│  │   -size 2TB                                  │    │
│  │   -security-style unix                       │    │
│  │   -junction-path /vol_app_data               │    │
│  │   -policy default                            │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │  [ ✅ Provision on cluster-east ]             │    │
│  │  [ 🔄 Show other options ]                   │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  Brief: "cluster-east is the best fit for a 2 TB   │
│  high-performance NFS volume. It has 38% free       │
│  capacity with a flat growth trend — no risk of     │
│  exhaustion within 90 days. Performance headroom    │
│  is 85% (12K of 80K IOPS). cluster-west would      │
│  work but is trending toward 85% capacity in ~30    │
│  days. cluster-south is not recommended — capacity  │
│  and performance both near limits."                 │
└─────────────────────────────────────────────────────┘
```

**Data sources**:
- `metrics_range_query` → capacity used % trend per cluster (30d)
- `metrics_range_query` → IOPS trend per cluster (7d)
- `metrics_query` → current available space, max IOPS, protocol support per cluster
- ONTAP tools → aggregate/SVM details for the proposed command

**Panels produced**:

| Panel | Chart Type | Data |
|-------|-----------|------|
| Candidate Clusters — Capacity | `area` (multi-series or small multiples) | Used % over 30d per cluster |
| Candidate Clusters — Performance | `area` (multi-series) | IOPS over 7d per cluster |
| Recommendation | `callout` | Text explaining the best pick |
| Proposed Command | `proposal` | The ONTAP CLI / API command to execute |
| Action Buttons | `action-button` | "Provision on cluster-east" (executes), "Show other options" (conversational) |

**Key interaction**: The "Provision" action button is wired to the existing **action confirmation flow**. Clicking it doesn't execute immediately — it produces an inline confirmation card in the chat (`ActionConfirmation` component) where the user reviews the action, then approves or denies. No popup modals — everything stays in the conversation. This reuses the existing `PendingApproval` / `approveAction` / `denyAction` machinery.

The "Show other options" button is conversational — it injects a follow-up message like "Show me provisioning options on cluster-west instead."

---

### 4.4 Interest Response as a Composite `dashboard` Block

Individual `chart` blocks work for single charts. But interests produce **multiple coordinated panels** — a mini-dashboard. We need a composite container type.

The LLM emits a single `dashboard` code block that contains an array of panels:

````
```dashboard
{
  "title": "☕ Infrastructure Health Summary",
  "panels": [
    {
      "type": "alert-summary",
      "title": "Alerts",
      "data": {
        "critical": 2,
        "warning": 5,
        "info": 12,
        "ok": 48
      }
    },
    {
      "type": "area",
      "title": "Performance Trend (7d)",
      "width": "half",
      "xKey": "time",
      "series": [
        { "key": "iops", "label": "IOPS", "color": "blue" },
        { "key": "latency_ms", "label": "Latency (ms)", "color": "orange" }
      ],
      "data": [ ... ]
    },
    {
      "type": "area",
      "title": "Capacity Trend (7d)",
      "width": "half",
      "xKey": "time",
      "series": [
        { "key": "used_pct", "label": "Used %", "color": "red" }
      ],
      "data": [ ... ]
    },
    {
      "type": "resource-table",
      "title": "Top 5 — Needs Attention",
      "columns": ["Resource", "Metric", "Status"],
      "rows": [
        { "name": "vol_prod_db01", "metric": "94% full", "status": "critical", "alerts": 3 },
        ...
      ]
    }
  ]
}
```
````

The `dashboard` block is a **first-class type** alongside individual `chart` blocks. The frontend renders it as a cohesive card with a grid layout.

#### Dashboard Toggle

A dashboard JSON may optionally include a `toggle` field:

```json
{
  "title": "Fleet Health Overview",
  "toggle": { "label": "Show Detailed", "message": "show me a per cluster view of my fleet" },
  "panels": [ ... ]
}
```

When present, `DashboardBlock` renders a clickable `Badge` next to the title. Clicking it calls `onAction(toggle.message)`, which injects that message into the chat — triggering a different interest. This enables paired interests (e.g., summary ↔ detailed) to cross-link without custom UI logic. The `DashboardData` TypeScript interface includes an optional `toggle?: { label: string; message: string }` field.

#### Preview Label

The chat drawer header displays "Feature in Preview – Experimental" in red text (`c="red"`) to the right of the "host application Assistant" title. This uses a Mantine `Text` component inside the existing `Group` and is intended to be removed when the chatbot exits preview status.

#### Object-List Interest

The `object-list` interest triggers on tabular queries like "show me volumes", "list aggregates", "top clusters", or "volumes over 80%". It produces a dashboard with:

- **resource-table** (`width: "full"`) — Clickable rows with inline sparklines for capacity and IOPS trends. Default 10 results. The column MUST be named "Capacity" (never "Used") so the sparkline alias system matches `capacity_trend` data.
- **action-button** (`width: "full"`) — Pagination ("Show next 10") when all requested results are returned.

#### Alias-Aware Sparkline Matching

`findInlineTrend()` in `ResourceTableBlock` resolves sparkline trend data using `COLUMN_ALIASES`. If the column is named "Used" but the row contains `capacity_trend`, the alias lookup maps "Used" → capacity group → `capacity_trend`. This makes inline sparklines resilient to LLM column-naming variations.

**Dashboard blocks break out of the chat bubble.** A multi-panel dashboard needs horizontal room — constraining it inside a normal message bubble wastes space, especially when users go full-screen. Dashboard blocks render edge-to-edge within the chat scroll area, not inside a bubble. Text before and after the dashboard stays in normal chat bubbles. Standalone single `chart` blocks remain inside bubbles — they're small enough to fit naturally.

### 4.5 Panel Width & Layout

Panels have an optional `width` field:

| Value | Behavior |
|-------|----------|
| `"full"` (default) | Full width of the chat panel |
| `"half"` | Two panels side-by-side (flex row) |
| `"third"` | Three across (for stat blocks) |

The frontend uses CSS flexbox/grid to arrange panels within the dashboard card.

### 4.6 Clickable Elements & Drill-Down

Some panels have interactive elements:

- **Alert summary counts**: Clicking a severity count triggers a follow-up message (e.g., "Show me the 2 critical alerts") — the frontend injects a user message into the chat.
- **Resource table rows**: Clicking a row triggers the `resource-status` interest for that resource — again by injecting a message like "Show me the status of vol_prod_db01."
- **Action buttons (execute)**: Clicking a "do it" button (e.g., "Provision on cluster-east") triggers the existing **action confirmation flow** — an inline `ActionConfirmation` card appears in the chat stream where the user reviews, approves or denies. No popup modals — everything stays in the conversation. Requires read-write mode.
- **Action buttons (conversational)**: Clicking a conversational button (e.g., "Show other options") injects a follow-up message into the chat.
- **Chart points**: Hover tooltip only (no click action for now).

Drill-down and conversational buttons are **chat-message-based** — clicking injects a user message, the LLM responds naturally. Execute buttons go through the existing `PendingApproval` / `approveAction` / `denyAction` machinery.

### 4.7 New Chart Types for Interests

In addition to the basic chart types (Section 5.2), interests introduce:

#### `alert-summary` — Severity count badges
```json
{
  "type": "alert-summary",
  "data": {
    "critical": 2,
    "warning": 5,
    "info": 12,
    "ok": 48
  }
}
```
Renders as colored badge/count tiles. Each is clickable.

#### `resource-table` — Clickable resource list
```json
{
  "type": "resource-table",
  "title": "Top 5 — Needs Attention",
  "columns": ["Resource", "Metric", "Status"],
  "rows": [
    { "name": "vol_prod_db01", "metric": "94% full", "status": "critical", "alerts": 3 }
  ]
}
```
Renders as a compact table with status-colored rows. Clicking a row sends a chat message.

#### `alert-list` — Active alerts with details
```json
{
  "type": "alert-list",
  "items": [
    { "severity": "critical", "message": "Snapshot failed", "time": "2h ago" },
    { "severity": "critical", "message": "Space nearly full", "time": "ongoing" },
    { "severity": "warning",  "message": "High latency", "time": "4h ago" }
  ]
}
```

#### `callout` — Highlighted recommendation or explanation
```json
{
  "type": "callout",
  "icon": "★",
  "title": "Recommendation: cluster-east",
  "body": "Best fit: 38% free capacity (7.6 TB available), low IOPS utilization (12K of 80K), NFS enabled, same datacenter as requestor."
}
```
Renders as a visually distinct card (colored left border, icon, bold title) for key recommendations or explanations.

#### `proposal` — Proposed command to execute
```json
{
  "type": "proposal",
  "title": "Proposed Command",
  "command": "volume create -vserver svm_prod_east -volume vol_app_data -aggregate aggr1_east -size 2TB -security-style unix -junction-path /vol_app_data -policy default",
  "format": "ontap-cli"
}
```
Renders as a syntax-highlighted code block with a distinct "proposed action" visual treatment (dashed border, label). Not directly executable — the action button triggers execution.

#### `action-button` — Clickable action triggers
```json
{
  "type": "action-button",
  "buttons": [
    {
      "label": "Provision on cluster-east",
      "action": "execute",
      "tool": "ontap_volume_create",
      "params": { "vserver": "svm_prod_east", "volume": "vol_app_data", "size": "2TB" },
      "icon": "✅",
      "variant": "primary"
    },
    {
      "label": "Show other options",
      "action": "message",
      "message": "Show me provisioning options on other clusters",
      "icon": "🔄",
      "variant": "outline"
    }
  ]
}
```

Two action modes:
- **`execute`**: Triggers the action confirmation flow. The `tool` and `params` fields are passed to the backend as if the LLM had made a tool call requiring approval. Requires read-write mode; button is disabled in read-only mode.
- **`message`**: Injects the `message` text as a new user message in the chat. Purely conversational.

### 4.8 Interest Catalog

Interests are **text files** — markdown with YAML frontmatter — that teach the LLM what to show for a given topic. They share a common file format, loading mechanism, index, and `get_interest` tool. But they come in two tiers with different authoring precision and consistency expectations:

#### Two Tiers, One Architecture

| | Bespoke (Built-in) | User-Defined |
|---|---|---|
| **Authored by** | Us — iterated, tested, refined across releases | User — via chat conversation or file drop |
| **Body style** | Prescriptive — names specific chart types, widths, tool hints, exact layout order | Descriptive — prose about what to show; LLM picks the rendering details |
| **Consistency** | High — precise instructions produce the same layout every time | Variable — LLM interprets prose and may choose differently between runs |
| **Editable by user** | No — read-only, ships embedded in the binary | Yes — create, edit, delete via chat or filesystem |
| **Ships with product** | Yes (`//go:embed interests/`) | No — stored on disk at `/etc/host application/interests/` |
| **Count** | 3 now, grows deliberately with product releases | Up to user (capped at ~10 for index budget) |
| **Quality bar** | Lighthouse — these define what "good" looks like | Best-effort — works well, may not be pixel-perfect |

The LLM doesn't know or care which tier it's reading. It processes the interest body alongside the chart format spec (Section 5) and the connected tools, then builds the dashboard. More precise instructions → more consistent output. The architecture is identical; the difference is purely in how carefully the interest is written.

#### Interest File Format

Every interest — bespoke or user-defined — is a markdown file with YAML frontmatter:

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
  - what should I look at
requires:
  - harvest
---

(body — instructions for the LLM)
```

The **frontmatter** provides structured metadata the backend needs:
- `id` — unique identifier, used for the index and `get_interest` lookup
- `name` — human-readable label for the index
- `source` — `"builtin"` or `"user"` (provenance tracking; prevents users from overwriting built-ins)
- `triggers` — phrases that signal the user is interested in this topic
- `requires` — which MCP capabilities must be enabled for this interest to appear

The **body** is where the two tiers diverge.

#### Bespoke Interests (Built-in)

Bespoke interests ship as embedded files (`//go:embed interests/`) inside the chat-service binary. Their bodies are **prescriptive** — we specify chart types by name, panel widths, tool references, layout order, and behavioral rules. This eliminates LLM interpretation variance and produces a consistent, polished experience.

```
interests/
  morning-coffee.md       ← Fleet health overview
  resource-status.md      ← Cluster/volume deep dive
  volume-provision.md     ← Smart volume placement
```

Example — `morning-coffee.md`:

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
  - what should I look at
requires:
  - harvest
---

When the user wants an overall health check of their infrastructure,
produce a `dashboard` code block with the following panels in this order:

1. alert-summary (full width) — Call get_active_alerts. Show counts grouped
   by severity: critical, warning, info, ok. Each count is clickable — clicking
   injects a message like "Show me the 2 critical alerts."

2. area (half width) — Call metrics_range_query for fleet-wide IOPS and latency
   over the last 7 days. Title: "Performance Trend (7d)". Two series: IOPS
   and latency.

3. area (half width) — Call metrics_range_query for fleet-wide capacity used %
   over the last 7 days. Title: "Capacity Trend (7d)". One series: used %.

4. resource-table (full width) — Call metrics_query to find the top 5
   volumes/aggregates by utilization combined with alert count. Columns:
   Resource, Key Metric, Status, Alerts. Each row is clickable — clicking
   injects a message like "Show me the status of vol_prod_db01."

End with a brief text summary (outside the dashboard block) that highlights:
- Which critical alerts need immediate attention and why
- Any capacity or performance trends that are concerning
- A recommended starting point for the user's morning
```

Example — `resource-status.md`:

```markdown
---
id: resource-status
name: Cluster or Volume Deep Dive
source: builtin
triggers:
  - tell me about [resource]
  - how is [resource]
  - status of [name]
  - what's going on with [name]
requires:
  - harvest
---

When the user asks about a specific cluster, volume, aggregate, or SVM,
produce a `dashboard` code block with the following panels:

1. area (full width) — Call metrics_range_query for the resource's IOPS,
   read/write throughput (KB/s), and latency over the last 24 hours.
   Title: "Performance (24h)". Three series on one chart.

2. area (full width) — Call metrics_range_query for the resource's capacity
   used % over the last 30 days. Title: "Capacity (30d)". Include a
   projection annotation estimating when capacity will be exhausted based
   on the trend.

3. sparkline (half width) — Call metrics_range_query for the resource's alert
   count per day over the last 7 days. Title: "Alert Trend (7d)".

4. alert-list (half width) — Call get_active_alerts filtered to this resource.
   Show severity, description, and time for each active alert.

End with a brief analysis (outside the dashboard block) that:
- Correlates observations across panels (e.g., "capacity at 94% and snapshot
  failures are likely related — low space prevents snapshot creation")
- Identifies the root issue if one is apparent
- Gives a concrete recommendation for what to do next
```

Example — `volume-provision.md`:

```markdown
---
id: volume-provision
name: Smart Volume Placement
source: builtin
triggers:
  - provision a volume
  - create a volume
  - I need a new volume with
  - allocate storage for
requires:
  - harvest
  - ontap
---

When the user wants to create or provision a new volume:

First, analyze the user's message to extract requirements: size, protocol
(NFS/CIFS/iSCSI), performance tier, and any other constraints. If critical
requirements are missing, ask before proceeding.

Then query candidate clusters/aggregates for available capacity, current IOPS
utilization, and protocol support. Select the top 3 candidates.

Produce a `dashboard` code block with the following panels:

1. area (half width) — Capacity trend (30d) for the top 3 candidate clusters
   as multi-series. Title: "Candidate Capacity (30d)".

2. area (half width) — Performance trend (7d) for the top 3 candidate clusters
   as multi-series. Title: "Candidate Performance (7d)".

3. callout (full width) — Recommendation explaining which cluster is the best
   fit and why. Reference capacity headroom, performance headroom, protocol
   support, and datacenter proximity.

4. proposal (full width) — The proposed ONTAP CLI command to create the volume
   on the recommended cluster. Use the `ontap-cli` format.

5. action-button (full width) — Two buttons:
   - Primary: "Provision on [cluster-name]" with action "execute", tool
     "ontap_volume_create", and the appropriate params. This MUST go through
     the existing action confirmation flow (read-write mode required).
   - Outline: "Show other options" with action "message", injecting
     "Show me provisioning options on other clusters."

End with a brief text summary comparing the candidates and justifying the
recommendation.
```

Notice the difference from user-defined interests: these name specific chart types (`alert-summary`, `area`, `sparkline`, `alert-list`, `callout`, `proposal`, `action-button`), specify widths (`half`, `full`), reference specific tools (`get_active_alerts`, `metrics_range_query`), and dictate layout order. The LLM follows these as instructions, not suggestions. This is what makes them consistent.

#### User-Defined Interests

User-defined interests live on disk at `/etc/host application/interests/` and use the **same file format** but with a **descriptive** body — the user says *what* they want to see, and the LLM decides *how* to render it.

Example — a user creates `backup-status.md`:

```markdown
---
id: backup-status
name: Backup Health Check
source: user
triggers:
  - backup status
  - how are my backups
  - any failed backups
requires:
  - harvest
---

When the user asks about backups:

Show a dashboard with:
- The status of all volume backups — whether each is current or overdue,
  and when the last successful backup ran
- A chart of backup sizes by volume
- Any failed backup jobs with the failure reason and when they failed

End with a summary of how many backups are current vs. overdue and what
needs attention.
```

No chart type names, no width specifications, no tool references. The LLM reads this, looks at the chart format spec (Section 5), and makes its own choices — maybe a `status-grid` for backup status, a `bar` chart for sizes, an `alert-list` for failures. It might choose differently on another run. That variability is acceptable for user-defined interests — the user got a dashboard that shows what they asked for, even if the exact chart types vary.

#### The Authoring Spectrum

The two tiers aren't a hard binary — they're ends of a spectrum:

```
Pure prose                                           Fully prescriptive
(user types freely)     ←─────────────────────→     (we specify everything)

"show me backup         "show a status-grid of      "1. status-grid (full)
 statuses and            volumes with backup          — Call metrics_query
 any failures"           times, a bar chart of        for backup_last_time.
                         sizes, and an alert-list      Columns: Volume, Last
                         of failures"                  Backup, Status."
     ▲                        ▲                            ▲
  User-defined           LLM-assisted save            Bespoke built-in
  (typed by user)        (LLM refines at save time)   (hand-crafted by us)
```

When a user creates an interest via chat (Path B), the LLM can help move them rightward on this spectrum — generating a more structured body that names chart types and suggests widths — without the user needing to know the vocabulary. This improves consistency for user-defined interests without requiring technical knowledge. See Section 4.10 for details.

#### Compact Index in the System Prompt

The system prompt does **not** include the full interest definitions. Instead, it includes a compact index (~30-50 tokens per interest) and a reference to the `get_interest` tool:

```
## Response Interests

You have a catalog of predefined response layouts for common topics. When you
recognize from the user's message that one of these interests is relevant,
call get_interest(id) to retrieve the full description before gathering data
and composing your response.

| ID | Name | Triggers |
|----|------|----------|
| morning-coffee | Fleet Health Overview | opening questions, "how's everything", "any issues", "summary" |
| resource-status | Cluster/Volume Deep Dive | "tell me about [resource]", "status of [name]" |
| volume-provision | Smart Volume Placement | "provision a volume", "create a volume" |
| backup-status | Backup Health Check | "backup status", "how are my backups" |

You are not required to use an interest — if the user's question is simple or
doesn't match any interest, just answer normally. Interests are for complex
topics that benefit from a multi-panel dashboard layout.
```

This index scales to dozens of interests at minimal token cost. Only the relevant interest's full text enters the context (via `get_interest`), only when needed.

#### The `get_interest` Tool

The backend exposes a simple internal tool:

```
get_interest(id: string) → string
```

Returns the full markdown body of the interest file (everything below the frontmatter). The LLM calls this when it decides an interest is relevant, reads the prose, then proceeds to gather data and compose the dashboard response. This is a local operation — it reads a file from the embedded or on-disk catalog, no network call.

#### Interest Loading & Filtering

At startup (or when triggered by a management operation), the backend:
1. Loads embedded built-in interests from `interests/*.md`
2. Loads user-defined interests from `/etc/host application/interests/*.md` (if any)
3. Parses frontmatter (ID, triggers, requires)
4. Filters by enabled capabilities — an interest with `requires: [harvest, ontap]` is excluded from the index if either MCP is not connected
5. Builds the compact index for the system prompt
6. Registers the `get_interest` tool

**Index rebuild triggers:**
- **`save_interest` / `delete_interest` tool calls** → immediate rebuild. The updated index takes effect for the current conversation and all subsequent ones.
- **Manual file drop** (Path A) → picked up on the next session start. Users who drop files directly don't get instant feedback, but this is the power-user path — they know to start a new conversation.
- **Startup** → always performs a full rebuild.

The Go types are minimal — just enough to parse frontmatter and build the index:

```go
type InterestMeta struct {
    ID       string   `yaml:"id"`
    Name     string   `yaml:"name"`
    Source   string   `yaml:"source"`
    Triggers []string `yaml:"triggers"`
    Requires []string `yaml:"requires"`
}

type Interest struct {
    Meta InterestMeta
    Body string // Raw markdown body — returned by get_interest
}
```

No `InterestPanel`, no `Type`/`Width`/`Tools` fields. The body is opaque text that only the LLM reads.

### 4.9 How Interest Context Flows

```
User message: "How's everything looking?"
    │
    ▼
System prompt includes:
  ├── Base instructions (existing)
  ├── Connected data sources (existing)
  ├── Chart format spec (Section 5 — vocabulary of panel types + JSON schemas)
  └── Interest INDEX (compact table of IDs + triggers + get_interest tool)
    │
    ▼
LLM reads user message + system prompt
    │
    ├── Recognizes "how's everything?" → matches morning-coffee triggers
    ├── Calls get_interest("morning-coffee")
    │     └── Returns: the prose body of morning-coffee.md
    ├── Reads the interest description + chart format spec
    ├── Follows panel instructions (bespoke) or chooses chart types (user-defined)
    ├── Calls tools: get_active_alerts, metrics_range_query (x2), metrics_query
    ├── Assembles results into a `dashboard` code block
    └── Adds narrative text summary
    │
    ▼
Frontend receives assistant message with ```dashboard block
    │
    ├── DashboardBlock component parses the JSON
    ├── Renders panels in grid layout
    ├── Wires click handlers (inject follow-up messages)
    └── Renders surrounding markdown text normally
```

The key difference from the prior design: the full interest description only enters the context **when the LLM decides it's relevant**, via a tool call. The system prompt stays slim regardless of catalog size.

The LLM does the interest matching — we don't need a classifier. The compact index gives it trigger patterns; when it sees a match, it calls `get_interest` to get the full description. For bespoke interests, the LLM then follows precise panel-by-panel instructions. For user-defined interests, it reads the prose and makes its own rendering choices from the chart format vocabulary. Either way, the flow is the same.

### 4.10 Creating & Managing Interests

#### For Us (Developers)

We author bespoke interest files in `interests/` in the source tree — prescriptive bodies with specific chart types, widths, tools, and layout order. These are iterated against real LLM output until the dashboard consistently matches our quality bar. Embedded into the binary at build time.

#### For Users

Two paths, depending on technical comfort:

**Path A — Drop a file.** Place a `.md` file in `/etc/host application/interests/`. Same format as built-in interests, but typically with a descriptive (not prescriptive) body. Users comfortable with YAML frontmatter and markdown can do this directly. The system picks it up on next index rebuild.

**Path B — Describe it in chat.** The user tells the chatbot:

> *"Save a new interest: when I ask about backup status, show me the status of all volume backups, a chart of backup sizes, and any failures."*

The LLM does the heavy lifting:

1. **Infers metadata** — picks an id (`backup-status`), name (`Backup Health Check`), trigger phrases, and `requires` list from the user's request. The LLM knows which MCPs are connected and which tools belong to each, so it can deduce that "volume backups" implies `ontap`, "metrics over time" implies `harvest`, etc. No need to ask the user about MCP dependencies.
2. **Refines the body** — takes the user's description and writes a clearer, more structured version that names chart types and suggests widths based on the chart format spec. This moves the interest rightward on the authoring spectrum (see 4.8) without requiring the user to know the vocabulary:

```markdown
When the user asks about backups:

Show a dashboard with:
1. status-grid (full width) — Status of all volumes: volume name, last
   successful backup time, current/overdue indicator
2. bar (full width) — Backup sizes by volume
3. alert-list (full width) — Failed backup jobs with failure reason
   and timestamp

End with a summary of how many backups are current vs. overdue and what
needs attention.
```

3. **Shows the user** what it generated and asks for confirmation: *"Here's what I'll save as the `backup-status` interest — does this look right?"* This preview is also where the LLM surfaces **data-availability gaps** — if the interest references data that no currently-connected tool can provide, the LLM flags it: *"Note: I don't have a tool that provides backup failure details right now, so that panel would be empty until a backup MCP is connected."* The user sees exactly what they'd get and can revise or save as-is.
4. On confirmation, calls `save_interest` to persist the file to `/etc/host application/interests/`
5. Triggers an immediate interest index rebuild — the new interest appears in the current and all subsequent conversations

The LLM-assisted refinement step is key: the **user** writes prose, the **LLM** translates it into a semi-structured body that will produce more consistent dashboards. The user never needs to learn chart type names — but the saved interest file uses them, which improves repeatability.

**Editing:** *"Update my backup-status interest to also include a chart of backup duration over time."* The LLM fetches the existing interest body, modifies it to add the new panel, shows the user the updated version, and on confirmation calls `save_interest` to overwrite.

**Deleting:** *"Delete the backup-status interest."* Maps to a `delete_interest` tool call. Cannot delete built-in interests.

**Listing:** *"What interests do I have?"* The LLM can answer from the compact index already in the system prompt, distinguishing built-in from user-defined.

#### UX Considerations

- **Admin UI settings page (future):** A "My Interests" section in settings could list user-defined interests with view/edit/delete actions. Editing opens the markdown in a simple text area. This is a nice-to-have — chat-based management works from day one.
- **Built-in interests are visible but not editable:** The settings page (or "list interests" response) shows built-in interests as read-only, so users can see what ships with the product and understand the format.
- **Progressive disclosure:** Users start by creating interests via chat (Path B). Power users who like the format can graduate to direct file editing (Path A). Both paths produce the same file format.

#### Guardrails

- Cap the number of user-defined interests (e.g., 10 — each adds ~30 tokens to the index)
- Validate frontmatter on save: `id` and `requires` are mandatory, `id` must be unique, `requires` must list valid capability names
- User interests cannot use a built-in interest's ID
- `save_interest` and `delete_interest` require read-write mode (same as action-button execute)
- The LLM **always** shows the generated interest to the user before saving — no silent writes
- **Data availability is surfaced at confirmation time**, not enforced as a hard block. If an interest asks for data no tool can source, the LLM warns the user in the preview. The user can still save it — maybe the tool will be connected later. At query time, the LLM gracefully omits panels it can't populate and explains why.

---

## 5. Visualization Data Contract

### 5.1 Chart Block Format

The LLM emits a fenced code block with language `chart`:

````
```chart
{
  "type": "area",
  "title": "Volume Used Capacity — Last 7 Days",
  "xKey": "time",
  "series": [
    { "key": "used_pct", "label": "Used %", "color": "blue" }
  ],
  "data": [
    { "time": "Mar 1", "used_pct": 72 },
    { "time": "Mar 2", "used_pct": 74 },
    { "time": "Mar 3", "used_pct": 73 },
    { "time": "Mar 4", "used_pct": 76 },
    { "time": "Mar 5", "used_pct": 78 }
  ]
}
```
````

### 5.2 Schema Per Chart Type

#### `area` — Time-series trend
```json
{
  "type": "area",
  "title": "string",
  "xKey": "string (field name for x-axis)",
  "yLabel": "string (optional, y-axis label)",
  "series": [
    { "key": "string", "label": "string", "color": "string (optional)" }
  ],
  "data": [ { "xKey": "value", "seriesKey": "number" }, ... ]
}
```

#### `bar` — Comparison
```json
{
  "type": "bar",
  "title": "string",
  "xKey": "string",
  "series": [
    { "key": "string", "label": "string", "color": "string (optional)" }
  ],
  "data": [ ... ]
}
```

#### `gauge` — Single utilization value
```json
{
  "type": "gauge",
  "title": "string",
  "value": 82,
  "max": 100,
  "unit": "%",
  "thresholds": { "warning": 80, "critical": 95 }
}
```

#### `sparkline` — Compact inline trend
```json
{
  "type": "sparkline",
  "title": "string (optional)",
  "data": [72, 74, 73, 76, 78],
  "color": "string (optional)"
}
```

#### `status-grid` — Multi-resource health
```json
{
  "type": "status-grid",
  "title": "string",
  "items": [
    { "name": "vol_data01", "status": "ok" },
    { "name": "vol_data02", "status": "warning", "detail": "87% full" },
    { "name": "vol_logs",   "status": "critical", "detail": "offline" }
  ]
}
```

#### `stat` — Single prominent value
```json
{
  "type": "stat",
  "title": "string",
  "value": "1.2 TB",
  "subtitle": "Available capacity",
  "trend": "up | down | flat (optional)",
  "trendValue": "+3.2% (optional)"
}
```

---

## 6. Chart Components

### 6.1 Component Mapping

| Chart Type | Mantine / Recharts Component | Notes |
|------------|------------------------------|-------|
| `area` | `AreaChart` from `@mantine/charts` | Already used in Home.page |
| `bar` | `BarChart` from `@mantine/charts` | Available, not yet used |
| `sparkline` | `MiniSparkline` — custom SVG (replaced Mantine) | 0-based Y axis, jsdom-compatible, alias-aware trend lookup |
| `gauge` | `RingProgress` from `@mantine/core` | Built-in Mantine component |
| `status-grid` | Custom: `SimpleGrid` + `Badge` / `ThemeIcon` | Straightforward composition |
| `stat` | Custom: `Text` + `Group` + optional `ThemeIcon` | Big number display |
| `alert-summary` | Custom: `Group` + colored count badges | Clickable — injects chat message |
| `resource-table` | Custom: `Table` + status-colored rows | Clickable rows — injects chat message |
| `alert-list` | Custom: severity icon + text list | Compact alert detail view |
| `callout` | Custom: `Paper` + colored left border + icon | Recommendation / explanation highlight |
| `proposal` | Custom: syntax-highlighted `Code` with label | Proposed command (not directly executable) |
| `action-button` | Custom: `Button` group | Execute (triggers confirmation) or conversational (injects message) |
| `dashboard` | `DashboardBlock` — grid container for panels | Arranges panels by `width` field |
| *(fallback)* | `AutoJsonBlock` — generic table/properties renderer | Last-resort for unrecognized JSON |

### 6.2 Proposed Component Structure

```
src/components/ChatPanel/
  charts/
    DashboardBlock.tsx      ← Parses dashboard JSON, arranges panels in flex grid
    ChartBlock.tsx           ← Dispatcher for standalone chart code blocks
    AreaChartBlock.tsx       ← Wraps Mantine AreaChart
    BarChartBlock.tsx        ← Wraps Mantine BarChart
    SparklineBlock.tsx       ← Wraps Mantine Sparkline
    GaugeBlock.tsx           ← Wraps Mantine RingProgress
    StatusGridBlock.tsx      ← Custom SimpleGrid + Badge
    StatBlock.tsx            ← Big number display
    AlertSummaryBlock.tsx    ← Severity count badges (clickable)
    ResourceTableBlock.tsx   ← Clickable resource list
    AlertListBlock.tsx       ← Active alerts with severity + time
    CalloutBlock.tsx         ← Highlighted recommendation / explanation card
    ProposalBlock.tsx        ← Proposed command with syntax highlighting
    ActionButtonBlock.tsx    ← Execute (confirmation flow) or conversational buttons
    AutoJsonBlock.tsx         ← Generic fallback: renders unknown JSON as table/properties
    chartTypes.ts            ← TypeScript interfaces for all JSON schemas
```

`ChartBlock` and `DashboardBlock` are registered as code-block handlers in the ReactMarkdown `components` prop:

```tsx
components={{
  code({ className, children }) {
    const content = String(children).replace(/\n$/, '');
    if (className === 'language-dashboard')
      return <DashboardBlock json={content} onAction={sendMessage} />;
    if (className === 'language-object-detail')
      return <ObjectDetailBlock json={content} onAction={sendMessage} />;
    if (className === 'language-chart')
      return <ChartBlock json={content} onAction={sendMessage} />;
    // Fallback: try parsing as JSON → typed renderers → AutoJsonBlock
    if (className === 'language-json' || !className) {
      const parsed = JSON.parse(sanitizeJson(content));
      // ... try DashboardBlock, ObjectDetailBlock, ChartBlock first ...
      if (typeof parsed === 'object' && parsed !== null)
        return <AutoJsonBlock json={content} />;
    }
    return <code className={className}>{children}</code>;
  }
}}
```

The `onAction` prop lets `DashboardBlock` pass click events (alert count clicks, resource row clicks) back up to the chat — `sendMessage` injects a new user message into the conversation.

#### Streaming Behavior

Dashboard blocks are **not** rendered incrementally. The frontend detects an opening `` ```dashboard `` fence during streaming and shows a placeholder (e.g., a subtle "Assembling dashboard..." skeleton) until the closing fence arrives and the JSON is complete. Then `DashboardBlock` parses and renders all panels at once. Surrounding markdown text streams normally before and after the block. This avoids the complexity and fragility of partial JSON parsing — and the dashboard typically follows tool calls that already took several seconds, so the brief wait is natural.

---

## 7. Rendering Architecture

### 7.1 Rendering Paths

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
     code block handler chain:          known data shape?
     ┌──────┬──────────┬──────┐          │          │
  dashboard chart  obj-detail │        YES         NO
     │       │       │        │          │          │
 Dashboard ChartBlock Object  │     Mini chart   Plain text
  Block   (single)  Detail   │                   (lineClamp 3)
                    Block    │
                             │
                     structured JSON?
                      │          │
                     YES         NO
                      │          │
                  AutoJsonBlock  <code>
                  (table or      (plain)
                   properties)
```

**Path 1 — Dashboard blocks (interest responses)**: The LLM recognizes an interest and emits a `dashboard` code block with multiple panels. `DashboardBlock` renders a grid of chart/status/table components. Clickable elements inject follow-up messages into the chat.

**Path 2 — Standalone chart blocks**: For simpler questions that don't match a full interest, the LLM emits a single `chart` code block. `ChartBlock` renders one chart inline.

**Path 3 — Object-detail blocks**: For questions about a single named entity, the LLM emits an `object-detail` code block. `ObjectDetailBlock` renders a rich detail view with sections.

**Path 4 — Generic JSON fallback (`AutoJsonBlock`)**: When the LLM emits JSON that doesn't match any known chart, dashboard, or object-detail schema, the frontend renders it as a formatted table or key-value property list rather than dumping raw JSON text. This ensures the user always sees a reasonable visual representation regardless of how the LLM structures its response. The detection applies to both fenced code blocks (` ```json `) and bare inline JSON. `AutoJsonBlock` is the final fallback — it only activates after all typed renderers have had a chance.

This fallback is intentionally design-free: it auto-detects the shape and picks the best generic layout. As dedicated components are developed for specific data types (e.g., a proper alert list view), those components take precedence and the fallback gracefully cedes control.

**Path 5 — ToolStatusCard auto-visualization**: When a tool result contains structured data that matches known patterns (e.g., time-series from `metrics_range_query`), the `ToolStatusCard` renders a small inline sparkline. Automatic — no LLM formatting needed.

### 7.2 Backend Role

For LLM-generated blocks (dashboards, charts), the backend does **not** pre-process tool results into chart format. The tool results flow through as-is and the LLM is responsible for deciding when to produce a `chart` block and shaping the data.

**Exception — bespoke render tools** (see [architecture §5.6](chatbot-architecture.md#56-bespoke-render-tools)): For single-object detail views like `render_volume_detail`, the Go handler fetches time-series chart data (IOPS, latency, capacity trends) directly from VictoriaMetrics via a `MetricsFetcher` interface. The LLM passes only scalar properties; the handler queries VictoriaMetrics and populates chart sections server-side. This avoids the LLM having to shuttle large time-series arrays through tool call arguments.

The frontend has three additional auto-detection paths:

1. **ToolStatusCard heuristic**: Detects array-of-objects with numeric fields → sparkline
2. **ChartBlock parser**: Validates the JSON schema and renders the appropriate chart
3. **AutoJsonBlock fallback**: Any structured JSON object that doesn't match a known type is rendered as a formatted table or key-value list (see §7.1 Path 4)

This keeps the backend simple and avoids coupling it to visualization logic.

### 7.3 Generic JSON Fallback Strategy

LLMs are unpredictable in their output formatting. Even with detailed schema instructions in the system prompt, they will sometimes:

- Omit the `type` discriminator field
- Use Prometheus-style field names (`alertname`, `startsAt`) instead of our schema names
- Return valid structured data in a shape we haven't defined a component for yet
- Wrap data in unexpected nesting levels

Rather than playing whack-a-mole with normalizations for every possible field name variation, the frontend uses a **generic fallback renderer** (`AutoJsonBlock`) that handles any structured JSON gracefully:

| JSON Shape | Rendered As |
|---|---|
| Array of objects | Striped table (columns from union of all keys) |
| Object with `items` array of objects | Optional title + table |
| Flat or nested object | Key-value property list |
| Array of primitives | Comma-separated text |
| Primitive value | Plain text |

**Activation criteria**: The `inlineChartDetector` wraps bare inline JSON in a ` ```json ` fence if the object has ≥3 keys or contains a nested array of objects. Trivial objects (e.g., `{"ok": true}`) are left as inline text.

**Precedence**: The fallback only activates after all typed renderers (`DashboardBlock`, `ObjectDetailBlock`, `ChartBlock` with shape inference) have declined the data. As dedicated components are built for specific data types, they automatically take precedence.

**Practical example — alert list data**: When the LLM returns alert data using Prometheus field names (`alertname`, `startsAt`, `instance`) instead of our `alert-list` schema (`message`, `time`), the data flows to `AutoJsonBlock` which renders a clean table with auto-formatted column headers. Once a dedicated alert list view is developed, it will intercept this data first and the fallback will no longer be reached for that shape.

---

## 8. Implementation Phases

### Phase 1 — Chart & Dashboard Renderers (Frontend)

- Build individual chart components: `AreaChartBlock`, `BarChartBlock`, `SparklineBlock`, `GaugeBlock`, `StatBlock`
- Build interest-specific components: `AlertSummaryBlock`, `ResourceTableBlock`, `AlertListBlock`, `CalloutBlock`, `ProposalBlock`, `ActionButtonBlock`
- Build `DashboardBlock` — parses `dashboard` JSON, arranges panels in a grid (half/full/third widths)
- Build `ChartBlock` — dispatcher for standalone `chart` code blocks
- Register `language-chart` and `language-dashboard` handlers in ReactMarkdown
- Wire click handlers: alert-summary counts and resource-table rows inject user messages into the chat
- Wire `action-button` execute mode into existing `ActionConfirmation` / `PendingApproval` flow
- Wire `action-button` message mode to inject follow-up messages
- Disable execute buttons when chat is in read-only mode
- TypeScript interfaces for all JSON schemas
- Unit tests: render each component from JSON fixtures
- **No backend changes. Test by manually crafting dashboard JSON.**

### Phase 2 — Interest Catalog + System Prompt (Backend)

- Write the three built-in interest files: `morning-coffee.md`, `resource-status.md`, `volume-provision.md`
- Implement `InterestMeta` / `Interest` structs (frontmatter + body) and the markdown parser
- Embed built-in interests via `//go:embed interests/`
- Load user-defined interests from `/etc/host application/interests/` at startup
- Filter interests by enabled capabilities
- Build compact index (ID + name + triggers) for the system prompt
- Implement `get_interest` tool — returns the full prose body for a given interest ID
- Extend `BuildSystemPrompt()` to inject:
  - Chart/dashboard format spec (the vocabulary of panel types + JSON schemas)
  - Compact interest index (not full definitions — just the table + reference to `get_interest`)
- **LLM starts producing `dashboard` blocks naturally for matching questions.**

### Phase 2b — Interest Management Tools (Backend)

- Implement `save_interest` tool — persists a user-defined interest file to disk (requires read-write mode)
- Implement `delete_interest` tool — removes a user-defined interest file (requires read-write mode, cannot delete built-in)
- Validate frontmatter on save (unique ID, valid requires, no overwrite of built-in IDs)
- Rebuild interest index after save/delete (no restart required)

### Phase 3 — ToolStatusCard Enhancement

- Auto-detect time-series shapes in `toolResult` → render inline sparkline
- Auto-detect capacity data → render mini gauge
- Expand/collapse toggle: chart view vs. raw JSON
- This is a **bonus** layer — improves tool cards even when the LLM doesn't produce a dashboard block

### Phase 4 — Additional Interests

- Write more built-in interest files as patterns emerge (e.g., `capacity-planning.md`, `alert-investigation.md`, `performance-comparison.md`)
- Each new interest is just a markdown file — no Go code changes, no frontend changes needed if it uses existing panel types
- New panel types only needed for genuinely new visualizations
- Users can create their own interests via chat ("Save a new interest: when I ask about...") or by dropping a file in `/etc/host application/interests/`

---

## 9. Related Work: A2UI

[A2UI](https://a2ui.org/) (Apache 2.0, Google) is a protocol for agent-driven interfaces. Instead of text-only responses, agents send declarative JSON messages describing UI components, and a client-side **renderer** maps them to native widgets (React, Flutter, Angular, Lit). Key properties:

- **Declarative** — agents send component descriptions, never executable code
- **Flat adjacency-list model** — components reference children by ID; easy for LLMs to stream and update incrementally
- **Custom catalogs** — clients register domain-specific components (charts, maps, dashboards) that agents can use alongside the standard catalog (Text, Button, Card, etc.)
- **Separated data model** — UI structure and application state are decoupled via JSON Pointer bindings; agents can update data without resending the whole UI
- **Progressive rendering** — components stream in one at a time and update by ID, so users see the interface building in real-time

### How It Compares to Our Approach

| Aspect | Our Spec | A2UI |
|--------|----------|------|
| Scope | Charts + dashboards inside chat bubbles | Full app-level UIs (forms, modals, tabs, complete screens) |
| Transport | Embedded in the chat stream (markdown code blocks) | Separate protocol layer (SSE, WebSocket, A2A) |
| Component model | Flat panel array in a `dashboard` JSON blob | Adjacency-list tree with ID references, incremental updates |
| Data model | Inline in each panel's JSON | Separated: `dataModelUpdate` messages + JSON Pointer bindings |
| Streaming | Entire `dashboard` block appears when LLM finishes | Incremental: components stream in one-by-one, update by ID |
| Custom components | We build all renderers (Mantine/recharts) | Same — standard catalog is generic primitives; our charts would all be custom catalog entries |

### Assessment

Our approach and A2UI are structurally similar — both are declarative JSON → client-side rendering with type-based dispatch. The differences are in scope and transport. A2UI is designed for agents that build *entire application screens*, while we're building *charts inside a chat conversation*. A2UI's standard catalog doesn't include any of our domain-specific panel types (AreaChart, AlertSummary, ResourceTable, Gauge, etc.) — those would all be custom catalog entries, so we'd still write every renderer ourselves.

**Why not adopt it now:**
- Our transport (markdown code blocks in chat SSE) is simpler than A2UI's surface lifecycle (`surfaceUpdate` → `dataModelUpdate` → `beginRendering` → `deleteSurface`)
- A2UI's React renderer for custom catalogs is not yet fully documented (marked "Coming soon")
- We control both the agent and the client — there's no cross-trust-boundary problem to solve today

**Why it matters for the future:**
- If host application ever exposes its chatbot as an A2A agent, A2UI would be the natural UI protocol for external clients to render our dashboards
- A2UI's incremental rendering (update by component ID) is genuinely better than our "wait for the whole dashboard JSON" model — worth adopting if dashboards grow large
- Migrating later would be evolutionary: our `DashboardBlock` becomes an A2UI renderer with a custom host application catalog; component implementations (Mantine/recharts) stay the same, only the JSON wire format changes

**What we do now to stay compatible:**
- Keep panel descriptions declarative and type-dispatched (already the case)
- Keep rendering logic in a single dispatcher component (`DashboardBlock`) that can be swapped
- Don't embed rendering logic in the agent/backend — keep it client-only

---

## 10. Open Questions

| # | Question | Options | Leaning |
|---|----------|---------|---------|
| 1 | Should the backend normalize MCP results into a chart-friendly intermediate format? | A) No, LLM shapes the data. B) Yes, backend pre-processes. | **Hybrid** — A for LLM-generated dashboards; B for bespoke render tools (§5.6) where the Go handler fetches chart data server-side from VictoriaMetrics |
| 2 | ToolStatusCard expand/collapse? | A) Always show chart, collapse raw text. B) Show both. C) Show chart with "show raw" toggle. | C |
| 3 | Chart interactivity? | A) Static. B) Hover tooltips. C) Click-to-zoom. | B — tooltips come free with recharts/Mantine |
| 4 | Should we limit chart data points? | A) No limit. B) Frontend caps at N points. C) Prompt instructs LLM to limit. | C + B as safety net (e.g., max 200 points) |
| 5 | How do we handle malformed chart JSON? | A) Show raw JSON. B) Show error + raw JSON. C) Silently fall back to code block. | C — treat as regular code block |
| 6 | Do we need chart-type-specific CSS for dark/light mode? | Mantine charts auto-theme, but custom components (status-grid, stat) need explicit token usage. | Yes, use Mantine CSS variables |
| 7 | ~~Should interest hints be user-editable?~~ | ~~A) No, hardcoded. B) Advanced setting.~~ | Resolved — interests are text files, user-editable from day one |
| 8 | Token budget impact? | Chart JSON in the response uses tokens. For large datasets, the LLM may hit limits. | Prompt should say "limit data to key points, max ~50–100 rows" |
| 9 | How does `action-button` execute mode pass tool+params to the backend? | A) Frontend sends a synthetic tool-call approval request. B) Frontend sends a chat message that the LLM interprets as "do it." C) New API endpoint for direct tool execution. | A — reuse existing `PendingApproval` flow. All interactions stay inline in the chat — **no popup modals**. Button click → approval card in chat → user confirms → tool executes → result appears in chat. |
| 10 | Should `volume-provision` auto-detect protocol/tier from the user's request? | A) LLM extracts from natural language. B) Structured form input. | A — the LLM is good at extracting "2 TB NFS high-perf" from prose |
| 11 | How many candidate clusters to show? | A) Top 3. B) Top 5. C) LLM decides. | A — keeps the dashboard compact; LLM can mention others in the text summary |
| 12 | ~~User-defined interest storage format?~~ | ~~A) YAML file. B) JSON in SQLite. C) Part of existing preferences YAML.~~ | Resolved — markdown files with YAML frontmatter, on disk at `/etc/host application/interests/` |
| 13 | Max number of user-defined interests (index budget)? | A) 5. B) 10. C) Unlimited with truncation. | B — 10 user interests + 3+ built-in ≈ 400-600 tokens in the compact index |
| 14 | Should we adopt A2UI as the wire format for dashboard responses? | A) No, keep custom JSON. B) Yes, adopt now. C) Design for migration, adopt when React renderer matures. | C — our JSON shape is already close; keep the door open |
| 15 | How well does the LLM pick chart types from prose descriptions? | Needs testing — the chart format spec gives it the vocabulary, but we may need iteration on how interests are worded to get consistent results. | Test with the 3 built-in interests first; refine wording based on actual LLM output |
| 16 | Should `get_interest` return just the body, or body + frontmatter? | A) Body only (LLM doesn't need metadata). B) Everything (LLM sees triggers for context). | A — the LLM already matched via the index; the body is all it needs for the response layout |

---

## 11. Implementation Plan

This section breaks down the work from Section 8's phases into concrete, ordered tasks with dependencies. Each task is sized to be completable in a single focused session and testable in isolation before moving on.

### 11.1 Milestone 1 — Chart & Dashboard Renderers (Frontend Only)

No backend changes. All testing uses manually crafted JSON fixtures injected as mock assistant messages. The frontend can be developed and demoed entirely independently.

#### Task 1.1 — TypeScript Interfaces & JSON Validation

**What:** Define TypeScript interfaces in `chartTypes.ts` for every panel type (`AreaChartData`, `BarChartData`, `GaugeData`, `SparklineData`, `StatusGridData`, `StatData`, `AlertSummaryData`, `ResourceTableData`, `AlertListData`, `CalloutData`, `ProposalData`, `ActionButtonData`, `DashboardData`). Add a `parseDashboard(json: string)` function that validates JSON against these types and returns a typed result or null on failure (malformed JSON falls back to a plain code block per open question #5).

**Depends on:** Nothing  
**Test:** Unit tests — valid JSON parses correctly, invalid JSON returns null, unknown panel types are skipped gracefully.

#### Task 1.2 — Basic Chart Components

**What:** Build `AreaChartBlock`, `BarChartBlock`, `SparklineBlock`, `GaugeBlock`, `StatBlock`. These wrap existing Mantine/recharts components with the props from our JSON schema. Each takes a typed prop (from `chartTypes.ts`) and renders a chart. Use Mantine CSS variables for dark/light mode.

**Depends on:** Task 1.1  
**Test:** Storybook-style unit tests — render each component from fixture data, snapshot comparison. Verify tooltips render on hover (recharts built-in).

#### Task 1.3 — Interest-Specific Components

**What:** Build `AlertSummaryBlock`, `ResourceTableBlock`, `AlertListBlock`, `CalloutBlock`, `ProposalBlock`. These are custom Mantine compositions (not chart library wrappers). `AlertSummaryBlock` and `ResourceTableBlock` accept an `onAction` callback for click events.

**Depends on:** Task 1.1  
**Test:** Unit tests — render from fixtures. Click handler tests: clicking an alert count calls `onAction` with the expected message string. Clicking a resource-table row calls `onAction` with the resource name.

#### Task 1.4 — ActionButtonBlock

**What:** Build `ActionButtonBlock` with two modes:
- `"message"` mode: clicking calls `onAction(message)` to inject a chat message
- `"execute"` mode: clicking triggers the existing `PendingApproval` flow — the button creates a synthetic approval request and pushes it into the chat as an inline `ActionConfirmation` card. Disabled when chat is read-only.

**Depends on:** Task 1.1, understanding of existing `ActionConfirmation` / `PendingApproval` / `approveAction` / `denyAction` machinery in `ChatPanel.tsx` and `useChatPanel.ts`  
**Test:** Unit tests — message-mode click injects the expected message. Execute-mode click creates a PendingApproval entry. Execute button is disabled in read-only mode.

#### Task 1.5 — ChartBlock Dispatcher

**What:** Build `ChartBlock` — parses a `chart` JSON string, identifies the `type` field, dispatches to the correct sub-component (AreaChartBlock, BarChartBlock, etc.). Returns a plain `<code>` block on parse failure.

**Depends on:** Tasks 1.1, 1.2, 1.3  
**Test:** Unit tests — each chart type dispatches correctly. Malformed JSON renders as code.

#### Task 1.6 — DashboardBlock

**What:** Build `DashboardBlock` — parses a `dashboard` JSON string, renders a grid of panels using CSS flexbox. Panels flow top-to-bottom; `half`-width panels sit side-by-side, `third`-width panels three across, `full` takes a row. Dashboard title rendered as a header. Passes `onAction` to clickable sub-components. Dashboard renders edge-to-edge (not inside a chat bubble).

**Depends on:** Tasks 1.2, 1.3, 1.4, 1.5  
**Test:** Unit tests — renders panels in correct order with correct widths. Clickable elements propagate `onAction`. Full fixture test with a morning-coffee-style dashboard.

#### Task 1.7 — ReactMarkdown Integration & Streaming

**What:** Register `language-chart` and `language-dashboard` code-block handlers in the `ReactMarkdown` `components` prop in `ChatPanel.tsx`. Add streaming placeholder: detect an opening `` ```dashboard `` fence and show an "Assembling dashboard..." skeleton until the closing fence completes. Dashboard blocks render outside the chat bubble; standalone chart blocks stay inside.

**Depends on:** Tasks 1.5, 1.6  
**Test:** Integration test — render a mock assistant message containing a `dashboard` code block surrounded by markdown text. Verify dashboard renders as a DashboardBlock, text renders normally. Test streaming placeholder appears during incomplete blocks.

#### Task 1.8 — Fixture-Based Demo

**What:** Create a set of JSON fixtures matching the three bespoke interests (morning-coffee, resource-status, volume-provision) with realistic synthetic data. Wire a dev-mode toggle or Storybook page that injects these as mock assistant messages so the full rendering pipeline can be demonstrated without a backend.

**Depends on:** Task 1.7  
**Test:** Visual review — dashboards look right with realistic data, click handlers work, breakout layout looks good at various viewport widths.

### 11.2 Milestone 2 — Interest Catalog & System Prompt (Backend)

Frontend is already rendering dashboards from JSON. Now the backend teaches the LLM how to produce them.

#### Task 2.1 — Interest File Parser

**What:** Implement Go types `InterestMeta` and `Interest`. Write a parser that reads a markdown file, splits YAML frontmatter from body, unmarshals frontmatter into `InterestMeta`, and returns an `Interest`. Handle parse errors gracefully (log and skip malformed files).

**Depends on:** Nothing (standalone Go package, likely in `agent/` or a new `interest/` package)  
**Test:** Go unit tests — parse a well-formed interest file, verify all fields. Parse a file with missing required fields, verify error. Parse a file with no frontmatter, verify error.

#### Task 2.2 — Built-in Interest Files

**What:** Write the three bespoke interest markdown files: `morning-coffee.md`, `resource-status.md`, `volume-provision.md`. Bodies are the prescriptive text already specified in Section 4.8. Place them in `agent/interests/` (or wherever the `//go:embed` directive lives).

**Depends on:** Task 2.1 (to verify they parse correctly)  
**Test:** Go unit test — embed and parse all three, verify IDs, triggers, requires, and non-empty bodies.

#### Task 2.3 — Interest Loading, Filtering & Index

**What:** Implement the interest loader:
1. Load embedded built-in interests (`//go:embed interests/`)
2. Load user-defined interests from `/etc/host application/interests/` (if dir exists)
3. Deduplicate by ID (user can't shadow built-in IDs)
4. Filter by enabled capabilities — exclude interests whose `requires` lists capabilities that aren't connected
5. Build the compact index string (markdown table of ID, Name, Triggers)
6. Store the loaded interests in a map for `get_interest` lookups

**Depends on:** Tasks 2.1, 2.2  
**Test:** Go unit tests — load embedded interests with all capabilities enabled → all appear. Disable `ontap` → `volume-provision` excluded. Add a mock user-defined interest file → appears in index. Duplicate ID with built-in → rejected.

#### Task 2.4 — `get_interest` Tool

**What:** Register a new internal tool `get_interest` that accepts an `id` parameter and returns the interest body. This is a local lookup — reads from the in-memory map, no network call. Register it alongside the existing MCP tools in the agent's tool list so the LLM can call it.

**Depends on:** Task 2.3  
**Test:** Go unit test — call with a valid ID, get body back. Call with an unknown ID, get an error message.

#### Task 2.5 — System Prompt Extension

**What:** Extend `BuildSystemPrompt()` in `agent.go` to inject:
1. The chart/dashboard format spec — a condensed version of Section 5 that gives the LLM the vocabulary of panel types and their JSON schemas
2. The compact interest index — the markdown table from Task 2.3
3. Instructions for when and how to use `get_interest` and how to compose dashboard blocks

**Depends on:** Tasks 2.3, 2.4  
**Test:** Go unit test — build a system prompt with mock interests, verify it contains the index table and format spec. Verify that disabling all MCPs produces an empty index section.

#### Task 2.6 — Lighthouse Interest Iteration

**What:** This is the most important task in the plan. The three bespoke interests (`morning-coffee`, `resource-status`, `volume-provision`) are **lighthouse examples** — they define what "good" looks like and set the quality bar for every dashboard the system produces. This task is hands-on iteration with a real LLM and the dev stack running end-to-end.

For each lighthouse interest:
1. Trigger it with representative user messages:
   - morning-coffee: "Good morning, how's everything?", "Any issues?", "Give me a summary"
   - resource-status: "Tell me about vol_prod_db01", "How is cluster-east?", "Status of aggr1"
   - volume-provision: "I need a new 2 TB NFS high-performance volume", "Provision storage for the new app"
2. Verify the LLM calls `get_interest`, then calls the right tools, then produces a valid `dashboard` JSON
3. Verify the frontend renders the dashboard correctly — correct panels in correct order, widths, click handlers
4. **Iterate on the interest body wording** until the output is consistently good across multiple runs. Tighten instructions where the LLM makes bad choices (wrong chart type, missing panel, wrong width). Loosen where instructions are overly rigid.
5. **Iterate on the system prompt format spec** (Section 5 condensed version) if the LLM misunderstands chart type schemas or layout rules
6. Capture the best dashboard JSON outputs as **regression fixtures** — these become the expected-output baselines for Task 1.8's fixture-based demo and future regression tests

This is not a one-pass task. Expect multiple rounds of: run → evaluate → tweak interest body → run again. The lighthouse interests aren't done until they reliably produce polished, correct dashboards across varied phrasings.

**Depends on:** All of Milestone 1 + Tasks 2.1–2.5  
**Test:** Each lighthouse interest produces correct, complete dashboard output for at least 5 varied trigger phrasings across 3+ runs with no manual intervention. Regression fixtures captured.

### 11.3 Milestone 3 — Interest Management Tools (Backend)

Users can create, edit, and delete their own interests via chat.

#### Task 3.1 — `save_interest` Tool

**What:** Register a `save_interest` tool that:
1. Accepts parameters: `id`, `name`, `triggers` (array), `requires` (array), `body` (string)
2. Validates: ID is unique (not a built-in ID), `requires` lists valid capability names, cap not exceeded (≤10 user interests)
3. Assembles the markdown file (YAML frontmatter + body) with `source: user`
4. Writes to `/etc/host application/interests/{id}.md`
5. Triggers an immediate interest index rebuild
6. Returns a success/error message

Requires read-write mode (same gate as action-button execute).

**Depends on:** Task 2.3  
**Test:** Go unit tests — save a valid interest, verify file on disk and index updated. Save with duplicate built-in ID, verify rejected. Save when at cap, verify rejected. Save in read-only mode, verify rejected.

#### Task 3.2 — `delete_interest` Tool

**What:** Register a `delete_interest` tool that:
1. Accepts `id` parameter
2. Rejects deletion of built-in interests
3. Removes the file from `/etc/host application/interests/`
4. Triggers an immediate interest index rebuild
5. Returns a success/error message

Requires read-write mode.

**Depends on:** Task 2.3  
**Test:** Go unit tests — delete a user interest, verify file removed and index updated. Delete a built-in interest, verify rejected. Delete a nonexistent interest, verify appropriate error.

#### Task 3.3 — Chat-Based Interest Creation Flow (LLM-Driven)

**What:** No code change — this is a system prompt refinement. Update the system prompt instructions (Task 2.5) to tell the LLM how to handle interest management requests:
- When the user says "save a new interest: ...", the LLM should infer metadata, refine the body, show the user the result for confirmation, surface data-availability gaps, and only call `save_interest` after explicit approval
- When the user says "update my X interest", fetch via `get_interest`, modify, show for confirmation, save
- When the user says "delete my X interest", confirm, then call `delete_interest`
- When the user says "what interests do I have?", answer from the compact index

**Depends on:** Tasks 3.1, 3.2, 2.5  
**Test:** Manual testing with real LLM — create an interest via chat, verify confirmation flow, verify file saved, verify it works in the next query. Edit and delete via chat.

### 11.4 Milestone 4 — ToolStatusCard Enhancement (Bonus)

Independent from the interest/dashboard system. Can be done in parallel with Milestone 2 or deferred.

#### Task 4.1 — Auto-Detection Heuristics

**What:** Add heuristics to `ToolStatusCard` that detect common data shapes in `toolResult`:
- Array of objects with a timestamp-like field + numeric fields → sparkline
- Single object with a `value` and `max` → mini gauge
- Otherwise → current plain-text behavior

**Depends on:** Tasks 1.2 (chart components exist)  
**Test:** Unit tests — feed representative `toolResult` shapes from Harvest and ONTAP tools, verify correct detection. Non-matching shapes render as plain text.

#### Task 4.2 — Expand/Collapse Toggle

**What:** Add a toggle to `ToolStatusCard` — when auto-visualization is available, default to the chart view with a "Show raw" toggle that reveals the original text. Per open question #2 (option C).

**Depends on:** Task 4.1  
**Test:** Unit test — toggle switches between chart and text views.

### 11.5 Milestone 5 — Polish & Hardening

#### Task 5.1 — Responsive Layout

**What:** Test dashboard rendering at various viewport widths. At narrow widths (< 600px), `half` panels should stack vertically (become `full`). At very wide widths (full-screen), ensure panels don't stretch excessively — add a max-width on the dashboard container.

**Depends on:** Task 1.6  
**Test:** Visual testing at 400px, 768px, 1024px, 1440px, 1920px widths.

#### Task 5.2 — Data Point Limits

**What:** Add a safety net in `ChartBlock`/`DashboardBlock` parsing: if a data array exceeds 200 points, downsample to 200 (pick every Nth point). Also add guidance in the system prompt telling the LLM to limit data to ~50–100 rows.

**Depends on:** Task 1.5  
**Test:** Unit test — chart with 500 data points is downsampled to ≤200.

#### Task 5.3 — Accessibility

**What:** Ensure all chart components have appropriate ARIA labels. Status-grid colors use both color and icon/text indicators (not color alone). Alert severity badges use semantic labels. Action buttons have proper focus management.

**Depends on:** All of Milestone 1  
**Test:** axe-core audit on rendered dashboards.

#### Task 5.4 — Dark/Light Mode

**What:** Verify all custom components (status-grid, stat, alert-summary, callout, proposal, action-button, dashboard container) use Mantine CSS variables for theming. Mantine/recharts chart components auto-theme, but custom compositions need explicit token usage.

**Depends on:** All of Milestone 1  
**Test:** Visual comparison in both themes.

### 11.6 Suggested Order of Work

```
Milestone 1 (Frontend)
  1.1 → 1.2 ──→ 1.5 ──→ 1.6 ──→ 1.7 ──→ 1.8
  1.1 → 1.3 ─┘         ┘
  1.1 → 1.4 ──────────┘

Milestone 2 (Backend — can overlap with late Milestone 1)
  2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6

Milestone 3 (Backend — after Milestone 2)
  3.1 → 3.3
  3.2 ─┘

Milestone 4 (Bonus — can run in parallel with Milestone 2/3)
  4.1 → 4.2

Milestone 5 (Polish — after Milestones 1+2 are working end-to-end)
  5.1, 5.2, 5.3, 5.4 (independent, any order)
```

### 11.7 Definition of Done

Each milestone has a clear "done" state:

| Milestone | Done When |
|-----------|-----------|
| 1 | All chart/dashboard components render correctly from JSON fixtures. Click handlers work. Demo page shows all three bespoke interest layouts with synthetic data. |
| 2 | LLM consistently produces valid `dashboard` JSON for all three bespoke interests. Frontend renders them correctly end-to-end. System prompt includes format spec and compact index. |
| 3 | User can create, edit, delete, and list interests via chat. `save_interest` and `delete_interest` tools work correctly with validation and index rebuild. LLM shows confirmation before saving. |
| 4 | ToolStatusCard auto-renders sparklines/gauges for matching tool results. Toggle works between chart and raw text views. |
| 5 | Dashboards look good at all viewport widths and both themes. Accessibility audit passes. Large datasets are safely capped. |
