# Volume Monitoring Design Spec

**Status**: Draft  
**Date**: 2026-03-17  
**Depends on**: Harvest MCP alert CRUD (HARVEST_RULES_PATH), ONTAP MCP
(`ontap_get` for QoS and snapshot policy lookups), checkbox UI field type

## Overview

Add per-volume alert monitoring to host application. When provisioning a new volume, the
user can opt in to monitoring via a checkbox. When enabled, an alert management
agent inside chat-service orchestrates the creation of vmalert rules scoped to that
volume by calling Harvest MCP's alert CRUD tools. The volume-detail view gains
"Stop Monitoring" / "Monitor this Volume" buttons to toggle monitoring on
existing volumes.

## Goals

1. One-click monitoring opt-in during volume provisioning
2. Deterministic alert rule generation (no LLM-authored YAML)
3. Clean enable/disable lifecycle from the volume-detail view
4. Leverage Harvest MCP's native alert CRUD tools for rule management
5. Structured for future extraction into a standalone agentic service (A2A)

## Architecture

### System Diagram

```
                            ┌─ host application Appliance ──────────────────────────────────────────────┐
                            │                                                                 │
 ┌──────────┐  HTTPS/JWT    │ ┌──────────────────── chat-service (single Go process) ─────────────┐ │
 │ Browser  │◄─────────────►│ │                                                             │ │
 │ Admin UI │ BasicAuth+JWT │ │  Agent Loop                                                 │ │
 └──────────┘ (Caddy TLS)  │ │    │                                                         │ │
                            │ │    ├─── LLM Provider ──────────► OpenAI / Anthropic (ext)    │ │
                            │ │    │     (HTTPS + API key)                                   │ │
                            │ │    │                                                         │ │
                            │ │    ├─── Internal Tools (in-process, no network)              │ │
                            │ │    │     ├── get_interest                                    │ │
                            │ │    │     ├── save_interest                                   │ │
                            │ │    │     ├── enable_volume_monitoring  ◄── NEW (alertmgr)    │ │
                            │ │    │     ├── disable_volume_monitoring ◄── NEW (alertmgr)    │ │
                            │ │    │     └── get_volume_monitoring_status ◄── NEW (alertmgr) │ │
                            │ │    │           │                                             │ │
                            │ │    │           │ alertmgr calls Harvest MCP alert CRUD       │ │
                            │ │    │           │ tools via MCP Router (same transport)       │ │
                            │ │    │           ▼                                             │ │
                            │ │    └─── MCP Router (HTTP, Docker-internal network)           │ │
                            │ │          ├──► harvest-mcp :8082 ◄── alert CRUD + metrics     │ │
                            │ │          ├──► ontap-mcp   :8084 ◄── policy lookups           │ │
                            │ │          └──► grafana-mcp :8085                              │ │
                            │ └─────────────────────────────────────────────────────────────┘ │
                            │                                                                 │
                            │ ┌─── Harvest MCP writes rules (filesystem) ──────────────────┐ │
                            │ │                                                             │ │
                            │ │  ${host application_HARVEST_PATH}/container/prometheus/                │ │
                            │ │  ├── alert_rules.yml     (Harvest default + custom rules)   │ │
                            │ │  └── ems_alert_rules.yml (EMS default + custom rules)       │ │
                            │ │       │                                                     │ │
                            │ │       │ bind-mounted into vmalert                           │ │
                            │ │       ▼                                                     │ │
                            │ │  vmalert :8880                                              │ │
                            │ │  ├── /alerts_harvest/*_rules.yml  (Harvest-managed)         │ │
                            │ │  └── /alerts_host application/*_rules.yml    (future: standalone)      │ │
                            │ │       │                                                     │ │
                            │ │       │ evaluates rules against                             │ │
                            │ │       ▼                                                     │ │
                            │ │  VictoriaMetrics :8428  ──────►  Alertmanager :9093          │ │
                            │ │  (datasource)                    (notifications)             │ │
                            │ └─────────────────────────────────────────────────────────────┘ │
                            └─────────────────────────────────────────────────────────────────┘
```

### Security Boundaries

