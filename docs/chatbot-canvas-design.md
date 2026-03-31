# Chatbot Canvas Panel

> **Status:** Design — not yet implemented  
> **Audience:** Engineers working on host application chatbot  
> **Prerequisite reading:** `docs/chatbot-architecture.md`, `docs/chatbot-graphical-ui-enhancements.md`, `docs/chatbot-object-detail-design.md`

---

## 1. Problem Statement

The chatbot today renders all content — dashboards, object details, tables, prose — inline in a single scrolling message stream. When the user drills down from a dashboard into a volume, the original dashboard scrolls out of view. When they ask a follow-up question, the volume detail scrolls too. There's no way to keep reference content visible while the conversation continues.

This creates friction in common workflows:

1. **Morning coffee → drill-down:** User views a fleet health dashboard, clicks a volume row, gets an object-detail. To compare the volume to the original dashboard, they must scroll back and forth.

2. **Multi-object investigation:** User asks about three volumes in sequence. Each detail view scrolls away when the next one arrives. There's no side-by-side comparison.

3. **Reference while exploring:** User opens a cluster detail, then asks follow-up questions about capacity planning. The cluster detail — which they want to keep referring to — has scrolled off-screen.

### 1.1 Current Layout

```
┌──────────────────────────────────────────────────────────┐
│ [AI]  AppShell Header                                    │
├─────────────────────┬─────────────────────┬──────────────┤
│                     │                     │              │
│   Chat Panel        │    Main Content     │              │
│   (Drawer, left)    │    (Grafana,        │   Navbar     │
│                     │     Settings)       │              │
│   ┌─────────────┐   │                     │              │
│   │ Messages     │   │                     │              │
│   │  ↕ scroll    │   │                     │              │
│   │             │   │                     │              │
│   ├─────────────┤   │                     │              │
│   │ Input       │   │                     │              │
│   └─────────────┘   │                     │              │
└─────────────────────┴─────────────────────┴──────────────┘
```

The chat panel is a left-side Mantine `Drawer` (default 480px, resizable 360px–80vw). All content — prose, dashboards, object details — scrolls vertically in the messages area. Once content scrolls past, it's gone from view.

### 1.2 Desired Behavior

Users should be able to **pin** rich content (object details, dashboards) into a persistent workspace alongside the chat. Pinned content stays visible while the conversation continues. Users can open multiple tabs, switch between them, and close them when done.

---

## 2. Design: Canvas Panel

### 2.1 Concept

The **canvas** is a tabbed workspace that appears to the **right** of the chat area within the same drawer. When the canvas has no tabs, it's hidden and the drawer behaves as it does today. When content is opened in the canvas, the drawer expands and splits into two regions:

```
┌──────────────────────────────────────────────────────────┐
│ [AI]  AppShell Header                                    │
├───────────────┬────────────────────┬──────────┬──────────┤
│               │                    │          │          │
│  Chat (40%)   │   Canvas (60%)     │  Main    │  Navbar  │
│               │                    │  Content │          │
│ ┌───────────┐ │ ┌──┬──┬──┐         │          │          │
│ │ Messages  │ │ │T1│T2│T3│ tabs    │          │          │
│ │  ↕ scroll │ │ ├──┴──┴──┤         │          │          │
│ │           │ │ │        │         │          │          │
│ ├───────────┤ │ │ Content │         │          │          │
│ │ Input     │ │ │ (static)│         │          │          │
│ │           │ │ │        │         │          │          │
│ └───────────┘ │ └────────┘         │          │          │
└───────────────┴────────────────────┴──────────┴──────────┘
```

### 2.2 Key Properties

| Property | Decision | Rationale |
|----------|----------|-----------|
| **Trigger** | Chatbot-initiated | Interests declare `output_target: canvas`; the LLM follows this guidance. The user doesn't manually pin — the system knows which content types benefit from persistence. |
| **Content freshness** | Static snapshots | Content is captured when opened. No auto-refresh. Simple, predictable, no ongoing API load. |
| **Tab identity** | One tab per entity | Deduplicated by `kind + name + qualifier`. Opening the same volume twice focuses the existing tab instead of creating a duplicate. |
| **Tab lifecycle** | User-closed | Tabs persist until the user closes them. Closing a tab removes it from the LLM's context permanently. |
| **LLM awareness** | System prompt injection | A structured summary of open canvas tabs is appended to the system prompt so the LLM can reference pinned content and avoid repeating information. |
| **Space allocation** | Chat 40%, Canvas 60% | Canvas gets more space because it holds data visualizations (charts, tables, property grids). Chat is on the left (narrower but still functional for conversation), canvas on the right. |

