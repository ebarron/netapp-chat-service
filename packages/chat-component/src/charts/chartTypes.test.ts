import { describe, it, expect } from 'vitest';
import { parseDashboard, parseChart, parseObjectDetail, downsamplePanel, inferChartType } from './chartTypes';
import type { AreaChartData, SparklineData, GaugeData } from './chartTypes';

describe('parseDashboard', () => {
  it('parses a valid dashboard JSON', () => {
    const json = JSON.stringify({
      title: 'Test Dashboard',
      panels: [
        { type: 'area', title: 'Trend', xKey: 'time', series: [], data: [] },
        { type: 'stat', title: 'Count', value: '42' },
      ],
    });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Test Dashboard');
    expect(result!.panels).toHaveLength(2);
    expect(result!.panels[0].type).toBe('area');
    expect(result!.panels[1].type).toBe('stat');
  });

  it('returns null for invalid JSON', () => {
    expect(parseDashboard('not valid json')).toBeNull();
  });

  it('returns null when title is missing', () => {
    const json = JSON.stringify({ panels: [] });
    expect(parseDashboard(json)).toBeNull();
  });

  it('returns null when panels is not an array', () => {
    const json = JSON.stringify({ title: 'Test', panels: 'not-array' });
    expect(parseDashboard(json)).toBeNull();
  });

  it('skips unknown panel types', () => {
    const json = JSON.stringify({
      title: 'Test',
      panels: [
        { type: 'area', title: 'Valid', xKey: 'x', series: [], data: [] },
        { type: 'unknown-chart-type', title: 'Invalid' },
        { type: 'bar', title: 'Also Valid', xKey: 'x', series: [], data: [] },
      ],
    });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.panels).toHaveLength(2);
    expect(result!.panels[0].type).toBe('area');
    expect(result!.panels[1].type).toBe('bar');
  });

  it('infers panel type from shape when type is missing', () => {
    const json = JSON.stringify({
      title: 'Provisioning',
      panels: [
        { type: 'callout', title: 'Rec', body: 'Use cluster A', width: 'full' },
        // action-button with no type — LLM omitted it
        { buttons: [{ label: 'Provision', tool: 'ontap_volume_create', params: { size: '100GB' } }], width: 'full' },
        // proposal with no type
        { title: 'CLI Command', command: 'volume create -vserver vs1 -volume vol1 -size 100GB' },
      ],
    });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.panels).toHaveLength(3);
    expect(result!.panels[0].type).toBe('callout');
    expect(result!.panels[1].type).toBe('action-button');
    expect(result!.panels[2].type).toBe('proposal');
  });

  it('skips null/non-object panels', () => {
    const json = JSON.stringify({
      title: 'Test',
      panels: [null, 42, 'string', { type: 'stat', title: 'OK', value: '1' }],
    });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.panels).toHaveLength(1);
  });

  it('returns null for a plain string', () => {
    expect(parseDashboard('"just a string"')).toBeNull();
  });

  it('returns null for null JSON', () => {
    expect(parseDashboard('null')).toBeNull();
  });

  it('tolerates trailing commas in JSON (common LLM error)', () => {
    const json = '{"title":"Fleet","panels":[{"type":"stat","title":"A","value":"1",},]}';
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Fleet');
    expect(result!.panels).toHaveLength(1);
  });

  it('strips single-line // comments from JSON (common LLM error)', () => {
    const json = [
      '{',
      '  "title": "Health",',
      '  "panels": [',
      '    // Performance panel',
      '    { "type": "stat", "title": "IOPS", "value": "42" }',
      '    // ... (additional panels truncated for brevity)',
      '  ]',
      '}',
    ].join('\n');
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Health');
    expect(result!.panels).toHaveLength(1);
    expect(result!.panels[0].type).toBe('stat');
  });

  it('handles all known panel types', () => {
    const panels = [
      { type: 'area', title: 'A', xKey: 'x', series: [], data: [] },
      { type: 'bar', title: 'B', xKey: 'x', series: [], data: [] },
      { type: 'gauge', title: 'G', value: 82, max: 100 },
      { type: 'sparkline', data: [1, 2, 3] },
      { type: 'status-grid', title: 'S', items: [] },
      { type: 'stat', title: 'St', value: '5' },
      { type: 'alert-summary', data: { critical: 0, warning: 0, info: 0, ok: 0 } },
      { type: 'resource-table', title: 'R', columns: [], rows: [] },
      { type: 'alert-list', items: [] },
      { type: 'callout', title: 'C', body: 'text' },
      { type: 'proposal', title: 'P', command: 'cmd' },
      { type: 'action-button', buttons: [] },
    ];
    const json = JSON.stringify({ title: 'All Types', panels });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.panels).toHaveLength(12);
  });
});