| Boundary | Mechanism | Trust Model |
|---|---|---|
| **User → chat-service** | Caddy TLS termination + BasicAuth + JWT cookies (5min access, 20min refresh) | Authenticated, encrypted. All authenticated users have equal tool access (no per-user ACL). |
| **Agent → alertmgr** | In-process Go function call | Same process — no network boundary. Gated by capability system and read-write mode. |
| **alertmgr → Harvest MCP** | MCP Router HTTP call on Docker bridge network | **No transport auth.** Relies entirely on Docker network isolation. Same trust model as all existing MCP calls. |
| **alertmgr → ONTAP MCP** | MCP Router HTTP call on Docker bridge network | Same as above. ONTAP MCP authenticates to ONTAP clusters using stored credentials. |
| **Harvest MCP → filesystem** | Direct file I/O (`alert_rules.yml`, `ems_alert_rules.yml`) | Harvest MCP owns these files. Atomic writes with `.old` backup. |
| **Harvest MCP → vmalert** | HTTP `POST /-/reload` on Docker bridge network | **No auth.** Consistent with all inter-container traffic in the stack. |
| **Capability gating** | Off / Ask / Allow per MCP server | alertmgr tools should require `harvest` capability (since they delegate to harvest-mcp). Write operations require **read-write mode** to be active. |

### Security Considerations

#### Input sanitization

The alertmgr tools accept volume, SVM, and cluster names that become part of
alert rule names and label matchers in PromQL expressions. These must be
sanitized:

- **Rule names**: Alphanumeric, underscore, hyphen only. Reject or strip any
  other characters. No formal length limit in Prometheus/vmalert (VictoriaMetrics
  defaults to 16384 bytes for label values), but keep names reasonable for
  readability.