### 2.3 When the Canvas Is Empty

When all tabs are closed (or none have been opened), the canvas region is **completely hidden**. The drawer reverts to its normal single-column layout at whatever width the user last set. No visual indicator of the canvas exists — it appears seamlessly when the first tab opens and disappears when the last tab closes.

---

## 3. Interest Frontmatter Extension

### 3.1 New Field: `output_target`

Interests gain a new optional YAML frontmatter field that tells the LLM where to render output:

```yaml
output_target: canvas | chat
```

- **`canvas`** — The LLM should emit a new SSE event that opens the content in a canvas tab rather than inline in the chat stream. The LLM also posts a short summary message in the chat (e.g., "I've opened the volume details for vol_prod_01 in the canvas.").
- **`chat`** (default) — Current behavior. Content renders inline in the message stream.

If `output_target` is omitted, it defaults to `chat` — maintaining backward compatibility with all existing interests.

### 3.2 Updated InterestMeta Struct

```go
type InterestMeta struct {
    ID           string   `yaml:"id"`
    Name         string   `yaml:"name"`
    Source       string   `yaml:"source"`
    Triggers     []string `yaml:"triggers"`
    Requires     []string `yaml:"requires"`
    OutputTarget string   `yaml:"output_target"` // "canvas" or "chat" (default: "chat")
}
```

### 3.3 Which Interests Target the Canvas

Based on content type and typical usage patterns:

| Interest | `output_target` | Rationale |
|----------|-----------------|-----------|
| `morning-coffee` | `chat` | Summary dashboard — fast to scan, user expects inline |
| `morning-coffee-v2` | `chat` | Same — inline dashboard |
| `object-list` | `chat` | Table listing — inline is natural (but drill-downs from it open in canvas — see §3.4) |
| `resource-status` | `canvas` | Deep-dive on a single entity — user will want to reference this while asking follow-ups |
| `volume-detail` | `canvas` | Single-volume detail — same rationale |
| `volume-provision` | `canvas` | Provision workflow produces a placement dashboard the user will reference while confirming — canvas keeps it visible alongside the chat conversation |

User-created interests can set `output_target: canvas` in their frontmatter to opt in.

### 3.4 Canvas Trigger Scenarios

The canvas is opened in two distinct scenarios:

#### Scenario 1: Object-detail drill-down from a list

When a user clicks a row in a `resource-table` panel (within any interest), the click injects a follow-up chat message (e.g., "Tell me about volume vol_prod_01 on SVM vdbench on cluster cls1"). This message matches an interest (`volume-detail`, `resource-status`) that has `output_target: canvas`. The LLM follows the interest's instructions and emits the object-detail in a canvas tab.

This applies to:

- **Morning coffee** (`morning-coffee`, `morning-coffee-v2`) — clicking a volume row in the fleet summary's resource-table triggers `volume-detail` (canvas)
- **Object list** (`object-list`) — clicking any object row in a list triggers the appropriate detail interest (`volume-detail`, `resource-status`) which opens in canvas
- **Any dashboard** with a `resource-table` panel — row clicks follow the same pattern

The key insight: the *listing* interest stays `output_target: chat` (it renders inline), but the *detail* interest it drills into is `output_target: canvas`. The canvas trigger is on the destination interest, not the source.

#### Scenario 2: Volume provision

The `volume-provision` interest produces a placement dashboard (candidate clusters, capacity comparison, recommendation, proposed command). This opens in a canvas tab so the user can reference the placement analysis while interacting with the chat to confirm, adjust parameters, or approve the operation.

#### LLM Discretion