describe('parseChart', () => {
  it('parses a valid chart JSON', () => {
    const json = JSON.stringify({
      type: 'area',
      title: 'Test',
      xKey: 'time',
      series: [{ key: 'val', label: 'Value' }],
      data: [{ time: 'Mon', val: 10 }],
    });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('area');
  });

  it('strips // comments from chart JSON', () => {
    const json = [
      '{',
      '  "type": "gauge",',
      '  "title": "CPU",',
      '  // this value comes from the metrics query',
      '  "value": 85,',
      '  "max": 100',
      '}',
    ].join('\n');
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('gauge');
  });

  it('returns null for invalid JSON', () => {
    expect(parseChart('bad json')).toBeNull();
  });

  it('returns null for unknown chart type', () => {
    const json = JSON.stringify({ type: 'pie', data: [] });
    expect(parseChart(json)).toBeNull();
  });

  it('returns null for null JSON', () => {
    expect(parseChart('null')).toBeNull();
  });

  it('returns null for non-object JSON', () => {
    expect(parseChart('42')).toBeNull();
  });

  it('parses all standalone chart types', () => {
    const types = ['area', 'bar', 'gauge', 'sparkline', 'status-grid', 'stat',
      'alert-summary', 'resource-table', 'alert-list', 'callout', 'proposal', 'action-button'];
    for (const type of types) {
      const json = JSON.stringify({ type, title: 'T', value: 1, data: [], items: [], buttons: [], columns: [], rows: [], xKey: 'x', series: [], body: '', command: '', max: 100 });
      const result = parseChart(json);
      expect(result).not.toBeNull();
      expect(result!.type).toBe(type);
    }
  });
});

// ---------------------------------------------------------------------------
// downsamplePanel / data-point limits (§5.2)
// ---------------------------------------------------------------------------
describe('downsamplePanel', () => {
  it('does not modify area chart with ≤200 data points', () => {
    const panel: AreaChartData = {
      type: 'area',
      title: 'Small',
      xKey: 'time',
      series: [{ key: 'v', label: 'V' }],
      data: Array.from({ length: 200 }, (_, i) => ({ time: i, v: i })),
    };
    const result = downsamplePanel(panel);
    expect(result.type).toBe('area');
    if (result.type === 'area') {
      expect(result.data).toHaveLength(200);
    }
  });

  it('downsamples area chart with >200 data points to 200', () => {
    const panel: AreaChartData = {
      type: 'area',
      title: 'Big',
      xKey: 'time',
      series: [{ key: 'v', label: 'V' }],
      data: Array.from({ length: 500 }, (_, i) => ({ time: i, v: i * 10 })),
    };
    const result = downsamplePanel(panel);
    if (result.type === 'area') {
      expect(result.data.length).toBeLessThanOrEqual(200);
      // First and last elements preserved
      expect(result.data[0]).toEqual({ time: 0, v: 0 });
      expect(result.data[result.data.length - 1]).toEqual({ time: 499, v: 4990 });
    }
  });

  it('downsamples sparkline with >200 data points to 200', () => {
    const panel: SparklineData = {
      type: 'sparkline',
      data: Array.from({ length: 500 }, (_, i) => i),
    };
    const result = downsamplePanel(panel);
    if (result.type === 'sparkline') {
      expect(result.data.length).toBeLessThanOrEqual(200);
      expect(result.data[0]).toBe(0);
      expect(result.data[result.data.length - 1]).toBe(499);
    }
  });

  it('does not modify non-array types like gauge', () => {
    const panel: GaugeData = {
      type: 'gauge',
      title: 'CPU',
      value: 85,
      max: 100,
    };
    const result = downsamplePanel(panel);
    expect(result).toEqual(panel);
  });
});