- **PromQL label values**: Wrapped in double quotes in expressions. Values must
  be escaped to prevent PromQL injection (escape `\`, `"`, and newlines).
- **No path construction**: Since alertmgr delegates to Harvest MCP's
  `create_alert_rule` tool (which manages its own files), there is no path
  traversal risk from alertmgr itself. Harvest MCP handles filename safety.

#### Threshold validation

Threshold values resolved from ONTAP MCP (IOPS, latency, intervals) must be
validated as positive numeric values before being embedded in rule expressions.
Reject non-numeric or negative values.

#### Capability assignment

The three alertmgr tools are **internal tools** (in-process), but they
delegate to `harvest-mcp` via the MCP Router. They should:

- Require the `harvest` capability to be enabled (not Off)
- Require **read-write mode** for `enable_volume_monitoring` and
  `disable_volume_monitoring` (they create/delete alert rules)
- `get_volume_monitoring_status` is read-only and works in any mode

This matches the existing pattern where `save_interest` and `delete_interest`
require read-write mode.

### Alert Management Agent (`chat-service/internal/alertmgr`)

A new `chat-service/internal/alertmgr` package provides three operations exposed as
internal MCP tools to the agent:

| Tool | Parameters | Description |
|------|-----------|-------------|
| `enable_volume_monitoring` | volume, svm, cluster, size_gb, qos_policy?, snapshot_policy? | Create vmalert rules for this volume via Harvest MCP |
| `disable_volume_monitoring` | volume, svm, cluster | Remove all monitoring rules for this volume via Harvest MCP |
| `get_volume_monitoring_status` | volume, svm, cluster | Check if monitoring rules exist (queries Harvest MCP `list_alert_rules`) |

#### Why a Go agent, not direct LLM tool calls?

Harvest MCP already has alert CRUD tools (`create_alert_rule`,
`update_alert_rule`, `delete_alert_rule`, `list_alert_rules`,
`validate_alert_syntax`, `reload_prometheus_rules`). The LLM could call them
directly. However, a Go agent adds value:

1. **Determinism**: Enabling monitoring for one volume creates up to 10 alert
   rules. The Go agent generates all rules from templates with exact
   expressions and thresholds — no risk of LLM hallucinating PromQL syntax.
2. **Atomicity**: The agent creates all rules as a batch operation, calling
   `create_alert_rule` for each and `reload_prometheus_rules` once at the end.
   If any rule fails, it rolls back previously created rules.
3. **Threshold resolution**: The agent calls ONTAP MCP to look up QoS and
   snapshot policy attributes, extracts numeric thresholds, validates them,
   and computes derived values (e.g., `PEAK_IOPS_PER_TB × volume_TB`).
4. **Convention enforcement**: Rule naming follows a strict pattern
   (`host application_vol_<cluster>_<svm>_<volume>_<alert>`) that enables reliable
   listing and cleanup.
5. **Future extraction**: The agent is a self-contained package that can
   become a standalone A2A service with its own MCP interface.

#### Harvest MCP tools used by alertmgr

| Harvest MCP Tool | alertmgr Usage |
|---|---|
| `create_alert_rule` | Create each volume monitoring rule (rule_name, expression, duration, severity, summary, group_name) |
| `delete_alert_rule` | Remove rules by name during disable or rollback |
| `list_alert_rules` | Check if monitoring rules exist for a volume (filter by naming convention) |
| `validate_alert_syntax` | Pre-validate generated PromQL expressions before creating rules |
| `reload_prometheus_rules` | Trigger vmalert reload after batch create/delete |

#### ONTAP MCP tools used by alertmgr

| ONTAP MCP Tool | alertmgr Usage |
|---|---|
| `ontap_get` (path: `/storage/qos/policies`) | Look up QoS policy attributes (max_throughput_iops, expected_latency) for performance alert thresholds |
| `ontap_get` (path: `/storage/snapshot-policies`) | Look up snapshot schedule intervals for data protection alert thresholds |

#### Rule naming convention

All rules created by alertmgr follow the pattern:

```
host application_vol_{cluster}_{svm}_{volume}_{alert_suffix}
```

Examples:
- `host application_vol_nvme_svm1_vol_prod_capacity_warning`
- `host application_vol_nvme_svm1_vol_prod_snapshot_creation_failed`
- `host application_vol_nvme_svm1_vol_prod_iops_peak_breach`

This convention allows `list_alert_rules` + prefix filtering to determine
monitoring status and enumerate all rules for a given volume.

#### Rule group name

All rules for a volume go into a single rule group:

```
host application.volume.{cluster}.{svm}.{volume}
```

This keeps volume-scoped rules organized and separate from Harvest's default
rule groups (`harvest.rules`, `ems.rules`).

### Enabling Harvest MCP Alert CRUD

The alert CRUD tools in Harvest MCP are gated by the `HARVEST_RULES_PATH`
environment variable. host application must configure this to enable the tools.

**Required changes to `build/platforms/common/root/etc/host application/compose.yaml`:**

```yaml
harvest-mcp:
  environment:
    - HARVEST_RULES_PATH=/rules
  volumes:
    - ${host application_HARVEST_PATH}/container/prometheus/:/rules
```

This points Harvest MCP at the same directory that vmalert reads via the
`/alerts_harvest` mount. When `HARVEST_RULES_PATH` is set, Harvest MCP
registers the alert CRUD tools during startup, and they appear in the MCP
`tools/list` response. The chat-service MCP Router discovers them automatically.

**Dev environment** (`local/` directory): Set `HARVEST_RULES_PATH` in
`.env.custom` or the local compose override, pointing to
`local/data/packages/harvest/container/prometheus/`.

### Future extraction to standalone agent

The `alertmgr` package is designed with minimal dependencies:

- **Inputs**: MCP Router (to call Harvest MCP and ONTAP MCP tools)
- **Config**: Rule naming prefix, rule group pattern
- **No filesystem access**: All file operations delegated to Harvest MCP
- **No direct HTTP calls**: vmalert reload delegated to Harvest MCP's
  `reload_prometheus_rules`

This means extracting alertmgr into a standalone service requires only:
1. Give it its own MCP client connections to Harvest MCP and ONTAP MCP
2. Expose its three tools via its own MCP server or A2A interface
3. Register the new service as an MCP server in chat-service's router

## Alert Catalog

### Capacity Alerts (always enabled)

| Alert Name | Severity | Expression | Duration | Summary |
|-----------|----------|------------|----------|---------|
| Volume Capacity Warning | warning | `volume_size_used_percent{volume="V", svm="S"} > 85` | 5m | Volume capacity at 85% |
| Volume Capacity Critical | critical | `volume_size_used_percent{volume="V", svm="S"} > 95` | 5m | Volume capacity at 95% |
| Inodes Exhausted | warning | `volume_inode_used_percent{volume="V", svm="S"} > 90` | 5m | Volume running out of inodes |

**Corrective actions:**
- Capacity Warning: Enable autogrow, increase volume size, or archive/delete data
- Capacity Critical: Immediate action required — expand volume or free space
- Inodes: Increase volume size or max-files limit

### Data Protection Alerts (when snapshot_policy is set)

| Alert Name | Severity | Expression | Duration | Summary |
|-----------|----------|------------|----------|---------|
| Snapshot Creation Failed | critical | `time() - max_over_time(snapshot_create_time{volume="V", svm="S"}[1h]) > MIN_INTERVAL × 2` | 0m | Scheduled snapshot not created |
| Snapshot Reserve Full | warning | `volume_snapshot_reserve_used_percent{volume="V", svm="S"} > 90` | 5m | Snapshot reserve nearly exhausted |
| Snapshot Reserve Overflow | critical | `volume_snapshot_reserve_used_percent{volume="V", svm="S"} > 100` | 0m | Snapshots consuming data space |

**Threshold derivation:**
- `MIN_INTERVAL` = most frequent schedule in the snapshot policy (e.g., hourly → 3600s,
  so alert fires at 7200s). Looked up via ONTAP MCP at enable time.

**Corrective actions:**
- Creation Failed: Check volume state, available space, and snapshot policy configuration
- Reserve Full: Increase snapshot reserve or delete old snapshots
- Reserve Overflow: Immediately increase reserve, delete snapshots, or expand volume

### Performance Alerts (when qos_policy is set)

| Alert Name | Severity | Expression | Duration | Summary |
|-----------|----------|------------|----------|---------|
| IOPS Peak Breach | warning | `volume_total_ops{volume="V", svm="S"} > PEAK_IOPS_PER_TB × volume_TB` | 5m | Volume IOPS exceeding peak threshold |
| IOPS Sustained High | critical | `volume_total_ops{volume="V", svm="S"} > PEAK_IOPS_PER_TB × volume_TB` | 15m | Volume IOPS sustained above peak |
| Latency Breach | warning | `volume_avg_latency{volume="V", svm="S"} > EXPECTED_LATENCY` | 5m | Volume latency exceeds SLA |
| Latency Critical | critical | `volume_avg_latency{volume="V", svm="S"} > EXPECTED_LATENCY × 2` | 5m | Volume latency critically high |

**Threshold derivation:**
- `PEAK_IOPS_PER_TB` and `EXPECTED_LATENCY` extracted from the QoS policy attributes
  via ONTAP MCP at enable time.
- `volume_TB` = volume size in TB (from the `size_gb` parameter ÷ 1024)

**Corrective actions:**
- IOPS Peak: Consider upgrading to higher performance policy or increasing QoS limits
- IOPS Sustained: Resize volume, rebalance workload, or upgrade performance tier
- Latency Breach: Investigate contention, consider volume relocation or QoS adjustment
- Latency Critical: Immediate investigation — check node/aggregate contention

## Provisioning Integration

### Checkbox field in action-form

Add a new `checkbox` field type to the ActionFormBlock component:

```json
{
  "key": "enable_monitoring",
  "label": "Enable Monitoring",
  "type": "checkbox",
  "defaultValue": "false"
}
```

The checkbox appears at the bottom of the provisioning action-form, after the
policy dropdowns. Default: unchecked.

### Post-provision flow

When the user clicks submit with monitoring enabled:

1. The UI sends a composed message including `enable_monitoring=true`
2. The LLM calls `create_volume` (via approval flow) to create the volume
3. After successful volume creation, the LLM calls `enable_volume_monitoring`
   with the volume name, SVM, cluster, size, and any selected policies
4. The alert management service creates the rule file and reloads vmalert
5. The LLM confirms: "Volume created and monitoring enabled with N alert rules"

If monitoring is not enabled (checkbox unchecked), step 3 is skipped.

## Volume-Detail Interest

### New bespoke interest: `volume-detail`

Replace the generic `resource-status` handling of volume queries with a
dedicated `volume-detail.md` interest.

**Triggers:**
- "tell me about volume X"
- "show volume X"
- "volume details for X"
- "what's going on with volume X"
- clicking a volume name in resource tables

**Output type:** `object-detail` (same as current resource-status for single
entities)

### Monitoring status detection

The interest prompt instructs the LLM to call `get_volume_monitoring_status`
for the queried volume. Based on the result:

**If monitoring is active:**
- Show a "Monitored" badge/status indicator in the properties section
- Include an actions section with a **"Stop Monitoring"** action button
- The button triggers: LLM calls `disable_volume_monitoring` → confirms removal
- Show current alert rules summary (which categories are active)

**If monitoring is NOT active:**
- Include an actions section with a **"Monitor this Volume"** action button
- The button triggers: LLM looks up volume attributes (size, QoS policy,
  snapshot policy) via ONTAP MCP, then calls `enable_volume_monitoring`
- The LLM resolves thresholds from the volume's actual policies

### Stop Monitoring UX

The "Stop Monitoring" button uses the existing action-button → approval flow:

1. User clicks "Stop Monitoring"
2. LLM calls `disable_volume_monitoring(volume, svm, cluster)` (requires approval)
3. Approval modal: "Remove all monitoring alert rules for volume X?"
4. On approval: rules deleted, vmalert reloaded
5. LLM confirms: "Monitoring disabled for volume X. N alert rules removed."

### Monitor Existing Volumes

For volumes not provisioned through host application (no prior policy context):

1. User clicks "Monitor this Volume" on volume-detail
2. LLM calls `ontap_get` to look up volume attributes:
   - Size → for IOPS threshold scaling
   - QoS policy assignment → for performance alert thresholds
   - Snapshot policy assignment → for data protection alert thresholds
3. LLM calls `enable_volume_monitoring` with the discovered attributes
4. Service creates rules with thresholds derived from actual policies

## Implementation Plan

### Phase 0: Enable Harvest MCP Alert CRUD
- [ ] Add `HARVEST_RULES_PATH` env var and volume mount to compose.yaml
- [ ] Add same config to dev environment (local compose / .env.custom)
- [ ] Verify alert CRUD tools appear in MCP tool discovery
- [ ] Manual smoke test: create and delete a test rule via chatbot

### Phase 1: UI Foundation
- [ ] Add `checkbox` field type to ActionFormBlock component
- [ ] Add `checkbox` to `ActionFormField` type in chartTypes.ts
- [ ] Unit tests for checkbox rendering, state, and form submission
- [ ] Update volumeProvision fixture with enable_monitoring checkbox

### Phase 2: Alert Management Agent
- [ ] Create `chat-service/internal/alertmgr` package
- [ ] Rule template engine: generates alert rule parameters from structured input
- [ ] Input sanitization: validate volume/SVM/cluster names, numeric thresholds
- [ ] ONTAP MCP integration: policy attribute lookup (QoS, snapshot) via Router
- [ ] Harvest MCP integration: create/delete/list rules, validate syntax, reload
- [ ] Batch create with rollback on failure
- [ ] Register as internal MCP tools in agent (with read-write mode gating)
- [ ] Unit tests for rule generation, sanitization, batch operations

### Phase 3: Provisioning Integration
- [ ] Update volume-provision.md prompt: add monitoring checkbox, post-create flow
- [ ] Handle `enable_monitoring=true` in submit message composition
- [ ] E2E test: provision with monitoring enabled

### Phase 4: Volume-Detail Interest
- [x] Create `volume-detail.md` interest with monitoring-aware actions
- [x] Integrate `get_volume_monitoring_status` in the prompt flow
- [x] "Stop Monitoring" action with approval flow
- [x] "Monitor this Volume" action with attribute lookup
- [x] Add monitoring awareness to `resource-status` interest (volumes)
- [ ] E2E tests for monitoring toggle in volume-detail view

## Resolved Questions

1. **Metric name verification** — Verified against Harvest metric catalog:
   - `volume_size_used_percent` ✅ — exists with `volume`, `svm`, `cluster` labels
   - `volume_inode_used_percent` ✅ — exists (computed: files_used / total)
   - `volume_total_ops` ✅ — exists with `volume`, `svm`, `cluster` labels
   - `volume_avg_latency` ✅ — exists in microseconds (replaces `volume_read_latency`
     to cover all I/O types, not just reads)
   - `volume_snapshot_reserve_used_percent` ✅ — note: requires `volume_` prefix
     (was incorrectly `snapshot_reserve_used_percent` in earlier draft)
   - `last_snapshot_age` ❌ — **does not exist**. Must compute as:
     `time() - max_over_time(snapshot_create_time{volume="V", svm="S"}[1h])`.
     The `snapshot_create_time` metric provides per-snapshot creation timestamps;
     we take the max (newest) and compare to current time.
2. **ONTAP MCP policy lookup**: `ontap_get` with paths `/storage/qos/policies`
   and `/storage/snapshot-policies` may return errors on some clusters
   (don't find cluster-level or default policies).
   This blocks threshold derivation for performance/data-protection alerts.
   Needs ONTAP MCP team fix. **Status: open, not blocking Phase 0–1.**
3. **Harvest MCP group_name support** — Verified via smoke test:
   `create_alert_rule` accepts a `group_name` parameter. Custom group names
   (e.g., `host application.volume.cluster.svm.vol`) are written correctly to
   `alert_rules.yml` as separate rule groups. ✅
4. **Rule limits** — No documented rule count limit per file in
   Prometheus/vmalert. At 10 rules × 100 volumes = 1,000 rules, this is well
   within normal Prometheus deployments. YAML parse time is negligible at this
   scale. Revisit only if monitoring 500+ volumes. ✅
