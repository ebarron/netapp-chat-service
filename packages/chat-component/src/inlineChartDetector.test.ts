import { describe, it, expect } from 'vitest';
import { wrapInlineChartJson, hideIncompleteChartJson, sanitizeJson } from './inlineChartDetector';

describe('sanitizeJson', () => {
  it('strips trailing commas', () => {
    expect(sanitizeJson('{"a":1,}')).toBe('{"a":1}');
  });

  it('strips single-line // comments', () => {
    const input = '{\n  "a": 1,\n  // a comment\n  "b": 2\n}';
    const result = sanitizeJson(input);
    expect(JSON.parse(result)).toEqual({ a: 1, b: 2 });
  });

  it('strips truncation comments from LLM output', () => {
    const input = '{"data":[\n  {"x":1},\n  // ... (additional points truncated for brevity)\n]}';
    const result = sanitizeJson(input);
    expect(JSON.parse(result)).toEqual({ data: [{ x: 1 }] });
  });
});

describe('wrapInlineChartJson', () => {
  it('returns plain text unchanged', () => {
    const input = 'Hello, here is some plain text.';
    expect(wrapInlineChartJson(input)).toBe(input);
  });

  it('wraps a bare bar chart JSON in a ```chart fence', () => {
    const json = '{"type":"bar","title":"Test","xKey":"x","series":[],"data":[]}';
    const input = `Here is a chart: ${json} and some text after.`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n' + json + '\n```');
    expect(result).toContain('Here is a chart:');
    expect(result).toContain('and some text after.');
  });

  it('wraps a bare area chart JSON', () => {
    const json = '{"type":"area","title":"Trend","xKey":"time","series":[{"key":"v","label":"Value"}],"data":[]}';
    const result = wrapInlineChartJson(`Look at this: ${json}`);
    expect(result).toContain('```chart\n' + json + '\n```');
  });

  it('wraps a bare dashboard JSON in a ```dashboard fence', () => {
    const json = '{"title":"My Dashboard","panels":[{"type":"stat","title":"CPU","value":"42%"}]}';
    const input = `Dashboard: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```dashboard\n' + json + '\n```');
  });

  it('does not double-wrap JSON already in a ```chart fence', () => {
    const json = '{"type":"bar","title":"Test","xKey":"x","series":[],"data":[]}';
    const input = '```chart\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    // Should be identical — no second wrapping.
    expect(result).toBe(input);
  });

  it('does not double-wrap JSON already in a ```dashboard fence', () => {
    const json = '{"title":"Dash","panels":[]}';
    const input = '```dashboard\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('leaves non-chart JSON objects alone', () => {
    const json = '{"name":"Alice","age":30}';
    const input = `User: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('handles mixed fenced and inline chart JSON', () => {
    const fencedJson = '{"type":"stat","title":"Fenced","value":"1"}';
    const bareJson = '{"type":"gauge","title":"Inline","value":50,"max":100}';
    const input = '```chart\n' + fencedJson + '\n```\nAlso see: ' + bareJson;
    const result = wrapInlineChartJson(input);
    // The fenced one should remain as-is.
    expect(result).toContain('```chart\n' + fencedJson + '\n```');
    // The bare one should be wrapped.
    expect(result).toContain('```chart\n' + bareJson + '\n```');
  });

  it('handles the exact user-reported failing case', () => {
    const json = `{ "type": "bar", "title": "Cluster Capacity Overview", "xKey": "Cluster", "series": [ {"key":"Total","label":"Total (TB)","color":"#0080FF"}, {"key":"Used","label":"Used (TB)","color":"#FF5733"} ], "data": [ { "Cluster": "sa-tme-flexpod-NVME-a250", "Total": 33.44, "Used": 2.48 }, { "Cluster": "sa-tme-flexpod-a700s-c10", "Total": 33.44, "Used": 2.47 } ] }`;
    const input = `Here is a comparison of total and used capacity for each ONTAP cluster:\n\n${json}\n\n"Total" shows overall physical space.`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n' + json + '\n```');
    expect(result).not.toContain(json + '\n\n"Total"'); // Should be wrapped, not raw.
  });

  it('handles curly braces in regular text without crashing', () => {
    const input = 'Use {placeholder} syntax in templates like { this }.';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('handles unbalanced braces gracefully', () => {
    const input = 'Some text { with an unbalanced brace and more text.';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('wraps multiple inline chart JSONs in a single message', () => {
    const chart1 = '{"type":"stat","title":"A","value":"1"}';
    const chart2 = '{"type":"stat","title":"B","value":"2"}';
    const input = `First: ${chart1} Second: ${chart2}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n' + chart1 + '\n```');
    expect(result).toContain('```chart\n' + chart2 + '\n```');
  });

  it('handles JSON with nested objects (data array with objects)', () => {
    const json = '{"type":"bar","title":"T","xKey":"x","series":[{"key":"a","label":"A"}],"data":[{"x":"v1","a":10},{"x":"v2","a":20}]}';
    const result = wrapInlineChartJson(`Chart: ${json}`);
    expect(result).toContain('```chart\n' + json + '\n```');
  });

  it('handles empty input', () => {
    expect(wrapInlineChartJson('')).toBe('');
  });

  it('handles JSON with escaped quotes in strings', () => {
    const json = '{"type":"stat","title":"Cluster \\"A\\"","value":"42%"}';
    const result = wrapInlineChartJson(`Info: ${json}`);
    expect(result).toContain('```chart\n' + json + '\n```');
  });

  it('retags a ```json fence containing a dashboard to ```dashboard', () => {
    const json = '{"title":"Fleet Health","panels":[{"type":"alert-summary","data":{"critical":0}}]}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```dashboard\n' + json + '\n```');
  });

  it('retags a ```json fence containing a chart to ```chart', () => {
    const json = '{"type":"area","title":"Trend","xKey":"time","series":[],"data":[]}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```chart\n' + json + '\n```');
  });

  it('leaves ```json fence with non-chart content unchanged', () => {
    const json = '{"name":"Alice","age":30}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('handles dashboard JSON with trailing commas (common LLM error)', () => {
    const json = '{"title":"Fleet Health","panels":[{"type":"alert-summary","data":{"critical":0,"warning":0,}},]}';
    const input = `Dashboard: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```dashboard\n' + json + '\n```');
  });

  it('handles chart JSON with trailing commas in data arrays', () => {
    const json = '{"type":"bar","title":"Test","xKey":"x","series":[{"key":"a","label":"A"},],"data":[{"x":"v1","a":10},]}';
    const input = `Chart: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n' + json + '\n```');
  });

  it('retags ```json fence with trailing commas to ```dashboard', () => {
    const json = '{"title":"Fleet","panels":[{"type":"stat","title":"A","value":"1"},]}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```dashboard\n' + json + '\n```');
  });

  it('infers alert-list from bare JSON with items containing severity+message', () => {
    const json = '{"items":[{"severity":"critical","message":"InstanceDown","time":"2026-03-07T00:38:00Z"}]}';
    const input = `Here are the alerts: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
    expect(result).toContain(json);
    expect(result).toContain('Here are the alerts:');
  });

  it('infers alert-list from ```json fence with items array', () => {
    const json = '{"items":[{"severity":"warning","message":"HighCPU","time":"2026-01-01T00:00:00Z"}]}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n' + json + '\n```');
  });

  it('infers alert-list from bare JSON with alertname instead of message', () => {
    const json = '{"items":[{"alertname":"InstanceDown","severity":"critical","startsAt":"2026-01-07T10:30:00Z"}]}';
    const input = `Alerts: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
    expect(result).toContain(json);
  });

  it('infers alert-list from bare JSON with description instead of message', () => {
    const json = '{"items":[{"description":"High CPU usage","severity":"warning","time":"5 min ago"}]}';
    const input = `Warnings: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
    expect(result).toContain(json);
  });

  it('infers gauge from bare JSON with value+max but no type', () => {
    const json = '{"title":"CPU Usage","value":85,"max":100}';
    const input = `Here is the gauge: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
    expect(result).toContain(json);
  });

  it('infers status-grid from bare JSON with items containing name+status', () => {
    const json = '{"title":"Node Status","items":[{"name":"node1","status":"ok"},{"name":"node2","status":"error"}]}';
    const input = `Status: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
    expect(result).toContain(json);
  });

  it('wraps unknown structured JSON (≥3 keys) in ```json fence', () => {
    const json = '{"cluster":"prod-east","nodes":4,"protocol":"NFS","version":"9.14"}';
    const input = `Cluster info: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```json\n');
    expect(result).toContain(json);
    expect(result).toContain('Cluster info:');
  });

  it('wraps unknown JSON with nested array of objects in ```json fence', () => {
    const json = '{"title":"Disks","items":[{"name":"disk1","type":"SSD"},{"name":"disk2","type":"HDD"}]}';
    const input = `Disk data: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```json\n');
    expect(result).toContain(json);
  });

  it('does not wrap trivial JSON objects (<3 keys, no nested arrays)', () => {
    const json = '{"ok":true}';
    const input = `Result: ${json}`;
    const result = wrapInlineChartJson(input);
    // Should not be fenced — too trivial
    expect(result).not.toContain('```');
    expect(result).toContain(json);
  });

  it('wraps exact user alert JSON (items with severity+message+time) as chart', () => {
    const json = '{ "items": [ { "severity": "critical", "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes. This impacts infrastructure container metrics collection. Alert group: Harvest Rules.", "time": "2026-03-07T02:14:00Z" }, { "severity": "critical", "message": "Endpoint [cadvisor:8080] down: cadvisor monitoring endpoint has been unreachable for more than 5 minutes. This impacts infrastructure container metrics collection. Alert group: Harvest Rules.", "time": "2026-03-07T02:14:00Z" } ] }';
    const input = `${json}\nCritical Alert Details:\n\nBoth alerts are for the cadvisor monitor endpoint being down.`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart\n');
  });

  it('retags ```json fence with alert-list items to ```chart', () => {
    const json = '{ "items": [ { "severity": "critical", "message": "Endpoint down", "time": "2026-03-07T02:14:00Z" } ] }';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```chart\n' + json + '\n```');
  });
});

describe('hideIncompleteChartJson', () => {
  const PLACEHOLDER = '\n\n*Building dashboard…*\n\n';

  it('returns complete chart JSON normally (no placeholder)', () => {
    const json = '{"type":"bar","title":"Test","xKey":"x","series":[],"data":[]}';
    const input = `Here is a chart: ${json}`;
    const result = hideIncompleteChartJson(input);
    expect(result).toContain('```chart\n' + json + '\n```');
    expect(result).not.toContain('Building dashboard');
  });

  it('returns complete dashboard JSON normally', () => {
    const json = '{"title":"Dash","panels":[{"type":"stat","title":"A","value":"1"}]}';
    const input = `Dashboard: ${json}`;
    const result = hideIncompleteChartJson(input);
    expect(result).toContain('```dashboard\n' + json + '\n```');
    expect(result).not.toContain('Building dashboard');
  });

  it('replaces incomplete bare dashboard JSON with placeholder', () => {
    // Simulates a partially streamed dashboard — braces not balanced
    const input = 'Here is your fleet overview:\n\n{"title":"Fleet Health","panels":[{"type":"alert-summary","data":{"critical":0';
    const result = hideIncompleteChartJson(input);
    expect(result).toContain(PLACEHOLDER);
    expect(result).not.toContain('"panels"');
  });

  it('replaces incomplete bare chart JSON with placeholder', () => {
    const input = 'Trend:\n\n{"type":"area","title":"IOPS","xKey":"time","series":[{"key":"iops"';
    const result = hideIncompleteChartJson(input);
    expect(result).toContain(PLACEHOLDER);
    expect(result).not.toContain('"type":"area"');
  });

  it('replaces incomplete ```json fence containing chart data with placeholder', () => {
    const input = 'Here is the dashboard:\n\n```json\n{"title":"Fleet","panels":[{"type":"stat"';
    const result = hideIncompleteChartJson(input);
    expect(result).toContain(PLACEHOLDER);
    expect(result).not.toContain('"panels"');
  });

  it('leaves incomplete non-chart JSON alone', () => {
    const input = 'User info: {"name":"Alice","age":';
    const result = hideIncompleteChartJson(input);
    // Non-chart incomplete JSON should NOT be replaced
    expect(result).not.toContain('Building dashboard');
    expect(result).toContain('"name"');
  });

  it('leaves plain text unchanged', () => {
    const input = 'Hello, how can I help you today?';
    const result = hideIncompleteChartJson(input);
    expect(result).toBe(input);
  });

  it('preserves text before partial chart JSON', () => {
    const input = 'Here is the overview of your fleet.\n\n{"title":"Health","panels":[{"type":"alert-summary"';
    const result = hideIncompleteChartJson(input);
    expect(result).toContain('Here is the overview of your fleet.');
    expect(result).toContain(PLACEHOLDER);
  });

  it('handles already-complete block followed by incomplete block', () => {
    const completeChart = '{"type":"stat","title":"Done","value":"42%"}';
    const incompleteChart = '{"type":"area","title":"Trend","xKey":"time","series":[{"key":"v"';
    const input = `Here: ${completeChart}\n\nAlso: ${incompleteChart}`;
    const result = hideIncompleteChartJson(input);
    // The complete chart should render normally
    expect(result).toContain('```chart\n' + completeChart + '\n```');
    // The incomplete chart should be replaced
    expect(result).toContain(PLACEHOLDER);
  });
});

describe('bare and mistagged fences', () => {
  it('retags a bare ``` fence containing dashboard JSON', () => {
    const json = '{"title":"Fleet","panels":[{"type":"stat","title":"A","value":"1"}]}';
    const input = '```\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```dashboard\n' + json + '\n```');
  });

  it('retags a bare ``` fence containing chart JSON', () => {
    const json = '{"type":"area","title":"Trend","xKey":"time","series":[],"data":[]}';
    const input = '```\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```chart\n' + json + '\n```');
  });

  it('retags a ```text fence containing dashboard JSON', () => {
    const json = '{"title":"Dash","panels":[{"type":"callout","title":"Info","body":"test"}]}';
    const input = '```text\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```dashboard\n' + json + '\n```');
  });

  it('retags a ```plaintext fence containing dashboard JSON', () => {
    const json = '{"title":"Dash","panels":[{"type":"callout","title":"Info","body":"test"}]}';
    const input = '```plaintext\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```dashboard\n' + json + '\n```');
  });

  it('leaves a bare ``` fence with non-chart JSON unchanged', () => {
    const json = '{"name":"Alice","age":30}';
    const input = '```\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('retags a bare ``` fence with structured JSON (≥3 keys) to ```json', () => {
    const json = '{"cluster":"prod","nodes":4,"protocol":"NFS","version":"9.14"}';
    const input = '```\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```json\n' + json + '\n```');
  });

  it('retags a ```text fence with structured JSON to ```json', () => {
    const json = '{"cluster":"prod","nodes":4,"protocol":"NFS","version":"9.14"}';
    const input = '```text\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```json\n' + json + '\n```');
  });

  it('keeps ```json fence with structured JSON unchanged', () => {
    const json = '{"cluster":"prod","nodes":4,"protocol":"NFS","version":"9.14"}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```json\n' + json + '\n```');
  });
});

