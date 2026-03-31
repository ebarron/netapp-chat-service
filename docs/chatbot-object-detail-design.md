# Object-Detail Type & Interest/Type Layering

> **Status:** Ready for implementation — all design decisions resolved  
> **Audience:** Engineers working on host application chatbot  
> **Prerequisite reading:** `docs/chatbot-architecture.md`, `docs/chatbot-graphical-ui-enhancements.md`  
> **Reference:** [Monitor, Alert, and Notify — Detailed Design](https://netapp.atlassian.net/wiki/spaces/UMF/pages/396512661) (Confluence)

---

## 1. Problem Statement

The chatbot has two systems that shape responses: **interests** (input-side) and **chart types** (output-side). Today these are loosely coupled through the LLM's prompt — interests describe what tools to call and what panels to emit, and the type system renders whatever JSON the LLM produces.

This works well for dashboard-style responses (morning-coffee, resource-status, volume-provision) where the output is a grid of independent panels. But there's a gap:

**When a user asks about a single object — a volume, a cluster, an alert — no panel type exists to render a rich, structured detail view.**

The LLM is forced to either:
- Flatten everything into prose (loses structure, hard to scan)
- Use `resource-table` (too tabular, no embedded charts or actions)
- Assemble an ad-hoc `dashboard` with loosely-related panels (no visual cohesion, doesn't feel like "a page about this thing")

We need a panel type purpose-built for **object detail views** — and a design for how interests and types interact to produce them.

### 1.1 Current Layering

```
Interest (input-side)              Type (output-side)
─────────────────────              ──────────────────
"Why is the user talking?"         "How do we render this?"

morning-coffee                     dashboard → [alert-summary, area, area, resource-table]
resource-status                    dashboard → [area, area, sparkline, alert-list]
volume-provision                   dashboard → [area, area, callout, proposal, action-button]
(no interest match)                standalone chart or prose
```

Interests shape *what content* gets produced. Types shape *how* it gets rendered. These are orthogonal axes — an interest doesn't determine the type, and a type doesn't imply an interest.

### 1.2 The Gap

Consider these user messages:

- *"Tell me about the critical InstanceDown alert"*
- *"What's going on with vol_prod_db01?"*
- *"Show me cluster-east details"*

These match the `resource-status` interest, but the desired output isn't a dashboard grid — it's a **detail page** for a single object: identity, status, properties, contextual charts, related alerts, and recommended actions. Today, `resource-status` produces a dashboard because that's the only multi-panel layout available.

The core issue: we have 12 panel types for *parts* of a response but no type for a *cohesive object view*.

---

## 2. Design: Interest ↔ Type Layering

### 2.1 Principle

Interest and type are two independent axes:

```
                        Types (output rendering)
                    ┌────────────────────────────────┐
                    │ dashboard  │ object-detail  │... │
        ┌──────────┼────────────┼────────────────┼────┤
        │ morning  │ fleet      │     —          │    │
Interest│ -coffee  │ overview   │                │    │
(input  ├──────────┼────────────┼────────────────┼────┤
context)│ resource │ multi-panel│ volume detail, │    │
        │ -status  │ comparison │ alert detail   │    │
        ├──────────┼────────────┼────────────────┼────┤
        │ volume   │ candidate  │     —          │    │
        │ -provis. │ comparison │                │    │
        ├──────────┼────────────┼────────────────┼────┤
        │ (none)   │ ad-hoc     │ ad-hoc detail  │    │
        └──────────┴────────────┴────────────────┴────┘
```

**Interest** = input-side context. It tells the LLM *why* the user is talking and *what angle* to take — what tools to activate, what data to gather, what narrative to provide. An interest can produce *any* output type.

**Type** = output-side rendering. It tells the frontend *how* to present the data — grid of independent panels, rich object page, standalone chart, or prose. A type can be used by *any* interest (or no interest at all).

**The interest shapes content *within* a type, not the type itself.** A `resource-status` interest about a volume produces an `object-detail` with volume-specific properties and charts. The same `resource-status` interest about a fleet-wide question might produce a `dashboard` grid instead. The interest provides the data-gathering strategy and narrative voice; the type provides the visual structure.

### 2.2 Refactoring the Current Design

Today's interests hardcode `dashboard` as their output type. The refactoring:

1. **Interests become type-agnostic** — the interest body describes *what to show*, not *what type to use*. The LLM picks the appropriate type based on the question context.

2. **Interests can suggest types** — for prescriptive interests (built-in), the body can recommend a type: "If the user asks about a specific resource, use `object-detail`. If they ask about the fleet, use `dashboard`." This is a suggestion, not a mandate — the LLM exercises judgment.

3. **The `object-detail` type is added** as a new panel/layout type alongside `dashboard` and standalone `chart`.

4. **The system prompt's chart format spec expands** to document `object-detail` alongside `dashboard`, so the LLM knows both layout vocabularies.

### 2.3 What Changes, What Stays

| Component | Change |
|-----------|--------|
| **Interest files** | Minor — add guidance on when to use `object-detail` vs `dashboard`. Body text stays mostly the same. |
| **System prompt** | Add `object-detail` to the chart format spec. Add routing guidance: "For questions about a single entity, prefer `object-detail`. For fleet-wide overviews, prefer `dashboard`." |
| **Type system** (`chartTypes.ts`) | Add `ObjectDetailData` interface and parser. |
| **Frontend rendering** | Add `ObjectDetailBlock` component. Register `language-object-detail` as a new code fence language. |
| **Backend** | No changes — the backend doesn't know or care about panel types. |
| **Inline chart detector** | Extend `classify()` to recognize object-detail shapes. |
| **`inferChartType()`** | Add object-detail shape detection. |

---

## 3. The `object-detail` Type

### 3.1 Design Principles

The `object-detail` type renders a **rich, cohesive view of a single entity** — a volume, cluster, aggregate, alert, SVM, or any other domain object. It is:

- **Generic** — same type for any kind of object. The `kind` field tells the renderer what it's looking at, but the rendering logic is the same regardless of kind.
- **Data-driven** — the page is entirely assembled from the JSON the LLM produces. No object-kind-specific code paths in the renderer. This mirrors the data-driven design principle from the alert monitoring Confluence page: *"the page is entirely data-driven — no alert-type-specific code."*
- **Composable** — sections embed existing panel types (sparklines, area charts, alert-lists, action-buttons) rather than inventing new primitives.
- **Scannable** — optimized for quick comprehension: identity at the top, key metrics front-and-center, detail below, actions at the bottom.

### 3.2 Schema

```json
{
  "type": "object-detail",
  "kind": "alert | volume | cluster | aggregate | svm | string",
  "name": "InstanceDown — node-east-01",
  "status": "critical | warning | ok | info | string",
  "subtitle": "Firing since 2025-06-14 09:32 UTC (4h 28m)",
  "qualifier": "on SVM svm1 on cluster cls1",

  "sections": [
    {
      "title": "Section Title",
      "layout": "properties | chart | alert-list | timeline | actions | text | table",
      "data": { ... }
    }
  ]
}
```

The top-level fields provide **identity** — what object this is, its current state, and a summary line. The `sections` array contains the detail, each with a `layout` that maps to a specific rendering style.

#### Qualifier — Identity Context for Follow-Up Actions

The top-level `qualifier` string carries the identity keys needed to uniquely look up this object in follow-up requests. The UI automatically appends it to action messages (button clicks, property link clicks) from this detail view. Examples by kind:

| Kind | Qualifier | Rationale |
|------|-----------|----------|
| volume | `"on SVM vdbench on cluster cls1"` | Volume names are only unique within an SVM+cluster |
| svm | `"on cluster cls1"` | SVM names are unique within a cluster |
| aggregate | `"on cluster cls1"` | Aggregate names are unique within a cluster |
| alert | `"(alert-id abc123)"` | Alerts need their unique identifier |
| cluster | omit or `""` | Cluster name alone is unique |

**Per-item qualifier overrides.** Property items and action buttons support an optional `qualifier` field that **overrides** the card-level qualifier for that specific link. This is essential when a link targets a *different kind* of object whose identity keys differ from the current object:

- `"qualifier": ""` (empty string) — suppress the qualifier entirely. Use for links to clusters from any detail card.
- `"qualifier": "on cluster cls1"` — use only cluster context. Use for links to SVMs or aggregates from a volume detail card.
- Omit `qualifier` on the item — inherit the card-level qualifier. Use for same-kind follow-ups (e.g. "Show snapshots" on a volume card).

**Action button messages must include the entity's kind and name.** Every action button `message` should reference the current entity explicitly (e.g. `"Show aggregates for volume vol01"`, `"Show SVMs on cluster cls1"`). This ensures the resulting message — after qualifier appending — is unambiguous. Never reference only a parent entity (SVM, cluster) when the action pertains to the current entity.

Example: A volume detail card with `qualifier: "on SVM svm1 on cluster cls1"` might have these property items:

```json
{"label": "Cluster", "value": "cls1", "link": "Show cluster cls1", "qualifier": ""}
{"label": "SVM", "value": "svm1", "link": "Tell me about SVM svm1", "qualifier": "on cluster cls1"}
{"label": "Aggregate", "value": "aggr1", "link": "Tell me about aggregate aggr1", "qualifier": "on cluster cls1"}
```

Without per-item overrides, clicking "Cluster" would produce `"Show cluster cls1 on SVM svm1 on cluster cls1"` — nonsensical because clusters don't belong to SVMs.

### 3.3 Section Layouts

Each section `layout` determines how its `data` is rendered:

#### `properties` — Key-value grid

```json
{
  "title": "Alert Details",
  "layout": "properties",
  "data": {
    "columns": 2,
    "items": [
      { "label": "Severity", "value": "critical", "color": "red" },
      { "label": "Alert Name", "value": "InstanceDown" },
      { "label": "Impact", "value": "Data access may be degraded" },
      { "label": "Firing Since", "value": "2025-06-14 09:32 UTC" },
      { "label": "Cluster", "value": "cluster-east", "link": "Tell me about cluster-east", "qualifier": "" },
      { "label": "Node", "value": "node-east-01" }
    ]
  }
}
```

Renders as a 2-column grid of label/value pairs. Optional `color` for status values. Optional `link` injects a chat follow-up message on click. Optional `qualifier` overrides the card-level qualifier for this specific link (see §3.2 *Qualifier*). Linked values are styled in the accent color with no underline at rest; on hover, an underline appears (Mantine `Anchor` with `underline="hover"`). This provides soft discoverability without cluttering the grid.

#### `chart` — Embedded chart

```json
{
  "title": "Metric Trends",
  "layout": "chart",
  "data": {
    "type": "area",
    "xKey": "time",
    "series": [{ "key": "value", "label": "Used %", "color": "blue" }],
    "data": [{ "time": "Jun 10", "value": 72 }, ...],
    "annotations": [
      { "y": 90, "label": "Threshold", "color": "red", "style": "dashed" }
    ]
  }
}
```

Embeds any existing chart type (area, bar, sparkline, gauge, etc.) as a section. The `data` field uses the same schema as standalone chart blocks, plus optional `annotations` for threshold lines. The renderer delegates to the existing chart components.

#### `alert-list` — Related alerts

```json
{
  "title": "Active Alerts",
  "layout": "alert-list",
  "data": {
    "items": [
      { "severity": "critical", "message": "InstanceDown — node-east-01", "time": "4h ago" },
      { "severity": "warning", "message": "HighLatency — vol_prod_db01", "time": "2h ago" }
    ]
  }
}
```

Renders using the existing `AlertListBlock` component.

#### `timeline` — Chronological events

```json
{
  "title": "Timeline",
  "layout": "timeline",
  "data": {
    "events": [
      { "time": "09:32", "label": "Alert fired", "severity": "critical" },
      { "time": "09:35", "label": "Notification sent to #ops-alerts", "icon": "notification" },
      { "time": "09:40", "label": "Auto-remediation attempted", "icon": "action" },
      { "time": "10:15", "label": "Escalation: no acknowledgment after 45m", "severity": "warning" }
    ]
  }
}
```

Renders as a vertical timeline with time labels, event descriptions, and optional severity coloring — a new frontend component but a simple Mantine composition (vertical stepper or custom timeline). When more than 10 events are present, the timeline collapses after the first 10 with a "Show N more" toggle. Entity names and event details within timeline entries should be clickable, injecting follow-up chat prompts where appropriate.

#### `actions` — Action buttons

```json
{
  "title": "Actions",
  "layout": "actions",
  "data": {
    "buttons": [
      { "label": "Investigate Node", "action": "message", "message": "What's happening on node-east-01?" },
      { "label": "Silence Alert (4h)", "action": "execute", "tool": "silence_alert", "params": {"alertname": "InstanceDown", "duration": "4h"}, "variant": "outline" },
      { "label": "Acknowledge", "action": "execute", "tool": "acknowledge_alert", "params": {"alertname": "InstanceDown"}, "variant": "outline" }
    ]
  }
}
```

Renders using the existing `ActionButtonBlock` component. Each button supports an optional `qualifier` field to override the card-level qualifier for that button's action message (see §3.2 *Qualifier*).

#### `text` — Free-form markdown

```json
{
  "title": "Recommended Actions",
  "layout": "text",
  "data": {
    "body": "1. Check node connectivity: `system node show`\n2. Review recent config changes...\n3. If node is unreachable, initiate failover..."
  }
}
```

Renders markdown text via ReactMarkdown — used for recommendations, analysis, or narrative sections.

#### `table` — Data table

```json
{
  "title": "Notifications Sent",
  "layout": "table",
  "data": {
    "columns": ["Time", "Channel", "Recipients", "Status"],
    "rows": [
      { "Time": "09:33", "Channel": "Slack", "Recipients": "#ops-alerts", "Status": "Delivered" },
      { "Time": "09:33", "Channel": "Email", "Recipients": "oncall@corp.com", "Status": "Delivered" }
    ]
  }
}
```

Renders using the existing `ResourceTableBlock` component (or a simplified variant without click actions).

### 3.4 Frontend Rendering

`ObjectDetailBlock.tsx` renders the `object-detail` type:

```
┌─────────────────────────────────────────────────┐
│ ● InstanceDown — node-east-01                    │  ← status dot + name
│   Firing since 2025-06-14 09:32 UTC (4h 28m)   │  ← subtitle
├─────────────────────────────────────────────────┤
│ Alert Details                                    │  ← section: properties
│ ┌──────────────────┬──────────────────┐         │
│ │ Severity   crit. │ Alert    InstDwn │         │
│ │ Impact     ...   │ Firing   09:32   │         │
│ │ Cluster    east  │ Node     east-01 │         │
│ └──────────────────┴──────────────────┘         │
├─────────────────────────────────────────────────┤
│ Metric Trends                          [▾ 24h]  │  ← section: chart
│ ┌───────────────────────────────────────┐       │
│ │  ╱╲    ╱╲                             │       │
│ │ ╱  ╲──╱  ╲───── threshold ─ ─ ─ ─    │       │
│ │╱        ╲╱                            │       │
│ └───────────────────────────────────────┘       │
├─────────────────────────────────────────────────┤
│ Timeline                                        │  ← section: timeline
│ ○ 09:32  Alert fired                            │
│ ○ 09:35  Notification sent to #ops-alerts       │
│ ○ 09:40  Auto-remediation attempted             │
│ ● 10:15  Escalation: no acknowledgment          │
├─────────────────────────────────────────────────┤
│ Recommended Actions                             │  ← section: text
│ 1. Check node connectivity: `system node show`  │
│ 2. Review recent config changes...              │
├─────────────────────────────────────────────────┤
│ [Investigate Node]  [Silence (4h)]  [Ack]       │  ← section: actions
└─────────────────────────────────────────────────┘
```

The component:
1. Renders the identity header (status badge + name + subtitle)
2. Iterates sections, dispatching each to the appropriate layout renderer
3. Uses existing chart/alert/action components for embedded content
4. One new component needed: `TimelineSection` (vertical event list)
5. Entry point: register `language-object-detail` as a dedicated code fence language in ReactMarkdown

### 3.5 Navigation Paradigm

`object-detail` is always a top-level code fence block — never embedded inside a `dashboard` panel. The navigation model is **drill-down via follow-up prompts**:

- **Dashboard → object-detail:** Clicking a row in a `resource-table`, an item in an `alert-list`, or an entity name in any panel injects a follow-up chat prompt (e.g., "Tell me about vol_prod_01"), which triggers the LLM to produce an `object-detail` response.
- **Object-detail → object-detail:** Clicking a linked value in a `properties` grid or an entity name in a `timeline` event injects another follow-up prompt, drilling into a related entity.
- **Object-detail → dashboard:** An action button with a `message` action can ask a fleet-wide question, returning to a dashboard view.

This mirrors standard UI drill-down (list → detail → related detail) but expressed entirely through the chat conversation. Every clickable element is a potential conversation turn.

### 3.6 How the LLM Produces It

The LLM emits an `object-detail` code block the same way it emits `dashboard` or `chart`:

````
```object-detail
{
  "type": "object-detail",
  "kind": "alert",
  "name": "InstanceDown — node-east-01",
  "status": "critical",
  "subtitle": "Firing since ...",
  "sections": [ ... ]
}
```
````

The system prompt's chart format spec documents the schema. Interest bodies guide the LLM on what sections to include and what data to gather for each.

---

## 4. Alerts as Lighthouse Use Case

Alerts are the first use case for `object-detail`. They exercise every section layout, have a well-defined target design (the Confluence Alert Details page), and are immediately useful — "tell me about the critical alert" is a natural thing to ask the chatbot.

### 4.1 Target: Alert Details Page

The Confluence [Monitor, Alert, and Notify — Detailed Design](https://netapp.atlassian.net/wiki/spaces/UMF/pages/396512661) defines an **Alert Details Page** with these sections:

```
┌──────────────────────────────────────────────────────────┐
│ HEADER                                                   │
│ [severity badge] alertname — impact                      │
│ Firing since: timestamp (duration)                       │
├──────────────────────────────────────────────────────────┤
│ VOLUME / OBJECT INFORMATION                              │
│ Properties grid: cluster, SVM, volume, labels,           │
│ expected threshold vs actual value                        │
├──────────────────────────────────────────────────────────┤
│ RECOMMENDED ACTIONS                                      │
│ From corrective_action annotation in alert rule          │
├──────────────────────────────────────────────────────────┤
│ ACTION BAR                                               │
│ [Fix-It] [Investigate] [Acknowledge] [Silence]           │
├──────────────────────────────────────────────────────────┤
│ TIMELINE                                                 │
│ Chronological: alerts fired, config changes,             │
│ notifications sent, remediation attempts                 │
├──────────────────────────────────────────────────────────┤
│ METRIC TRENDS                                            │
│ Primary metric chart with threshold line +               │
│ metric picker for multiple related metrics               │
├──────────────────────────────────────────────────────────┤
│ NOTIFICATIONS SENT                                       │
│ Table: channel, recipients, timestamp, status,           │
│ source attribution                                       │
└──────────────────────────────────────────────────────────┘
```

**Key design principle from Confluence:** *"The page is entirely data-driven. The same component renders any alert type — InstanceDown, VolumeSpaceFullPercent, HighLatency, SnapmirrorLagTime. No alert-type-specific code paths."*

### 4.2 Mapping to `object-detail`

Every section of the Confluence Alert Details page maps directly to an `object-detail` section layout:

| Alert Details Section | `object-detail` Layout | Notes |
|----------------------|----------------------|-------|
| Header | Top-level `kind`, `name`, `status`, `subtitle` | Kind = "alert" |
| Object Information | `properties` (2-column grid) | Cluster, SVM, volume, threshold vs actual |
| Recommended Actions | `text` (markdown) | From alert rule's `corrective_action` annotation |
| Action Bar | `actions` (buttons) | Investigate, Silence, Acknowledge |
| Timeline | `timeline` (chronological events) | Alert events, notifications, config changes |
| Metric Trends | `chart` (area with annotations) | Primary metric + threshold line |
| Notifications Sent | `table` | Channel, recipients, time, status |

This is a 1:1 mapping. No new section layouts needed beyond what Section 3.3 defines. The `object-detail` type is sufficient to render the full alert detail view as designed in Confluence.

### 4.3 Data Assembly

The Confluence document defines a **data assembly pipeline** for alert details:

```
Alert trigger (from AlertManager/VictoriaMetrics)
    │
    ├── Context lookup: match alert → rule definition
    │     └── Source: Harvest defaults, user-created, or storage-class-generated
    │     └── Fields: thresholds, corrective_action, severity, labels
    │
    ├── Metric resolution: query recent values for the firing metric
    │     └── Source: VictoriaMetrics via Harvest MCP
    │
    ├── Object hierarchy: resolve cluster → SVM → volume → aggregate
    │     └── Source: ONTAP MCP
    │
    ├── Audit log: recent config changes and events
    │     └── Source: ONTAP MCP (EMS events)
    │
    └── Notification history: what was sent, when, to whom
          └── Source: AlertManager API via Harvest MCP
```

In the chatbot, the LLM orchestrates this pipeline via tool calls. The interest body tells the LLM what data to gather and in what order; the `object-detail` type tells the frontend how to render it.

### 4.4 Example: Alert Interest Body (Updated)

The `resource-status` interest needs to handle the alert case. Currently it produces a dashboard grid for all resources. After this refactoring, it would route to `object-detail` for single-entity deep dives:

```markdown
When the user asks about a specific alert, produce a fenced code block with
language tag `object-detail` containing:

kind: "alert"
name: "<alertname> — <target>"
status: severity from the alert
subtitle: "Firing since <timestamp> (<duration>)"

Sections (in this order):

1. properties — Alert metadata. Two columns. Include: Severity, Alert Name,
   Impact (from annotations.description), Cluster, SVM, Volume (from labels),
   Expected (threshold from rule expr), Actual (current metric value).
   Make Cluster and Volume values clickable (link to "Tell me about <name>").

2. text — Recommended Actions. Get the corrective_action annotation from
   the alert rule. If none exists, provide general guidance based on the
   alert type.

3. chart — Metric Trends. Query the firing metric for the last 24h via
   metrics_range_query. Add a threshold annotation line at the alert
   threshold value. Title: "<metric_name> (24h)".

4. timeline — Recent Events. Gather: when the alert first fired, any
   related alerts on the same resource, recent EMS events from the cluster.
   Present chronologically.

5. actions — Three buttons:
   - "Investigate <target>" (message action — deep dive on the resource)
   - "Silence Alert (4h)" (execute action — requires read-write mode)
   - "Acknowledge" (execute action — requires read-write mode)

When the user asks about a specific volume, cluster, or aggregate (not an
alert), produce an object-detail with kind set to the resource type and
sections appropriate to that resource (properties, performance chart,
capacity chart, related alerts, actions). Follow the same section structure
pattern.

When the user asks a fleet-wide question that matches resource-status
triggers (e.g., "how are my clusters?"), produce a dashboard grid instead.
```

This demonstrates the interest/type layering in action: the same interest produces different types based on the user's question. The interest provides data-gathering strategy; the type provides visual structure.

### 4.5 Alert Rule Data Sources

Per Confluence, alert rules come from three sources:

| Source | Location | Example |
|--------|----------|---------|
| **Harvest defaults** | Bundled with Harvest MCP | `VolumeSpaceFullPercent`, `InstanceDown` |
| **User-created** | Manually configured rules | Custom thresholds and expressions |
| **Storage-class-generated** | Auto-generated from storage class definitions | Security, capacity, performance, data protection rules |

Each rule carries annotations (including `corrective_action`) that the LLM can surface in the Recommended Actions section. The chatbot queries active alerts via `get_active_alerts` and can inspect rule definitions to retrieve these annotations.

### 4.6 Storage Class Alert Catalog

The Confluence page defines a catalog of alert rules organized by concern:

- **Security**: Encryption disabled, anti-ransomware off, insecure protocols
- **Capacity**: Volume/aggregate fullness, snapshot reserve, quota exceeded
- **Performance**: High latency, IOPS limits, QoS violations
- **Data Protection**: SnapMirror lag, failed transfers, missing snapshots

Each category maps to specific metrics queryable via the Harvest MCP. The alert `object-detail` view surfaces these through the properties grid (showing which category/rule triggered) and the metric trends chart (showing the relevant metric).

---

## 5. Beyond Alerts: Other Object Types

The `object-detail` type is intentionally generic. After proving the pattern with alerts, the same type handles:

### 5.1 Volume Detail

Volume detail is the first object type implemented with a **bespoke render
tool** (`render_volume_detail`). The LLM gathers all data, then calls the
render tool which deterministically produces the `object-detail` JSON. See
`docs/chatbot-architecture.md` §5.6 for the design rationale and wireframe.

```
kind: "volume"
name: "vol_prod_db01"
status: "warning" (based on capacity/health)
subtitle: "Volume on SVM svm_prod, cluster cluster-east"

sections (always 6, in this order):
  properties  → State, size, used%, aggregate (→), SVM (→), cluster (→),
                style, protocol, snapshot policy, QoS policy, monitoring status
  chart       → Performance (24h) — IOPS read/write + latency (or text fallback)
  chart       → Capacity trend (30d) with warning (85%) and critical (95%) annotations (or text fallback)
  alert-list  → Active alerts on this volume (empty list if none)
  text        → LLM-written health analysis
  actions     → [Stop Monitoring🔒 | Monitor this Volume🔒] [Show Snapshots] [Resize Volume]
                🔒 = requiresReadWrite — disabled in read-only mode
```

**Implementation**: `render/volume.go`, registered in `server/server.go`,
instructed via `interest/interests/volume-detail.md`.

### 5.2 Cluster Detail

```
kind: "cluster"
name: "cluster-east"
status: "ok"
subtitle: "2 nodes | 4 aggregates | ONTAP 9.14.1 | 127 volumes"

sections:
  properties  → Name, version, node count, aggregate count, volume count, location
  chart       → Cluster IOPS + latency (7d)
  chart       → Aggregate capacity utilization (stacked bar)
  table       → Top 5 volumes by utilization
  alert-list  → Active alerts across the cluster
  actions     → [Show nodes] [Show aggregates] [Cluster performance comparison]
```

### 5.3 Design Pattern

Every object type follows the same structure:
1. **Identity** — kind, name, status, subtitle
2. **Context** — properties grid with key metadata
3. **Trends** — embedded charts showing relevant metrics over time
4. **Issues** — related alerts or warnings
5. **Actions** — contextual next steps (investigate, remediate, drill down)

The LLM assembles this from tool calls guided by the interest body. For
bespoke interests (like volume-detail), the LLM calls a dedicated render
tool that guarantees layout consistency. For other object types, the LLM
produces the `object-detail` JSON directly — no frontend code changes are
needed per object type, as `ObjectDetailBlock` renders any `kind` the same way.

---

## 6. Refactoring the `resource-status` Interest

The `resource-status` interest is the primary interest that would use `object-detail`. Here's how it evolves:

### 6.1 Current Behavior

Always produces a `dashboard` grid:
```
area (full) — Performance 24h
area (full) — Capacity 30d
sparkline (half) — Alert trend 7d
alert-list (half) — Active alerts
```

### 6.2 Proposed Behavior

**Single-entity questions** → `object-detail`:
- "Tell me about vol_prod_db01" → volume object-detail
- "What's the critical InstanceDown alert?" → alert object-detail
- "How is cluster-east?" → cluster object-detail

**Fleet-wide questions** → `dashboard`:
- "How are my clusters?" → dashboard with comparison grid
- "Show me all volumes over 80%" → dashboard with resource-table
- "What's the capacity situation?" → dashboard with aggregate charts

The interest body provides routing guidance:

```markdown
If the user is asking about a **specific named entity** (volume, cluster,
aggregate, alert, SVM), produce an `object-detail` view.

If the user is asking a **fleet-wide or comparative question**, produce a
`dashboard` grid.
```

This is a soft instruction — the LLM may choose differently if the context warrants it.

### 6.3 Hard Rule: Always Include Associated Alerts

Regardless of whether the output is `object-detail` or `dashboard`, any resource-status response **must include associated alerts** for the resource(s) in question. This is not optional — if the user asks about a volume, the response includes active alerts on that volume. If the user asks about a cluster, the response includes alerts across that cluster. The LLM should always call alert-querying tools as part of the resource-status data-gathering pipeline.

---

## 7. System Prompt Changes

### 7.1 Chart Format Spec Addition

Add to the `chartFormatSpec` constant in `agent.go`:

```
### Object detail — use language "object-detail"

For questions about a single entity (volume, cluster, alert, SVM, aggregate),
produce a rich detail view:

` ` `object-detail
{
  "type": "object-detail",
  "kind": "volume | cluster | alert | aggregate | svm | string",
  "name": "Display name or title",
  "status": "critical | warning | ok | info",
  "subtitle": "Brief context line",
  "sections": [
    { "title": "Section Title", "layout": "properties|chart|alert-list|timeline|actions|text|table", "data": { ... } }
  ]
}
` ` `

Section layouts:
- **properties**: {"columns": 2, "items": [{"label":"string","value":"string","color":"string (opt)","link":"string (opt, injects chat message)"}]}
- **chart**: Any chart type JSON (area, bar, gauge, sparkline, etc.) + optional "annotations": [{"y":number,"label":"string","color":"string","style":"solid|dashed"}]
- **alert-list**: {"items": [{"severity":"string","message":"string","time":"string"}]}
- **timeline**: {"events": [{"time":"string","label":"string","severity":"string (opt)","icon":"string (opt)"}]}
- **actions**: {"buttons": [ActionButton schema]}
- **text**: {"body": "markdown string"}
- **table**: {"columns": ["Col1",...], "rows": [{...}]}

When to use object-detail vs dashboard:
- Single named entity → object-detail
- Fleet-wide overview or comparison → dashboard
- Ambiguous → prefer object-detail if one entity is the primary focus
```

### 7.2 Routing Guidance

Add a brief paragraph after the interest catalog table:

```
**Output type selection:** When following an interest's instructions, choose
the appropriate output format based on the user's question:
- Questions about a single entity (volume, cluster, alert) → `object-detail`
- Fleet-wide overviews, comparisons, or multi-entity views → `dashboard`
- Simple factual questions → inline chart or prose (no wrapper needed)
```

---

## 8. Implementation Plan

### 8.0 Testing Strategy

All testing follows existing project conventions:

| Layer | Tool | Location | Pattern |
|-------|------|----------|---------|
| **Frontend unit** | Vitest + React Testing Library | `chat-service/frontend/src/**/*.test.{ts,tsx}` | `render()` from `@test-utils`, `describe/it/expect`, `vi.fn()` spies, inline TS fixture objects |
| **Frontend fixtures** | TypeScript objects | `chat-service/frontend/src/components/ChatPanel/charts/fixtures.ts` | Named exports (`morningCoffee`, `resourceStatus`, etc.) validated via parsers |
| **Backend unit** | Go `testing` + `httptest` | `chat-service/**/*_test.go` | Table-driven subtests, `httptest.NewRequest/NewRecorder`, no assertion libraries |
| **E2E** | Playwright | `chat-service/frontend/e2e/*.spec.ts` | `page.route()` mocking all API calls, mock SSE streams, fixture data in `e2e/fixtures/` |
| **E2E fixtures** | TypeScript objects | `chat-service/frontend/e2e/fixtures/mock-llm-responses.ts` | `mockTextResponse`, `mockChartResponse`, `mockDashboardResponse`, etc. |

**Key principle:** Every new component and parser gets unit tests. Every new code fence type gets E2E coverage. Backend prompt/interest changes get Go tests. No phase is complete without its tests passing.

---

### Phase 1 — `object-detail` Frontend Components

**No backend changes.** Build and test using JSON fixtures.

#### 8.1 TypeScript Types & Parsing

Add to `chartTypes.ts`:

```typescript
interface ObjectDetailData {
  type: 'object-detail';
  kind: string;
  name: string;
  status?: string;
  subtitle?: string;
  sections: ObjectDetailSection[];
}

interface ObjectDetailSection {
  title: string;
  layout: 'properties' | 'chart' | 'alert-list' | 'timeline' | 'actions' | 'text' | 'table';
  data: unknown; // validated per-layout at render time
}

interface PropertiesData {
  columns?: number;
  items: PropertyItem[];
}

interface PropertyItem {
  label: string;
  value: string;
  color?: string;
  link?: string;   // injects follow-up chat prompt on click
}

interface TimelineData {
  events: TimelineEvent[];
}

interface TimelineEvent {
  time: string;
  label: string;
  severity?: string;
  icon?: string;
}

interface ChartAnnotation {
  y: number;
  label: string;
  color?: string;
  style?: 'solid' | 'dashed';
}
```

Add parsing: `parseObjectDetail(json: string): ObjectDetailData | null`

Update `inferChartType()` to detect object-detail shape (presence of `kind`, `name`, `sections` array).

**Frontend unit tests** (`chartTypes.test.ts` or new `objectDetail.test.ts`):
- Parse valid object-detail JSON → returns `ObjectDetailData`
- Parse with missing required fields (`name`, `sections`) → returns `null`
- Parse with unknown section layout → still parses (forward-compatible)
- `inferChartType()` recognizes `{kind, name, sections}` shape → `"object-detail"`
- `inferChartType()` does not misclassify dashboard JSON as object-detail

#### 8.2 `ObjectDetailBlock` Component

New component: `chat-service/frontend/src/components/ChatPanel/charts/ObjectDetailBlock.tsx`

Renders:
- Identity header: status badge (colored dot) + name + subtitle
- Section loop: dispatch each section to the appropriate renderer by `layout`
- Section renderers:
  - `properties` → new `PropertiesSection` (2-col CSS grid of label/value pairs, accent-colored linked values with `underline="hover"`)
  - `chart` → delegates to existing `ChartBlock` with annotation support
  - `alert-list` → delegates to existing `AlertListBlock`
  - `timeline` → new `TimelineSection` (vertical event list, collapses after 10 events with "Show N more")
  - `actions` → delegates to existing `ActionButtonBlock`
  - `text` → inline ReactMarkdown
  - `table` → delegates to existing `ResourceTableBlock`

**New sub-components needed:** `PropertiesSection`, `TimelineSection`
**Everything else reuses existing chart components.**

**Frontend unit tests** (`ObjectDetailBlock.test.tsx`):
- Renders identity header: name, status badge color, subtitle
- Renders sections in order: verify DOM order matches `sections` array
- Renders empty sections array: no crash
- `PropertiesSection`: renders label/value pairs in 2-column grid
- `PropertiesSection`: linked values rendered as Mantine `Anchor` with `underline="hover"`
- `PropertiesSection`: clicking linked value calls `onAction` with the `link` string
- `PropertiesSection`: colored values render with correct color
- `TimelineSection`: renders events with time and label
- `TimelineSection`: ≤10 events → all visible, no toggle
- `TimelineSection`: >10 events → first 10 visible, "Show N more" button present
- `TimelineSection`: clicking "Show more" reveals remaining events
- `TimelineSection`: clickable entity names call `onAction`
- Chart section delegates to `ChartBlock` with data passed through
- Alert-list section delegates to `AlertListBlock`
- Actions section delegates to `ActionButtonBlock`
- Text section renders markdown via ReactMarkdown
- Table section delegates to `ResourceTableBlock`

#### 8.3 Registration & Integration

Register in `ChatPanel.tsx`:
- `language-object-detail` code fence → `ObjectDetailBlock`
- Update `inlineChartDetector.ts` to classify object-detail JSON
- Update `inferChartType()` for shape detection

**Frontend unit tests** (`inlineChartDetector.test.ts` — extend existing):
- Bare `{type: "object-detail", kind: "alert", name: "...", sections: [...]}` → classified as object-detail, wrapped in `language-object-detail` fence
- Object-detail JSON inside code fence → not double-wrapped
- Dashboard JSON → still classified as dashboard (no regression)
- Ambiguous JSON without `type` field but with `kind` + `sections` → classified as object-detail

#### 8.4 Fixtures

Add to `chat-service/frontend/src/components/ChatPanel/charts/fixtures.ts`:
- `alertDetail` — Alert object-detail matching Confluence page layout (all 7 section types)
- `volumeDetail` — Volume object-detail with properties, capacity chart, performance chart, alert-list, actions
- `clusterDetail` — Cluster object-detail with properties, charts, top-volumes table, alert-list, actions

**Frontend unit tests** (`fixtures.test.tsx` — extend existing):
- Each fixture parses successfully via `parseObjectDetail()`
- Each fixture renders via `ObjectDetailBlock` without errors
- Click actions on fixtures propagate correctly

---

### Phase 2 — Chart Annotations (Threshold Lines)

Moved before backend changes because the alert object-detail lighthouse (Phase 4) needs threshold lines to validate the full visual.

#### 8.5 `AreaChartBlock` Annotation Support

- Accept optional `annotations` array in chart data
- Render each annotation as a recharts `<ReferenceLine>` (for `y` values) or `<ReferenceArea>` (for ranges)
- Styling: solid or dashed stroke, colored line, small label (keep sparse — 1-2 per chart max for screen real-estate)
- Works for both standalone `area` charts in dashboards and embedded `chart` sections in object-detail

**Frontend unit tests** (`AreaChartBlock.test.tsx` or `ChartBlock.test.tsx`):
- Chart with no annotations → renders normally (no regression)
- Chart with one annotation → `<ReferenceLine>` present in rendered output
- Chart with dashed style annotation → correct stroke-dasharray
- Annotation label text visible
- Multiple annotations render without overlap issues

---

### Phase 3 — Backend: System Prompt & Interest Updates

#### 8.6 System Prompt Extension

Add `object-detail` documentation to `chartFormatSpec` constant in `agent.go`:
- Full schema reference for `object-detail` code fence (as specified in Section 7.1)
- Type selection guidance: single entity → object-detail, fleet-wide → dashboard
- Annotation guidance: limit to 1-2 per chart

**Backend Go tests** (`agent_test.go` or `prompt_test.go`):
- `TestBuildSystemPrompt` — verify prompt output contains `"object-detail"` string
- `TestBuildSystemPrompt` — verify prompt contains all 7 section layout names (`properties`, `chart`, `alert-list`, `timeline`, `actions`, `text`, `table`)
- `TestBuildSystemPrompt` — verify type selection guidance text present
- Verify prompt still contains `dashboard` and `chart` specs (no regression)

#### 8.7 Interest File Updates

Update `resource-status.md` to include:
- Object-detail guidance for single-entity questions (volume, cluster, alert, SVM, aggregate)
- Dashboard guidance for fleet-wide / comparative questions
- **Hard rule: always include associated alerts** for any resource-status response regardless of output type
- Routing hint: "If the user asks about a specific named entity, use `object-detail`. If fleet-wide, use `dashboard`."

No changes to `morning-coffee.md` (always dashboard) or `volume-provision.md` (always dashboard).

**Backend Go tests** (`interest/catalog_test.go` — extend existing):
- Load `resource-status` interest → body contains `"object-detail"` string
- Load `resource-status` interest → body contains alert-inclusion guidance
- All existing interest tests pass (no regression)

#### 8.8 Create `alert-investigation` Interest (Optional)

A new built-in interest specifically for alert deep dives:

```yaml
---
id: alert-investigation
name: Alert Investigation
source: builtin
triggers:
  - tell me about the alert
  - what's this alert
  - investigate alert
  - alert details
requires:
  - harvest
---
```

This is optional — `resource-status` covers alerts too. A dedicated alert interest could provide more prescriptive instructions optimized for the alert use case. Defer until after Phase 4 validates the pattern with `resource-status`.

**Backend Go tests** (if created):
- Interest loads and parses correctly
- Interest triggers match expected phrases
- Interest requires `harvest` capability

---

### Phase 4 — End-to-End: Alert Object-Detail Lighthouse

#### 8.9 E2E Test: Object-Detail Rendering

Add `chat-service/frontend/e2e/object-detail.spec.ts`:

```
Setup:
  - Mock all /api/** routes (catch-all pattern per existing convention)
  - Mock /config (host-integration)config → configured LLM
  - Mock /chat SSE → stream containing object-detail code fence

Tests:
  - "renders alert object-detail from SSE stream"
    → Send mock SSE with alert object-detail JSON fixture
    → Verify: status badge visible, name rendered, subtitle rendered
    → Verify: properties section has label/value pairs
    → Verify: chart section renders SVG (recharts)
    → Verify: timeline section shows events
    → Verify: action buttons present

  - "object-detail links inject follow-up prompts"
    → Render alert object-detail with property links
    → Click a linked value (e.g., cluster name)
    → Verify: chat input populated with follow-up prompt text

  - "timeline collapses when >10 events"
    → Render object-detail with 15 timeline events
    → Verify: only 10 events visible initially
    → Click "Show 5 more"
    → Verify: all 15 events visible

  - "chart annotations render threshold lines"
    → Render object-detail with chart section containing annotations
    → Verify: ReferenceLine element present in chart SVG
```

Add to `chat-service/frontend/e2e/fixtures/mock-llm-responses.ts`:
- `mockObjectDetailResponse` — SSE event containing the `alertDetail` fixture wrapped in a `language-object-detail` code fence

#### 8.10 Alert Object-Detail Integration Testing

The critical manual iteration task: run the full stack with `task run` + `yarn run dev`, trigger real LLM responses, and iterate on:
- Interest body wording until the LLM consistently produces valid object-detail JSON
- Section ordering and content until the output matches the Confluence Alert Details page layout
- Chart annotation rendering (threshold lines) in the metric trends section
- Timeline component UX: collapsibility, clickable entities, severity colors
- Action button wiring: "Investigate" (follow-up prompt), "Silence" / "Acknowledge" (execute actions requiring read-write mode)
- Navigation flow: dashboard → click alert → object-detail → click cluster → cluster object-detail

**Definition of done:** "Tell me about the InstanceDown alert" → renders a rich object-detail view matching the Confluence Alert Details page layout, with working click actions and navigation drill-down, for 5+ varied phrasings across 3+ runs.

#### 8.11 Volume & Cluster Object-Detail Integration Testing

After alerts work, test volume and cluster object-detail views against the full stack:
- "What's going on with vol_prod_db01?" → volume object-detail
- "How is cluster-east?" → cluster object-detail
- Verify: resource-status always includes associated alerts (Section 6.3 hard rule)
- Verify: fleet-wide questions ("how are my clusters?") still produce dashboard, not object-detail

---

### Phase 5 — Navigation Paradigm: Drill-Down Links

Wire up the click-to-drill-down paradigm across existing components so dashboard views link to object-detail views:

#### 8.12 Dashboard → Object-Detail Navigation

- `ResourceTableBlock`: clicking a row injects a follow-up prompt ("Tell me about <entity>")
- `AlertListBlock`: clicking an alert injects "Tell me about the <alertname> alert"
- `AlertSummaryBlock`: clicking a severity category injects "Show me all <severity> alerts"
- Entity names in any panel that refer to a named object → accent-colored, `underline="hover"`, click injects follow-up prompt

**Frontend unit tests** (extend existing component tests):
- `ResourceTableBlock`: clicking row calls `onAction` with expected follow-up prompt string
- `AlertListBlock`: clicking alert item calls `onAction`
- Verify linked text styling: accent color, hover underline

**E2E tests** (extend `chatbot.spec.ts` or new `navigation.spec.ts`):
- Render dashboard with resource-table → click row → verify chat input populated with follow-up
- Render object-detail with property links → click → verify chat input populated

---

### Phase Summary

| Phase | What | Tests | Depends On |
|-------|------|-------|------------|
| 1 | Types, parsing, `ObjectDetailBlock`, `PropertiesSection`, `TimelineSection`, fixtures | Frontend unit (Vitest): ~20 tests | — |
| 2 | Chart annotations (`<ReferenceLine>`) on `AreaChartBlock` | Frontend unit: ~5 tests | — |
| 3 | System prompt expansion, `resource-status` interest update | Backend Go: ~6 tests | — |
| 4 | E2E object-detail rendering, alert lighthouse, volume/cluster integration | E2E (Playwright): ~4 specs + manual integration | Phases 1-3 |
| 5 | Drill-down navigation wiring across dashboard components | Frontend unit: ~3 tests, E2E: ~2 specs | Phase 4 |

> **Note — Generic JSON Fallback**: Alongside the typed rendering pipeline
> (`dashboard` → `object-detail` → `chart`), the frontend includes `AutoJsonBlock`
> as a last-resort renderer for any structured JSON that doesn't match a known
> schema. This means data the LLM returns in an unexpected shape (e.g., alert
> lists with Prometheus-style field names, or entirely novel object structures)
> is rendered as a formatted table or key-value list rather than raw JSON text.
> Dedicated components always take precedence when they exist. See
> §7.3 of `chatbot-graphical-ui-enhancements.md` for the full fallback strategy.

Phases 1, 2, and 3 are independent and can be worked in parallel. Phase 4 requires all three. Phase 5 is independent of Phase 4 at the unit level but E2E tests need Phase 4.

---

## 9. Design Decisions

| # | Question | Options | Leaning |
|---|----------|---------|---------|
| 1 | Should `object-detail` be a new code fence language (`language-object-detail`) or embedded as a type within `dashboard` blocks? | A) New language tag. B) Type within existing dashboard handler. | **Decision: A** — new code fence language. Cleaner separation, focused schema per format, simpler LLM selection, and better fit for future user-defined interests where the persisted format metadata maps directly to a code fence language. |
| 2 | Should the `resource-status` interest be split into separate interests for alerts vs other resources? | A) Keep unified. B) Split into `alert-investigation` + `resource-status`. | **Decision: A** — keep unified. Resource-status always includes associated alerts for the resource (this is a hard rule, not optional). Narrow alert investigation is still possible as a separate interaction, but any resource-status view must surface related alerts. Split later only if the interest body grows unwieldy. |
| 3 | How should chart annotations (threshold lines) interact with existing chart components? | A) Add `annotations` prop to AreaChartBlock. B) Overlay component. | **Decision: A** — use recharts' built-in `<ReferenceLine>` / `<ReferenceArea>`. Schema: `"annotations": [{"y": 95, "label": "Warning", "color": "yellow"}]`. Applies to both dashboard `area` panels and object-detail `metrics-strip` charts. Be mindful of screen real-estate — annotations should be minimal (thin lines, small labels) and the LLM should limit to 1-2 per chart to avoid clutter. |
| 4 | Should the timeline section support collapsible groups (e.g., collapse 50 EMS events)? | A) Flat list. B) Collapsible with "show more." | **Decision: B** — show first 10 events, collapse the rest behind "Show N more." Broader design principle: wherever possible, UI elements should be clickable and drive sensible follow-up prompts. "Show more" expands inline, but entity names, alert links, and event details should inject follow-up chat prompts when clicked (e.g., clicking a volume name sends "Tell me about vol_prod_01"). This applies across all object-detail sections, not just timeline. |
| 5 | Should property grid links be styled differently from regular values? | A) Underline like hyperlinks. B) Subtle hover indicator. | **Decision: B+** — entity-reference values rendered in accent color (no underline at rest), with underline + highlight on hover. Uses Mantine `Anchor` with `underline="hover"`. Provides soft discoverability without cluttering the grid. Clicking injects a follow-up chat prompt (per Q4 principle). |
| 6 | Should we support `object-detail` within `dashboard` panels (embedded detail cards)? | A) No, object-detail is top-level only. B) Yes, allow nesting. | **Decision: A** — object-detail is always top-level. Navigation paradigm: dashboard panels (e.g., resource-table rows, alert-list items) and table cells can link *to* an object-detail view by injecting a follow-up prompt, but object-detail is never embedded inside a dashboard grid. This mirrors standard UI drill-down: list/grid → detail page. |
