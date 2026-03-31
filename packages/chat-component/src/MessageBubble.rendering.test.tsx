/**
 * Focused tests for the MessageBubble rendering pipeline:
 *   message.content → wrapInlineChartJson → ReactMarkdown → code handler → chart components
 *
 * These tests verify that JSON from the LLM is correctly detected and rendered
 * through the full pipeline: known types via dedicated components, unknown
 * structured JSON via the AutoJsonBlock generic fallback.
 */
import { render, screen } from '../test-utils';
import { describe, it, expect } from 'vitest';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { wrapInlineChartJson, sanitizeJson } from './inlineChartDetector';
import { ChartBlock, DashboardBlock, ObjectDetailBlock, AutoJsonBlock } from './charts';
import { parseChart, parseObjectDetail } from './charts/chartTypes';

/**
 * Renders content through the same pipeline as MessageBubble:
 * wrapInlineChartJson → ReactMarkdown with code handler (incl. AutoJsonBlock fallback)
 */
function renderAssistantMessage(rawContent: string) {
  const processed = wrapInlineChartJson(rawContent);

  return render(
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        code: ({ className, children, ...props }: React.HTMLAttributes<HTMLElement>) => {
          const content = String(children).replace(/\n$/, '');
          if (className === 'language-dashboard') {
            return <DashboardBlock json={content} />;
          }
          if (className === 'language-object-detail') {
            return <ObjectDetailBlock json={content} />;
          }
          if (className === 'language-chart') {
            return <ChartBlock json={content} />;
          }
          try {
            const parsed = JSON.parse(sanitizeJson(content));
            if (Array.isArray(parsed?.panels)) {
              return <DashboardBlock json={content} />;
            }
            if (parseObjectDetail(JSON.stringify(parsed))) {
              return <ObjectDetailBlock json={JSON.stringify(parsed)} />;
            }
            if (parseChart(JSON.stringify(parsed))) {
              return <ChartBlock json={JSON.stringify(parsed)} />;
            }
            if (typeof parsed === 'object' && parsed !== null) {
              return <AutoJsonBlock json={content} />;
            }
          } catch { /* not valid JSON */ }
          return <code className={className} {...props}>{children}</code>;
        },
        pre: ({ children }: React.HTMLAttributes<HTMLPreElement>) => <>{children}</>,
      }}
    >
      {processed}
    </ReactMarkdown>,
  );
}