// ---------------------------------------------------------------------------
// object-detail detection and wrapping
// ---------------------------------------------------------------------------
describe('object-detail detection', () => {
  it('wraps bare object-detail JSON in an ```object-detail fence', () => {
    const json = '{"type":"object-detail","kind":"alert","name":"InstanceDown","sections":[]}';
    const input = `Detail: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```object-detail\n' + json + '\n```');
  });

  it('classifies object-detail by shape (kind + name + sections) without type field', () => {
    const json = '{"kind":"volume","name":"vol_prod_01","sections":[{"title":"Props","layout":"properties","data":{}}]}';
    const input = `Info: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```object-detail\n' + json + '\n```');
  });

  it('retags a ```json fence containing object-detail to ```object-detail', () => {
    const json = '{"type":"object-detail","kind":"alert","name":"Test","sections":[]}';
    const input = '```json\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe('```object-detail\n' + json + '\n```');
  });

  it('does not double-wrap JSON already in an ```object-detail fence', () => {
    const json = '{"type":"object-detail","kind":"alert","name":"Test","sections":[]}';
    const input = '```object-detail\n' + json + '\n```';
    const result = wrapInlineChartJson(input);
    expect(result).toBe(input);
  });

  it('dashboard JSON is still classified as dashboard (no regression)', () => {
    const json = '{"title":"Fleet","panels":[{"type":"stat","title":"A","value":"1"}]}';
    const input = `Dashboard: ${json}`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```dashboard\n' + json + '\n```');
  });

  it('wraps pretty-printed multi-line alert-list JSON (real LLM format)', () => {
    const json = `{
      "type": "alert-list",
      "items": [
        {
          "severity": "critical",
          "message": "Endpoint [cadvisor:8080] down — [cadvisor:8080] of job [cadvisor] has been down for more than 5 minutes.",
          "time": "2026-03-07T02:14:00Z"
        },
        {
          "severity": "critical",
          "message": "Endpoint [cadvisor:8080] down — [cadvisor:8080] of job [cadvisor] has been down for more than 5 minutes.",
          "time": "2026-03-07T02:14:00Z"
        }
      ]
    }`;
    const input = `Here are your alerts:\n${json}\nYou currently have 2 critical alerts.`;
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart');
    expect(result).toContain('```');
    // The surrounding text should be preserved
    expect(result).toContain('Here are your alerts:');
    expect(result).toContain('You currently have 2 critical alerts.');
  });

  it('handles JSON wrapped in single backticks (inline code from LLM)', () => {
    const json = '{"type":"alert-list","items":[{"severity":"critical","message":"Endpoint down","time":"2026-03-07T02:14:00Z"}]}';
    const input = 'Here are your alerts: `' + json + '`\nYou currently have 1 critical alert.';
    const result = wrapInlineChartJson(input);
    // The JSON should be detected and properly fenced
    expect(result).toContain('```chart');
    // No stray backticks wrapping the fenced block
    expect(result).not.toContain('`\n\n```chart');
    expect(result).not.toContain('```\n\n`');
  });

  it('handles pretty-printed JSON wrapped in single backticks', () => {
    const json = `{\n  "type": "alert-list",\n  "items": [\n    {\n      "severity": "critical",\n      "message": "Endpoint down",\n      "time": "2026-03-07T02:14:00Z"\n    }\n  ]\n}`;
    const input = 'Here are your alerts:\n`' + json + '`\nYou currently have 1 critical alert.';
    const result = wrapInlineChartJson(input);
    expect(result).toContain('```chart');
    // No stray backticks wrapping the fenced block
    expect(result).not.toContain('`\n\n```chart');
    expect(result).not.toContain('```\n\n`');
  });

  it('replaces incomplete bare object-detail JSON with placeholder during streaming', () => {
    const input = 'Here is the detail:\n\n{"type":"object-detail","kind":"alert","name":"Test","sections":[{"title":"Props"';
    const result = hideIncompleteChartJson(input);
    expect(result).toContain('*Building dashboard');
    expect(result).not.toContain('"sections"');
  });
});