The LLM may also open canvas tabs for responses that don't match a specific interest. When the LLM produces an `object-detail` or `dashboard` response for a user question and judges that the user would benefit from persistent reference (e.g., a detailed cluster analysis in response to an ad-hoc question), it may use the `canvas-object-detail` or `canvas-dashboard` fence at its discretion. The system prompt instructs the LLM to prefer canvas for single-entity detail views and reference-heavy content.

### 3.5 Interest Index Update

The `BuildIndex()` method includes `output_target` in the interest table so the LLM can see it:

```
| ID | Name | Triggers | Target |
|----|------|----------|--------|
| morning-coffee | Fleet Health Overview | how's everything, ... | chat |
| volume-detail | Volume Detail with Monitoring | tell me about volume, ... | canvas |
```

The system prompt instructions are updated to explain:

> When an interest has `Target: canvas`, emit the final output block using the `canvas_open` SSE event instead of inline markdown. Also emit a short chat message confirming what was opened. If the interest has `Target: chat`, render output inline as you do today.

---

## 4. SSE Protocol Extension

### 4.1 New Event: `canvas_open`

A new SSE event type tells the frontend to open content in a canvas tab:

```
event: canvas_open
data: {
  "tab_id": "volume::vol_prod_01::on SVM vdbench on cluster cls1",
  "title": "vol_prod_01",
  "kind": "volume",
  "qualifier": "on SVM vdbench on cluster cls1",
  "content": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `tab_id` | string | Unique identifier for deduplication. Constructed as `kind::name::qualifier`. |
| `title` | string | Human-readable tab label (entity name). |
| `kind` | string | Entity type (`volume`, `cluster`, `svm`, `aggregate`, `alert`). |
| `qualifier` | string | Identity context for the entity. |
| `content` | object | The full JSON payload — an `object-detail` or `dashboard` block, same schema as inline rendering. |

### 4.2 Tab ID Construction

The `tab_id` is a stable identifier for deduplication:

```
{kind}::{name}::{qualifier}
```

Examples:
- `volume::vol_prod_01::on SVM vdbench on cluster cls1`
- `cluster::cls-east::`
- `alert::InstanceDown::on cluster cls-east`

When the frontend receives a `canvas_open` event with a `tab_id` that matches an existing tab, it **replaces** the content and **focuses** the existing tab rather than opening a new one.

### 4.3 Backend Emission

The agent loop needs a mechanism for the LLM to signal "open in canvas." Two approaches:

**Approach A: LLM emits a sentinel code fence**

The LLM outputs a code fence with language `canvas-object-detail` or `canvas-dashboard` instead of the regular `object-detail` or `dashboard`. The backend's SSE writer intercepts this in the text stream, parses the JSON, and emits a `canvas_open` event instead of a `message` text event.

```markdown
```canvas-object-detail
{ "type": "object-detail", "kind": "volume", "name": "vol_prod_01", ... }
```​
```

**Approach B: Dedicated tool call**

A synthetic tool `open_in_canvas(content)` that the LLM calls. The backend handles it by emitting `canvas_open`.

**Decision: Approach A (sentinel fence).** This aligns with the existing pattern where the LLM emits typed code fences and the system dispatches on the language tag. No new tool definition is needed, and the interest body instructions simply say "emit your output in a `canvas-object-detail` fence" instead of "emit in an `object-detail` fence."

### 4.4 Frontend Handling

The SSE reader in `useChatPanel.ts` already dispatches on event type. A new case is added:

```typescript
case 'canvas_open':
  const tab = JSON.parse(data) as CanvasTab;
  addOrFocusCanvasTab(tab);
  break;
```

### 4.5 Sentinel Fence Interception (Backend)

During SSE text streaming, the backend already processes the LLM's output token-by-token. When a complete code fence with language `canvas-object-detail` or `canvas-dashboard` is detected:

1. Parse the JSON content
2. Extract `kind`, `name`, `qualifier` from the payload to construct `tab_id` and `title`
3. Emit a `canvas_open` SSE event with the full content
4. Suppress the text tokens from the regular `message` stream (don't render the fence inline)

This keeps the chat stream clean — the user sees only the confirmation message, not the raw JSON.

---

## 5. Frontend Architecture

### 5.1 New Components

```
ChatPanel (existing Drawer, position: left)
├── Chat area (existing — left region, 40%)
│   ├── Messages (ScrollArea)
│   └── Input area
├── Divider (optional visual separator)
└── CanvasPanel (new — right region, 60%)
    ├── Tab bar (Mantine Tabs)
    │   └── Tab × N (title + close button)
    └── Tab content area
        └── ObjectDetailBlock | DashboardBlock (reused existing components)