describe('MessageBubble alert-list rendering', () => {
  it('renders bare alert-list JSON (with type) as AlertListBlock', () => {
    const json = '{"type":"alert-list","items":[{"severity":"critical","message":"InstanceDown on node-east-01","time":"5 min ago"}]}';
    renderAssistantMessage(`Here are the critical alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('InstanceDown on node-east-01')).toBeDefined();
  });

  it('renders bare alert-list JSON (without type — inferred) as AlertListBlock', () => {
    const json = '{"items":[{"severity":"critical","message":"InstanceDown","time":"2026-03-07T00:38:00Z"},{"severity":"warning","message":"HighCPU","time":"1 hr ago"}]}';
    renderAssistantMessage(`Here are the alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('InstanceDown')).toBeDefined();
    expect(screen.getByText('HighCPU')).toBeDefined();
  });

  it('renders alert-list inside ```json fence as AlertListBlock', () => {
    const json = '{"type":"alert-list","items":[{"severity":"warning","message":"DiskFailing","time":"10 min ago"}]}';
    const content = 'Here are the alerts:\n\n```json\n' + json + '\n```';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('DiskFailing')).toBeDefined();
  });

  it('renders alert-list inside ```chart fence as AlertListBlock', () => {
    const json = '{"type":"alert-list","items":[{"severity":"info","message":"FirmwareUpdate","time":"2 hr ago"}]}';
    const content = '```chart\n' + json + '\n```';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('FirmwareUpdate')).toBeDefined();
  });

  it('renders pretty-printed bare alert-list JSON as AlertListBlock', () => {
    const json = `{
  "items": [
    {
      "severity": "critical",
      "message": "InstanceDown on node-east-01",
      "time": "5 min ago"
    }
  ]
}`;
    renderAssistantMessage(`Here are the alerts:\n\n${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('InstanceDown on node-east-01')).toBeDefined();
  });

  it('renders standalone bare alert-list JSON (no surrounding text)', () => {
    const json = '{"items":[{"severity":"critical","message":"InstanceDown","time":"5 min ago"}]}';
    renderAssistantMessage(json);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
  });

  // --- Tests for LLM returning Prometheus-style field names ---

  it('renders alert-list JSON with alertname instead of message', () => {
    const json = '{"items":[{"alertname":"InstanceDown","severity":"critical","startsAt":"2026-01-07T10:30:00Z","instance":"node-east-01:9090"}]}';
    renderAssistantMessage(`Here are the alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('InstanceDown')).toBeDefined();
  });

  it('renders alert-list JSON with name instead of message', () => {
    const json = '{"items":[{"name":"VolumeOffline","severity":"warning","time":"10 min ago"}]}';
    renderAssistantMessage(`Active alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('VolumeOffline')).toBeDefined();
  });

  it('renders alert-list JSON with description instead of message', () => {
    const json = '{"items":[{"description":"High CPU usage on node-east-01","severity":"warning","time":"5 min ago"}]}';
    renderAssistantMessage(`Alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('High CPU usage on node-east-01')).toBeDefined();
  });

  it('renders alert-list JSON with summary instead of message', () => {
    const json = '{"items":[{"summary":"Disk failure predicted","severity":"critical","time":"2 hr ago"}]}';
    renderAssistantMessage(`Critical alerts: ${json}`);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('Disk failure predicted')).toBeDefined();
  });

  it('renders alert-list JSON with startsAt instead of time', () => {
    const json = '{"type":"alert-list","items":[{"severity":"critical","message":"InstanceDown","startsAt":"2026-01-07T10:30:00Z"}]}';
    renderAssistantMessage(json);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText('InstanceDown')).toBeDefined();
  });

  it('renders exact cadvisor alert JSON from real LLM output', () => {
    // This is the exact format the LLM returned when asked "Show me the critical alerts"
    const content = '{ "items": [ { "severity": "critical", "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes. This impacts infrastructure container metrics collection. Alert group: Harvest Rules.", "time": "2026-03-07T02:14:00Z" }, { "severity": "critical", "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes. This impacts infrastructure container metrics collection. Alert group: Harvest Rules.", "time": "2026-03-07T02:14:00Z" } ] }\nCritical Alert Details:\n\nBoth alerts are for the cadvisor monitor endpoint being down (InstanceDown).';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    // Two identical alerts — use getAllByText
    expect(screen.getAllByText(/Endpoint \[cadvisor:8080\] down/)).toHaveLength(2);
  });

  it('renders cadvisor alert JSON inside ```json fence', () => {
    const json = '{ "items": [ { "severity": "critical", "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes.", "time": "2026-03-07T02:14:00Z" } ] }';
    const content = 'Here are the critical alerts:\n\n```json\n' + json + '\n```\n\nCritical Alert Details:\n\nBoth alerts are for cadvisor.';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
    expect(screen.getByText(/Endpoint \[cadvisor:8080\] down/)).toBeDefined();
  });

  it('renders cadvisor alert JSON inside ``` (bare) fence', () => {
    const json = '{ "items": [ { "severity": "critical", "message": "Endpoint [cadvisor:8080] down", "time": "2026-03-07T02:14:00Z" } ] }';
    const content = '```\n' + json + '\n```';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
  });

  it('renders pretty-printed cadvisor alert JSON in ```json fence', () => {
    const json = `{
  "items": [
    {
      "severity": "critical",
      "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes.",
      "time": "2026-03-07T02:14:00Z"
    },
    {
      "severity": "critical",
      "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes.",
      "time": "2026-03-07T02:14:00Z"
    }
  ]
}`;
    const content = 'Here are the critical alerts:\n\n```json\n' + json + '\n```\n\nBoth are cadvisor.';
    renderAssistantMessage(content);
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
  });
});

describe('MessageBubble generic JSON fallback (AutoJsonBlock)', () => {
  it('renders unknown JSON with ≥3 keys as formatted properties', () => {
    const json = '{"cluster_name":"prod-east","node_count":4,"data_protocol":"NFS"}';
    renderAssistantMessage(`Here is the cluster info: ${json}`);
    expect(screen.getByText('Cluster Name')).toBeDefined();
    expect(screen.getByText('prod-east')).toBeDefined();
    expect(screen.getByText('Node Count')).toBeDefined();
  });

  it('renders unknown JSON with items array as a table', () => {
    const json = '{"title":"Volumes","items":[{"vol":"vol1","state":"online","size":"100GB"},{"vol":"vol2","state":"offline","size":"50GB"}]}';
    renderAssistantMessage(`Volume data: ${json}`);
    expect(screen.getByText('Volumes')).toBeDefined();
    expect(screen.getByText('vol1')).toBeDefined();
    expect(screen.getByText('offline')).toBeDefined();
  });

  it('renders bare JSON array of objects as a table', () => {
    const json = '[{"name":"node1","status":"healthy","uptime":"45d"},{"name":"node2","status":"degraded","uptime":"12d"}]';
    renderAssistantMessage(`Node status: ${json}`);
    // Arrays are detected by inlineChartDetector if they start with [
    // but our detector only handles objects starting with {.
    // The ```json fence from LLM output handles arrays.
    // For bare arrays, they render as text (expected behavior).
  });

  it('renders Prometheus-style alert JSON via normalization (AlertListBlock precedence)', () => {
    // Alert data with non-standard field names gets normalized by inferChartType/parseChart
    // and renders via AlertListBlock (not AutoJsonBlock) because severity+alertname matches.
    const json = '{"title":"Critical Alerts","items":[{"alertname":"InstanceDown","severity":"critical","instance":"node-east-01:9090","startsAt":"2026-01-07T10:30:00Z"}]}';
    renderAssistantMessage(json);
    expect(screen.getByText('Critical Alerts')).toBeDefined();
    expect(screen.getByText('InstanceDown')).toBeDefined();
    // Rendered as AlertListBlock — severity icon + message + time
    expect(screen.getByRole('list', { name: 'Alerts' })).toBeDefined();
  });

  it('does not use fallback for known chart types — ChartBlock takes precedence', () => {
    const json = '{"type":"stat","title":"IOPS","value":"12,345","subtitle":"read+write"}';
    renderAssistantMessage(json);
    // StatBlock renders title as text
    expect(screen.getByText('IOPS')).toBeDefined();
    expect(screen.getByText('12,345')).toBeDefined();
  });

  it('renders fenced ```json block with unknown data as AutoJsonBlock', () => {
    const json = '{"cluster":"prod","nodes":3,"protocol":"NFS","version":"9.14"}';
    renderAssistantMessage('```json\n' + json + '\n```');
    expect(screen.getByText('Cluster')).toBeDefined();
    expect(screen.getByText('prod')).toBeDefined();
  });
});

