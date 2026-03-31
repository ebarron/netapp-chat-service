# Reusable Chat Service — TODO

> Tracks open issues and pre-extraction work for the `netapp-chat-service` project.
> See [reusable-chat-interface.md](reusable-chat-interface.md) for the full design.

---

## Open Issues

### 1. Cluster-Level Access Enforcement in the Chatbot Path

**Priority:** High — must be resolved before multi-tenant deployment (NACL)
**Status:** Open

**Problem:**

The chatbot connects to MCP servers (Harvest, ONTAP, Grafana) over the internal Docker/K8s network with no per-user cluster restrictions. When a user asks "show me volumes on cluster bigcluster", the request flows through:

1. `server/server.go` — no cluster check
2. `agent/agent.go` — filters by capability state only, not cluster
3. `mcpclient/router.go` — forwards tool calls with no auth headers or cluster filter
4. MCP server — trusts all internal callers, returns data for any cluster

**Today in host application** this is acceptable because all frontend users are full admins (JWT/Basic Auth). The per-cluster restriction infrastructure exists in `packages/auth/` — `IsValidWithCluster()`, `TokenClustersFromContext()`, and context values are stored by `AuthMiddleware` (with the comment "for agent cluster enforcement") — but the chatbot code path never reads them.

**In the reusable `netapp-chat-service`** this becomes a real gap. The design declares the service "auth-agnostic" and trusts the upstream proxy, but provides no mechanism for the host product to communicate which clusters a user may access, and no mechanism for the agent to enforce those restrictions.

**Why we can't push enforcement to MCP servers:**

- We don't control all MCP servers (Harvest MCP, Grafana MCP, future third-party MCPs)
- MCP is a standard protocol — callers can't inject arbitrary auth semantics
- Even MCP servers we do control (ONTAP MCP) don't currently support per-caller cluster filtering

**Proposed solution — agent-loop pre-call validation:**

The enforcement point is the agent loop, which already intercepts every tool call for capability checks (Off/Ask/Allow). Add a cluster gate at the same interception point:

1. **Host product passes allowed clusters** — via a header (e.g., `X-Allowed-Clusters: clusterA,clusterB`), a JWT claim, or a config-driven mapping. The chat service reads this on each request.

2. **Agent injects cluster constraints into the system prompt** — tells the LLM which clusters the user has access to, so the LLM avoids requesting data for unauthorized clusters (best-effort, not a security boundary).

3. **Agent validates tool call arguments before execution** — inspects the tool call's JSON arguments for cluster-identifying fields (e.g., `cluster`, `poller`, `target`). If the value doesn't match the user's allowed list, the call is blocked and the LLM receives an "access denied" error result, just like a capability=Off block.

4. **Cluster field identification** — each MCP server's tools have different argument names for cluster/target. The chat service config maps MCP server names to their cluster argument fields:
   ```yaml
   cluster_enforcement:
     harvest:
       cluster_fields: ["cluster", "poller"]
     ontap:
       cluster_fields: ["cluster", "target"]
   ```

**What this does NOT require:**
- Changes to MCP servers
- A new agent framework or architecture
- Changes to the MCP protocol

**What this does require:**
- A way for the host product to communicate allowed clusters per request
- A pre-call validation step in the agent loop (alongside capability checks)
- A config mapping of MCP tool argument names to cluster identifiers

**Relevant code:**
- `packages/auth/token_hash.go` — `IsValidWithCluster()`, `TokenClustersFromContext()`, context storage (already built)
- `packages/auth/require_scope.go` — `BearerTokenFromContext()` (already built)
- `agent/agent.go` — `filteredTools()` and tool call interception (enforcement point)
- `mcpclient/router.go` — `CallTool()` (where headers could be added)
- `server/server.go` — request handler (where allowed clusters would be read from context)

---

## Pre-Extraction Refactoring

In-place changes to the host application codebase that reduce coupling and simplify Phase 1 extraction.

### 2. Extract SSE Handler from routes/chat.go

**Status:** Not started

`server/server.go` has 9 host application-specific imports. Separate the SSE streaming logic (which is generic) from the chat-service-specific wiring (alertmgr, prometheus hooks). The SSE handler should accept interfaces, not concrete chat-service types.

### 3. Extract MCP Connection Setup from chatbot.go

**Status:** Not started

`chatbot.go` mixes config parsing, MCP server creation, Grafana SA provisioning, and route wiring. Move the "connect to N MCP servers from a config list" logic into `mcpclient/` so the MCP client package is self-bootstrapping.

### 4. Parameterize Interest Directory Paths

**Status:** Not started

`interest/catalog.go` may have hardcoded paths. Change to constructor parameters so the chat service can mount interests from any directory.

### 5. Clean MetricsFetcher Interface in render/

**Status:** Not started

Ensure `render/volume.go`'s VictoriaMetrics dependency is behind a clean interface that any metrics backend can implement.

---

## Decisions Needed

### ~~6. Public GitHub vs Internal BitBucket~~

**Resolved:** Private GitHub repo, inner-source. Will open-source later when ready. Image registry TBD (GHCR private or docker.repo.eng.netapp.com). Extraction happens AFTER in-place refactoring (items 2–5 above) is complete.

### ~~7. Confirm Recharts as Chart Library~~

**Resolved:** Recharts. Both known consumers (host application, NACL) are React apps, so the React-only constraint is a non-issue. The extracted `@netapp/chat-component` bundles Recharts. Non-React consumers (hypothetical) use the SSE API directly and render their own charts.

---

## Phased Approach (Updated)

1. **Phase 1: Extract backend** — Move Go packages to standalone `netapp-chat-service` repo. host application consumes it as a container.
2. **Phase 2: Extract frontend** — Publish `@netapp/chat-component` npm package. host application switches from local ChatPanel.
3. **Phase 2.5: Harvest bundling** — Harvest ships with the chat UI as an optional built-in for standalone Harvest deployments.
4. **Phase 3: NACL integration** — Deploy chat-service as NACL Helm sub-chart.
5. **Phase 4: Registry model** — Dynamic MCP server discovery via registry.

host application uses **Option A (built-in config UI)** for frontend integration — self-contained widget with bundled settings.