describe('parseChart data-point limits', () => {
  it('downsamples a chart with 500 data points to ≤200', () => {
    const json = JSON.stringify({
      type: 'sparkline',
      data: Array.from({ length: 500 }, (_, i) => i),
    });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    if (result && result.type === 'sparkline') {
      expect(result.data.length).toBeLessThanOrEqual(200);
    }
  });
});

// ---------------------------------------------------------------------------
// inferChartType — shape-based type detection
// ---------------------------------------------------------------------------
describe('inferChartType', () => {
  it('infers bar from xKey + series + data', () => {
    expect(inferChartType({ xKey: 'x', series: [{ key: 'a', label: 'A' }], data: [{ x: 1, a: 2 }] })).toBe('bar');
  });

  it('infers area when yLabel is present', () => {
    expect(inferChartType({ xKey: 'x', yLabel: 'IOPS', series: [], data: [] })).toBe('area');
  });

  it('infers gauge from numeric value + max', () => {
    expect(inferChartType({ title: 'CPU', value: 85, max: 100 })).toBe('gauge');
  });

  it('infers sparkline from array of numbers', () => {
    expect(inferChartType({ data: [1, 2, 3, 4, 5] })).toBe('sparkline');
  });

  it('infers resource-table from columns + rows', () => {
    expect(inferChartType({ title: 'Vols', columns: ['Name'], rows: [['vol1']] })).toBe('resource-table');
  });

  it('infers proposal from command + title', () => {
    expect(inferChartType({ title: 'Create vol', command: 'volume create ...' })).toBe('proposal');
  });

  it('infers action-button from buttons array', () => {
    expect(inferChartType({ buttons: [{ label: 'Go', action: 'do-it' }] })).toBe('action-button');
  });

  it('infers callout from title + body', () => {
    expect(inferChartType({ title: 'Note', body: 'This is important' })).toBe('callout');
  });

  it('infers alert-summary from data with severity counts', () => {
    expect(inferChartType({ data: { critical: 1, warning: 2, info: 0, ok: 5 } })).toBe('alert-summary');
  });

  it('infers alert-list from items with severity + message', () => {
    expect(inferChartType({ items: [{ severity: 'critical', message: 'Down', time: '2026-01-01T00:00:00Z' }] })).toBe('alert-list');
  });

  it('infers alert-list from items with severity + alertname', () => {
    expect(inferChartType({ items: [{ severity: 'critical', alertname: 'InstanceDown', startsAt: '2026-01-01T00:00:00Z' }] })).toBe('alert-list');
  });

  it('infers alert-list from items with severity + description', () => {
    expect(inferChartType({ items: [{ severity: 'warning', description: 'High CPU' }] })).toBe('alert-list');
  });

  it('infers alert-list from items with severity + summary', () => {
    expect(inferChartType({ items: [{ severity: 'critical', summary: 'Disk failing' }] })).toBe('alert-list');
  });

  it('infers alert-list from items with severity + name (no status)', () => {
    expect(inferChartType({ items: [{ severity: 'warning', name: 'VolumeOffline', time: '5 min ago' }] })).toBe('alert-list');
  });

  it('infers status-grid from items with name + status', () => {
    expect(inferChartType({ title: 'Nodes', items: [{ name: 'node1', status: 'ok' }] })).toBe('status-grid');
  });

  it('infers stat from string value + title', () => {
    expect(inferChartType({ title: 'IOPS', value: '12345' })).toBe('stat');
  });

  it('returns null for unknown shapes', () => {
    expect(inferChartType({ name: 'Alice', age: 30 })).toBeNull();
  });

  it('returns null for empty object', () => {
    expect(inferChartType({})).toBeNull();
  });

  it('infers object-detail from kind + name + sections', () => {
    expect(inferChartType({
      kind: 'alert',
      name: 'InstanceDown — node-east-01',
      sections: [{ title: 'Details', layout: 'properties', data: {} }],
    })).toBe('object-detail');
  });

  it('does not infer object-detail when sections is missing', () => {
    expect(inferChartType({ kind: 'alert', name: 'foo' })).toBeNull();
  });

  it('does not misclassify dashboard as object-detail', () => {
    expect(inferChartType({
      title: 'Fleet',
      panels: [{ type: 'stat', title: 'A', value: '1' }],
    })).toBeNull(); // classify() handles dashboards differently
  });
});