```

#### `CanvasPanel.tsx`

New component responsible for the tabbed workspace:

```typescript
interface CanvasTab {
  tabId: string;       // Deduplication key
  title: string;       // Tab label
  kind: string;        // Entity type
  qualifier: string;   // Identity context
  content: object;     // Dashboard or object-detail JSON
}

interface CanvasPanelProps {
  tabs: CanvasTab[];
  activeTab: string | null;
  onTabChange: (tabId: string) => void;
  onTabClose: (tabId: string) => void;
}
```

Renders a Mantine `Tabs` component. Each tab's content panel uses the existing `ObjectDetailBlock` or `DashboardBlock` (dispatched by `content.type`), wrapped in a `ScrollArea` for independent scrolling.

#### State additions to `useChatPanel.ts`

```typescript
// Canvas tab state
const [canvasTabs, setCanvasTabs] = useState<CanvasTab[]>([]);
const [activeCanvasTab, setActiveCanvasTab] = useState<string | null>(null);

// Add or focus a canvas tab (deduplication by tabId)
const addOrFocusCanvasTab = useCallback((tab: CanvasTab) => {
  setCanvasTabs(prev => {
    const existing = prev.findIndex(t => t.tabId === tab.tabId);
    if (existing >= 0) {
      // Replace content in existing tab, focus it
      const updated = [...prev];
      updated[existing] = tab;
      return updated;
    }
    return [...prev, tab];
  });
  setActiveCanvasTab(tab.tabId);
}, []);

// Close a canvas tab
const closeCanvasTab = useCallback((tabId: string) => {
  setCanvasTabs(prev => {
    const filtered = prev.filter(t => t.tabId !== tabId);
    // If we closed the active tab, focus the last remaining tab
    if (activeCanvasTab === tabId) {
      setActiveCanvasTab(filtered.length > 0 ? filtered[filtered.length - 1].tabId : null);
    }
    return filtered;
  });
}, [activeCanvasTab]);
```

### 5.2 Drawer Layout Changes

The `ChatPanel` drawer adapts its width and internal layout based on whether the canvas has tabs:

```
canvasTabs.length === 0:
  → Drawer width = user's stored width (current behavior)
  → Single-column layout: chat only

canvasTabs.length > 0:
  → Drawer width = max(current width × 2.5, 80vw)
  → Two-column flex layout: chat (40%) | canvas (60%)
  → Transition animates the expansion
```

#### Width Calculation

```typescript
const hasCanvas = canvasTabs.length > 0;
const effectiveWidth = hasCanvas
  ? Math.max(drawerWidth * 2.5, window.innerWidth * 0.8)
  : drawerWidth;
```

The expansion uses a CSS transition on `width` for a smooth animation when the first tab opens or the last tab closes.

#### Internal Flex Layout

```css
.drawerBody {
  display: flex;
  flex-direction: row;
  height: 100%;
}

.panel {
  flex: 0 0 40%;
  min-width: 320px;
}