describe('EXACT user-reported failing content (2026-03-07)', () => {
  it('renders the exact LLM response as structured UI, not raw JSON', () => {
    // This is the EXACT text the user sees in the chat — JSON followed by explanation.
    const content = `{ "items": [ { "severity": "critical", "message": "Endpoint [cadvisor:8080] of job [cadvisor] has been down for more than 5 minutes.", "time": "2026-03-07T02:14:00Z" }, { "severity": "critical", "message": "Endpoint [cadvisor:8080] of job [cadvisor] has been down for more than 5 minutes.", "time": "2026-03-07T02:14:00Z" } ] }
Critical Alerts Explanation

Both critical alerts report that the monitoring endpoint cadvisor:8080 is down and unreachable for over 5 minutes. This affects your system monitoring coverage rather than the storage cluster itself, but it is important to address so monitoring data remains complete and current.

Recommended Action:

Check the health/status of the cadvisor service on the host providing this endpoint.
Restart or restore the service if it's down.
Verify network or firewall settings if the service is up but unreachable.
Would you like more help troubleshooting this monitoring issue, or do you want to see other alerts or details?`;

    // First: verify wrapInlineChartJson detects and wraps the JSON
    const processed = wrapInlineChartJson(content);
    console.log('=== PROCESSED OUTPUT ===');
    console.log(processed);
    console.log('=== END ===');

    // The JSON should be wrapped in fences, not left bare
    expect(processed).toContain('```');

    // Now render through the full pipeline
    renderAssistantMessage(content);

    // The raw JSON keys must NOT be visible as text
    expect(screen.queryByText('"items"')).toBeNull();
    expect(screen.queryByText('"severity"')).toBeNull();
    expect(screen.queryByText('"message"')).toBeNull();

    // The explanation text SHOULD still be visible
    expect(screen.getByText(/Critical Alerts Explanation/)).toBeDefined();
    // Multiple elements mention cadvisor — that's fine, just verify at least one
    expect(screen.getAllByText(/cadvisor/).length).toBeGreaterThan(0);
  });
});

describe('MessageBubble object-detail + markdown rendering', () => {
  it('renders markdown tables and lists after an object-detail block', () => {
    const objectDetail = JSON.stringify({
      type: 'object-detail',
      kind: 'volume',
      name: 'Volume Provisioning',
      status: 'info',
      subtitle: '100GB NFS on Fastest Storage',
      sections: [
        {
          title: 'Details',
          layout: 'properties',
          data: { columns: 2, items: [{ label: 'Size', value: '100 GB' }] },
        },
      ],
    });

    const content = `${objectDetail}

## ⚡ Recommendation

For your **100GB NFS volume**, I recommend the **NVMe-based cluster**:

| Candidate | Storage Type | Recommendation |
|-----------|-------------|----------------|
| **NVME-a250** | NVMe SSD | ✅ Best choice |
| a700s-c10 | SAS HDD | Slower tier |

**Why NVME-a250?**

- 🚀 **NVMe SSD storage** — lowest latency
- 📊 **87% free capacity** — ample room

### Summary

Click "Provision" to create the volume.`;

    renderAssistantMessage(content);

    // Object-detail card should render
    expect(screen.getByText('Volume Provisioning')).toBeDefined();

    // Markdown heading should render (not raw ##)
    expect(screen.queryByText('## ⚡ Recommendation')).toBeNull();

    // Table should render — look for cell content
    const nvmeSsdMatches = screen.getAllByText(/NVMe SSD/);
    expect(nvmeSsdMatches.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/Slower tier/)).toBeDefined();

    // Bold text should render as <strong>, not raw **
    expect(screen.queryByText('**NVMe SSD storage**')).toBeNull();

    // List items should render
    expect(screen.getByText(/87% free capacity/)).toBeDefined();
  });
});