// ---------------------------------------------------------------------------
// parseObjectDetail
// ---------------------------------------------------------------------------
describe('parseObjectDetail', () => {
  it('parses a valid object-detail JSON', () => {
    const json = JSON.stringify({
      type: 'object-detail',
      kind: 'alert',
      name: 'InstanceDown — node-east-01',
      status: 'critical',
      subtitle: 'Firing since 2025-06-14 09:32 UTC',
      sections: [
        { title: 'Details', layout: 'properties', data: { columns: 2, items: [] } },
        { title: 'Trend', layout: 'chart', data: { type: 'area', xKey: 'time', series: [], data: [] } },
      ],
    });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('alert');
    expect(result!.name).toBe('InstanceDown — node-east-01');
    expect(result!.status).toBe('critical');
    expect(result!.subtitle).toBe('Firing since 2025-06-14 09:32 UTC');
    expect(result!.sections).toHaveLength(2);
    expect(result!.sections[0].layout).toBe('properties');
    expect(result!.sections[1].layout).toBe('chart');
  });

  it('returns null for invalid JSON', () => {
    expect(parseObjectDetail('not valid json')).toBeNull();
  });

  it('returns null when name is missing', () => {
    const json = JSON.stringify({ kind: 'alert', sections: [] });
    expect(parseObjectDetail(json)).toBeNull();
  });

  it('returns null when sections is not an array', () => {
    const json = JSON.stringify({ kind: 'alert', name: 'Test', sections: 'bad' });
    expect(parseObjectDetail(json)).toBeNull();
  });

  it('defaults kind to "unknown" when missing', () => {
    const json = JSON.stringify({ name: 'Test', sections: [] });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('unknown');
  });

  it('keeps unknown section layouts (forward-compatible)', () => {
    const json = JSON.stringify({
      kind: 'volume',
      name: 'vol1',
      sections: [
        { title: 'Future', layout: 'some-future-layout', data: {} },
        { title: 'Props', layout: 'properties', data: { items: [] } },
      ],
    });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.sections).toHaveLength(2);
    expect(result!.sections[0].layout).toBe('some-future-layout');
  });

  it('skips sections missing title or layout', () => {
    const json = JSON.stringify({
      kind: 'alert',
      name: 'Test',
      sections: [
        { layout: 'properties', data: {} },         // missing title
        { title: 'Good', layout: 'text', data: {} }, // valid
        { title: 'Bad', data: {} },                   // missing layout
      ],
    });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.sections).toHaveLength(1);
    expect(result!.sections[0].title).toBe('Good');
  });

  it('omits status and subtitle when not present', () => {
    const json = JSON.stringify({ kind: 'volume', name: 'vol1', sections: [] });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.status).toBeUndefined();
    expect(result!.subtitle).toBeUndefined();
  });

  it('parseChart does not match object-detail shape', () => {
    const json = JSON.stringify({
      kind: 'alert',
      name: 'InstanceDown',
      sections: [{ title: 'Details', layout: 'properties', data: {} }],
    });
    expect(parseChart(json)).toBeNull();
  });

  it('normalizes alert-list section items with alternative field names', () => {
    const json = JSON.stringify({
      kind: 'volume',
      name: 'vol1',
      sections: [{
        title: 'Alerts',
        layout: 'alert-list',
        data: {
          items: [{ alertname: 'VolumeOffline', severity: 'critical', startsAt: '2026-01-07T10:30:00Z' }],
        },
      }],
    });
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    const alertData = result!.sections[0].data as any;
    expect(alertData.items[0].message).toBe('VolumeOffline');
    expect(alertData.items[0].time).toBe('2026-01-07T10:30:00Z');
  });
});