.canvasRegion {
  flex: 0 0 60%;
  border-left: 1px solid var(--mantine-color-gray-3);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
```

### 5.3 Tab Close Behavior

Each tab has a close button (×) in the tab header. Closing a tab:

1. Removes it from `canvasTabs` state
2. If it was the active tab, the last remaining tab becomes active
3. If it was the *only* tab, the canvas region is hidden and the drawer width reverts
4. The LLM is notified on the next message (see §6 — the tab summary in the system prompt no longer includes it)

### 5.4 Resize Handle

The existing right-edge resize handle continues to work. When the canvas is open, dragging it resizes the overall drawer — both regions scale proportionally (40/60 ratio maintained). The stored width in `localStorage` always reflects the chat-only width; the expanded width is computed dynamically.

---

## 6. LLM Context Awareness

### 6.1 System Prompt Injection

The system prompt includes a section describing what's currently visible in the canvas. This is built from the frontend's canvas state, passed to the backend with each message request.

#### Request Body Extension

```typescript
// POST /chat/message
{
  message: string;
  mode: string;
  session_id?: string;
  canvas_tabs?: CanvasTabSummary[];  // NEW
}
```

```typescript
interface CanvasTabSummary {
  tab_id: string;
  kind: string;
  name: string;
  qualifier: string;
  status?: string;        // From object-detail status field
  key_properties?: Record<string, string>;  // Subset of properties section
}
```

The frontend constructs `CanvasTabSummary` from each open tab's content, extracting only the identity and key properties — not the full JSON payload.

#### System Prompt Section

When `canvas_tabs` is non-empty, the system prompt includes:

```
## Canvas Context

The user has the following items pinned in the canvas (visible alongside this chat):

| Tab | Kind | Name | Status | Context |
|-----|------|------|--------|---------|
| 1 | volume | vol_prod_01 | warning | on SVM vdbench on cluster cls1 |
| 2 | cluster | cls-east | ok | |

The user can see these items without scrolling. You can refer to them
("the volume in your canvas", "as shown in the cluster detail") without
repeating their full content. When the user asks follow-up questions,
consider whether they're referring to a canvas item.

When the user closes a canvas tab, it will no longer appear here.
Do not reference closed tabs.
```

### 6.2 Backend Handling

The `BuildSystemPrompt` function gains a new parameter:

```go
func BuildSystemPrompt(
    router mcpclient.ToolRouter,
    interestIndex string,
    canvasTabs []CanvasTabSummary,  // NEW
) string
```

If `canvasTabs` is non-empty, the canvas context section is appended to the prompt after the interest section. The `CanvasTabSummary` struct mirrors the frontend's:

```go
type CanvasTabSummary struct {
    TabID         string            `json:"tab_id"`
    Kind          string            `json:"kind"`
    Name          string            `json:"name"`
    Qualifier     string            `json:"qualifier"`
    Status        string            `json:"status,omitempty"`
    KeyProperties map[string]string `json:"key_properties,omitempty"`
}
```

### 6.3 Context Size Budget

Each canvas tab summary adds approximately 1–2 lines to the system prompt. With a practical maximum of ~10 open tabs, this adds at most ~20 lines — negligible relative to the system prompt's existing size and the LLM's context window.

The full JSON content of canvas tabs is **not** included in the system prompt. The LLM has enough context from the summary to refer to items and answer follow-up questions. If the user asks a question that requires data not in the summary (e.g., "what was the IOPS trend on that volume?"), the LLM can re-query the metrics tools.

---

## 7. Interaction Flows

### 7.1 Morning Coffee → Volume Drill-Down (Scenario 1)

```
User: "Good morning, how's everything?"

LLM: (matches morning-coffee interest, output_target: chat)
     Emits dashboard inline in chat with fleet summary,
     resource-table with volume rows.

User: [clicks volume "vol_prod_01" row in resource-table]

Frontend: Injects chat message "Tell me about volume vol_prod_01
          on SVM vdbench on cluster cls1"

LLM: (matches volume-detail interest, output_target: canvas)
     1. Calls tools to gather volume data
     2. Emits ```canvas-object-detail fence with volume detail JSON
     3. Emits chat message: "I've opened the details for vol_prod_01
        in the canvas."

Backend: Intercepts canvas-object-detail fence → emits canvas_open SSE event

Frontend:
     1. Drawer expands to show canvas region
     2. "vol_prod_01" tab opens with object-detail content
     3. Chat shows: "I've opened the details for vol_prod_01 in the canvas."
     4. Original morning-coffee dashboard is still visible in chat scroll history

User: "What's the capacity projection for the next 30 days?"

LLM: (system prompt includes canvas context: vol_prod_01 is pinned)
     Understands the question refers to the pinned volume.
     Answers inline in chat with projection analysis.
     User can glance at canvas to see current capacity alongside the projection.
```

### 7.2 Multi-Volume Comparison

```
User: "Tell me about vol_db_01 on SVM prod on cluster east"

LLM: Opens vol_db_01 in canvas tab 1.

User: "Now show me vol_db_02 on the same SVM"

LLM: Opens vol_db_02 in canvas tab 2. Both tabs visible.

User: "Which one has more headroom?"

LLM: (system prompt shows both volumes in canvas)
     Answers by comparing the two, referencing "the volumes in your canvas."
```

### 7.3 Tab Close → Context Forgotten

```
User: [closes vol_db_01 tab]

Frontend: Removes tab from canvasTabs state.

User: "What was the IOPS on vol_db_01?"

LLM: (canvas context no longer includes vol_db_01)
     Treats this as a new question. Calls tools to fetch data fresh.
     Opens a new canvas tab (or answers inline, depending on interest match).
```

### 7.4 Same Entity Reopened → Tab Reused

```
User: "Tell me about vol_prod_01 on SVM vdbench on cluster cls1"

LLM: Emits canvas_open with tab_id "volume::vol_prod_01::on SVM vdbench on cluster cls1"

Frontend: Tab already exists → replaces content with fresh data, focuses tab.
          No duplicate tab created.
```

---

## 8. Implementation Plan

Each phase includes its own tests. **A phase is not complete until all its tests are written and passing.** Tests are the exit criteria — no phase closes without green tests covering every item listed below.

### Phase 1: Interest Schema & Backend Foundation

1. **Interest schema** — Add `output_target` field to `InterestMeta` struct and parser. Default to `"chat"` when omitted.
2. **Interest index** — Include `output_target` in `BuildIndex()` table as a `Target` column.
3. **SSE event type** — Define `canvas_open` event type in the backend SSE writer.
4. **Sentinel fence interception** — Backend detects `canvas-object-detail` / `canvas-dashboard` fences in the LLM text stream, parses the JSON, and emits `canvas_open` SSE events. Suppresses the raw fence from the `message` stream.

**Phase 1 exit gate — all must pass:**
- [x] Go unit test: `InterestMeta` parsing with `output_target: canvas`, `output_target: chat`, and omitted (defaults to `"chat"`)
- [x] Go unit test: `BuildIndex()` output includes `Target` column with correct values
- [x] Go unit test: sentinel fence detection — given a text stream containing `` ```canvas-object-detail ``, verify `canvas_open` event is emitted and text is suppressed
- [x] Go unit test: sentinel fence with malformed JSON — verify graceful fallback (emit as regular text)

### Phase 2: Frontend Canvas Components

5. **`CanvasPanel` component** — Mantine `Tabs` with close buttons, content dispatch to `ObjectDetailBlock` / `DashboardBlock`.
6. **Canvas state in `useChatPanel`** — `canvasTabs`, `activeCanvasTab`, `addOrFocusCanvasTab`, `closeCanvasTab`. Max 5 tabs — when exceeded, oldest tab auto-closes.
7. **SSE handler** — `canvas_open` event case in the SSE reader calls `addOrFocusCanvasTab`.
8. **Drawer layout** — Conditional two-column flex layout. Canvas hidden when no tabs. Drawer expands to `max(width × 2.5, 80vw)` when canvas has tabs.
9. **CSS transitions** — Smooth width animation on expand/collapse.
10. **Narrow viewport fallback** — On viewports < 1024px, canvas is disabled. `canvas_open` events fall back to inline rendering in chat.

**Phase 2 exit gate — all must pass:**
- [x] Vitest component test: `CanvasPanel` renders tabs, displays correct content for each tab
- [x] Vitest component test: closing a tab removes it; closing active tab focuses the previous tab; closing last tab hides canvas
- [x] Vitest component test: tab deduplication — opening same `tabId` replaces content and focuses existing tab
- [x] Vitest component test: max 5 tabs — opening a 6th auto-closes the oldest
- [x] Vitest component test: drawer width expands when canvas has tabs, reverts when empty
- [x] Vitest unit test: `addOrFocusCanvasTab` and `closeCanvasTab` state logic
- [x] Vitest component test: narrow viewport (< 1024px) — `canvas_open` renders content inline in chat instead of canvas

### Phase 3: LLM Context Awareness

11. **Request body extension** — Add `canvas_tabs` field to `POST /chat/message`.
12. **System prompt canvas section** — `BuildSystemPrompt` appends canvas context table when tabs are present.
13. **Tab summary extraction** — Frontend extracts `CanvasTabSummary` (kind, name, qualifier, status, key properties) from each open tab's content.
14. **State persistence** — Elevate canvas tab state to React context so it survives page navigation within the app.

**Phase 3 exit gate — all must pass:**
- [x] Go unit test: `BuildSystemPrompt` with non-empty `canvasTabs` — verify canvas context section is appended with correct table
- [x] Go unit test: `BuildSystemPrompt` with empty `canvasTabs` — verify no canvas section
- [x] Go unit test: chat message handler deserializes `canvas_tabs` from request body
- [x] Vitest unit test: `CanvasTabSummary` extraction — given an object-detail content blob, verify correct kind/name/qualifier/status/key_properties
- [x] Vitest component test: canvas state persists after navigating to Settings and back

### Phase 4: Interest Updates & System Prompt

15. **Update built-in interests** — Set `output_target: canvas` on `resource-status`, `volume-detail`, and `volume-provision`.
16. **System prompt instructions** — Add guidance explaining `canvas-*` fences, LLM discretion for ad-hoc canvas use, and how to emit a chat confirmation message alongside canvas content.
17. **User interest support** — Validate `output_target` field in `save_interest` tool (must be `"canvas"` or `"chat"`).

**Phase 4 exit gate — all must pass:**
- [x] Go unit test: each updated interest file parses with correct `output_target` value
- [x] Go unit test: `save_interest` rejects invalid `output_target` values
- [x] Go unit test: system prompt includes canvas fence instructions when interests are loaded
- [x] Playwright E2E test: trigger `volume-detail` interest → verify `canvas_open` SSE event is received (mock LLM emitting `canvas-object-detail` fence)

### Phase 5: Integration & E2E

18. **Full flow E2E** — Morning coffee dashboard renders inline → click volume row → volume-detail opens in canvas tab → chat continues alongside.
19. **Multi-tab E2E** — Open two object-details → both tabs visible → close one → verify canvas context updates.
20. **Provision flow E2E** — Trigger `volume-provision` → opens in canvas → user interacts in chat → canvas shows placement analysis throughout.
21. **Deduplication E2E** — Open same entity twice → verify tab is reused, not duplicated.
22. **Canvas collapse E2E** — Open tab → close it → verify drawer reverts to single-column width.

**Phase 5 exit gate — all must pass:**
- [x] Playwright E2E: morning coffee → volume drill-down → canvas tab appears with object-detail
- [x] Playwright E2E: object-list → click row → canvas tab appears
- [x] Playwright E2E: volume-provision → canvas tab with placement dashboard
- [x] Playwright E2E: open 6 tabs → verify oldest auto-closed, 5 remain
- [x] Playwright E2E: close all tabs → drawer width reverts to stored single-column width
- [x] Playwright E2E: same entity opened twice → single tab with updated content

---

## 9. Design Decisions

The following questions were resolved during design review:

| # | Question | Decision | Detail |
|---|----------|----------|--------|
| 1 | **Maximum tab count** | 5 tabs, oldest auto-closes | When a 6th tab is opened, the oldest tab is automatically closed. No user warning — the eviction is silent. This keeps the canvas focused and the LLM context compact. |
| 2 | **Canvas in narrow viewports** | Disabled below 1024px | On viewports narrower than 1024px, the canvas is disabled entirely. `canvas_open` SSE events fall back to inline rendering in the chat stream. The LLM's canvas fence is rendered as a regular `object-detail` or `dashboard` block. No degraded canvas layout. |
| 3 | **Tab reordering** | No reordering | Tabs appear in insertion order (left to right). No drag-to-reorder. Keeps implementation simple; tab position is not meaningful enough to warrant the complexity. |
| 4 | **Canvas without interests** | LLM discretion | The LLM may open canvas tabs for responses that don't match a specific interest. When the LLM produces a single-entity detail view or reference-heavy dashboard in response to an ad-hoc question, it can use `canvas-*` fences at its judgment. The system prompt provides guidance (prefer canvas for detail views, use chat for quick answers). |
| 5 | **Persistence across navigation** | Persist | Canvas tab state is elevated to a React context provider so it survives page navigation within the app (e.g., navigating to Settings and back). State is not persisted to `localStorage` — a full page refresh clears canvas tabs (same as chat messages today). |