// ---------------------------------------------------------------------------
// parseChart type inference fallback
// ---------------------------------------------------------------------------
describe('parseChart type inference', () => {
  it('infers alert-list when type field is missing', () => {
    const json = JSON.stringify({ items: [{ severity: 'critical', message: 'InstanceDown', time: '2026-03-07T00:38:00Z' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('alert-list');
  });

  it('infers gauge when type field is missing', () => {
    const json = JSON.stringify({ title: 'CPU', value: 85, max: 100 });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('gauge');
  });

  it('infers bar chart when type field is missing', () => {
    const json = JSON.stringify({ title: 'Capacity', xKey: 'cluster', series: [{ key: 'used', label: 'Used' }], data: [{ cluster: 'c1', used: 10 }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('bar');
  });

  it('still returns null for truly unknown shapes', () => {
    const json = JSON.stringify({ name: 'Alice', age: 30 });
    expect(parseChart(json)).toBeNull();
  });

  // --- Alternative field name normalization ---

  it('infers and normalizes alert-list with alertname field', () => {
    const json = JSON.stringify({ items: [{ alertname: 'InstanceDown', severity: 'critical', startsAt: '2026-01-07T10:30:00Z' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('alert-list');
    const items = (result as any).items;
    expect(items[0].message).toBe('InstanceDown');
    expect(items[0].time).toBe('2026-01-07T10:30:00Z');
  });

  it('infers and normalizes alert-list with description field', () => {
    const json = JSON.stringify({ items: [{ description: 'High CPU usage', severity: 'warning', time: '5 min ago' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('alert-list');
    expect((result as any).items[0].message).toBe('High CPU usage');
  });

  it('infers and normalizes alert-list with summary field', () => {
    const json = JSON.stringify({ items: [{ summary: 'Disk failure predicted', severity: 'critical', time: '2 hr ago' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('alert-list');
    expect((result as any).items[0].message).toBe('Disk failure predicted');
  });

  it('infers and normalizes alert-list with name field', () => {
    const json = JSON.stringify({ items: [{ name: 'VolumeOffline', severity: 'warning', time: '10 min ago' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect(result!.type).toBe('alert-list');
    expect((result as any).items[0].message).toBe('VolumeOffline');
  });

  it('preserves original message when canonical fields exist', () => {
    const json = JSON.stringify({ items: [{ severity: 'info', message: 'Original', time: 'now' }] });
    const result = parseChart(json);
    expect(result).not.toBeNull();
    expect((result as any).items[0].message).toBe('Original');
    expect((result as any).items[0].time).toBe('now');
  });
});

describe('parseDashboard data-point limits', () => {
  it('downsamples panels with >200 data points', () => {
    const json = JSON.stringify({
      title: 'Big Dashboard',
      panels: [
        {
          type: 'area',
          title: 'Heavy',
          xKey: 'time',
          series: [{ key: 'v', label: 'V' }],
          data: Array.from({ length: 300 }, (_, i) => ({ time: i, v: i })),
        },
      ],
    });
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    if (result) {
      const panel = result.panels[0];
      if (panel.type === 'area') {
        expect(panel.data.length).toBeLessThanOrEqual(200);
      }
    }
  });
});
