import { render, screen, fireEvent } from '../test-utils';
import { describe, it, expect } from 'vitest';
import { ToolStatusCard, detectToolViz } from './ToolStatusCard';
import type { ChatMessage } from './useChatPanel';

// ---------------------------------------------------------------------------
// detectToolViz — unit tests for auto-detection heuristics
// ---------------------------------------------------------------------------
describe('detectToolViz', () => {
  it('returns null for undefined input', () => {
    expect(detectToolViz(undefined)).toBeNull();
  });

  it('returns null for non-JSON string', () => {
    expect(detectToolViz('just some text output')).toBeNull();
  });

  it('returns null for a plain array of numbers', () => {
    expect(detectToolViz('[1, 2, 3]')).toBeNull();
  });

  it('returns null for a single-element array', () => {
    expect(detectToolViz(JSON.stringify([{ time: '2024-01-01', value: 10 }]))).toBeNull();
  });

  // --- Gauge detection ---

  it('detects a gauge from { value, max }', () => {
    const input = JSON.stringify({ value: 75, max: 100 });
    const result = detectToolViz(input);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('gauge');
    if (result!.kind === 'gauge') {
      expect(result!.data.value).toBe(75);
      expect(result!.data.max).toBe(100);
    }
  });

  it('detects a gauge with title, unit and thresholds', () => {
    const input = JSON.stringify({
      title: 'CPU Usage',
      value: 85,
      max: 100,
      unit: '%',
      thresholds: { warning: 70, critical: 90 },
    });
    const result = detectToolViz(input);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('gauge');
    if (result!.kind === 'gauge') {
      expect(result!.data.title).toBe('CPU Usage');
      expect(result!.data.unit).toBe('%');
      expect(result!.data.thresholds).toEqual({ warning: 70, critical: 90 });
    }
  });

  it('returns null for object without max', () => {
    expect(detectToolViz(JSON.stringify({ value: 42 }))).toBeNull();
  });

  it('returns null for object with non-numeric value', () => {
    expect(detectToolViz(JSON.stringify({ value: 'high', max: 100 }))).toBeNull();
  });

  // --- Sparkline detection ---

  it('detects a sparkline from time-series array', () => {
    const data = [
      { timestamp: '2024-01-01T00:00:00Z', iops: 100 },
      { timestamp: '2024-01-01T01:00:00Z', iops: 150 },
      { timestamp: '2024-01-01T02:00:00Z', iops: 120 },
    ];
    const result = detectToolViz(JSON.stringify(data));
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('sparkline');
    if (result!.kind === 'sparkline') {
      expect(result!.data.data).toEqual([100, 150, 120]);
      expect(result!.data.title).toBe('iops');
    }
  });

  it('detects sparkline with "time" key', () => {
    const data = [
      { time: 't1', latency: 5 },
      { time: 't2', latency: 8 },
    ];
    const result = detectToolViz(JSON.stringify(data));
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('sparkline');
  });

  it('detects sparkline with "date" key', () => {
    const data = [
      { date: '2024-01-01', throughput: 200 },
      { date: '2024-01-02', throughput: 250 },
    ];
    const result = detectToolViz(JSON.stringify(data));
    expect(result!.kind).toBe('sparkline');
  });

  it('detects sparkline with "collected_at" key', () => {
    const data = [
      { collected_at: '2024-01-01', ops: 42 },
      { collected_at: '2024-01-02', ops: 55 },
    ];
    const result = detectToolViz(JSON.stringify(data));
    expect(result!.kind).toBe('sparkline');
  });

  it('returns null for array of objects without timestamp key', () => {
    const data = [
      { name: 'vol1', used: 100 },
      { name: 'vol2', used: 200 },
    ];
    expect(detectToolViz(JSON.stringify(data))).toBeNull();
  });

  it('returns null for array of objects with timestamp but no numeric field', () => {
    const data = [
      { timestamp: 't1', status: 'ok' },
      { timestamp: 't2', status: 'error' },
    ];
    expect(detectToolViz(JSON.stringify(data))).toBeNull();
  });

  it('returns null for nested arrays', () => {
    expect(detectToolViz(JSON.stringify([[1, 2], [3, 4]]))).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// ToolStatusCard — component rendering tests
// ---------------------------------------------------------------------------
describe('ToolStatusCard', () => {
  function makeMsg(overrides: Partial<ChatMessage> = {}): ChatMessage {
    return {
      id: 'msg-1',
      role: 'tool',
      content: '',
      toolName: 'metrics_query',
      toolStatus: 'completed',
      toolResult: 'some result text',
      capability: 'harvest',
      ...overrides,
    };
  }

  it('renders tool name, status badge, and capability', () => {
    render(<ToolStatusCard message={makeMsg()} />);
    expect(screen.getByText('metrics_query')).toBeDefined();
    expect(screen.getByText('completed')).toBeDefined();
    expect(screen.getByText('harvest')).toBeDefined();
  });

  it('renders plain text when no visualization detected', () => {
    render(<ToolStatusCard message={makeMsg({ toolResult: 'plain output' })} />);
    expect(screen.getByText('plain output')).toBeDefined();
  });

  it('renders sparkline when time-series data detected', () => {
    const data = [
      { timestamp: '2024-01-01', iops: 100 },
      { timestamp: '2024-01-02', iops: 200 },
      { timestamp: '2024-01-03', iops: 150 },
    ];
    const { container } = render(
      <ToolStatusCard message={makeMsg({ toolResult: JSON.stringify(data) })} />,
    );
    // SparklineBlock renders a Paper with the title
    expect(screen.getByText('iops')).toBeDefined();
    // Should have a toggle button
    expect(screen.getByLabelText('Show raw')).toBeDefined();
    // Should NOT show raw text by default
    expect(container.textContent).not.toContain('"timestamp"');
  });

  it('renders gauge when value/max data detected', () => {
    const data = { title: 'Capacity', value: 80, max: 100, unit: '%' };
    render(
      <ToolStatusCard message={makeMsg({ toolResult: JSON.stringify(data) })} />,
    );
    expect(screen.getByText('Capacity')).toBeDefined();
    expect(screen.getByLabelText('Show raw')).toBeDefined();
  });

  it('does not auto-detect while executing', () => {
    const data = { value: 80, max: 100 };
    render(
      <ToolStatusCard
        message={makeMsg({ toolStatus: 'executing', toolResult: JSON.stringify(data) })}
      />,
    );
    expect(screen.queryByLabelText('Show raw')).toBeNull();
  });

  it('does not auto-detect when status is failed', () => {
    const data = { value: 80, max: 100 };
    render(
      <ToolStatusCard
        message={makeMsg({ toolStatus: 'failed', toolResult: JSON.stringify(data) })}
      />,
    );
    expect(screen.queryByLabelText('Show raw')).toBeNull();
  });

  // --- Expand/Collapse Toggle ---

  it('toggles between chart and raw text', () => {
    const tsData = [
      { timestamp: 't1', val: 10 },
      { timestamp: 't2', val: 20 },
    ];
    const raw = JSON.stringify(tsData);
    const { container } = render(
      <ToolStatusCard message={makeMsg({ toolResult: raw })} />,
    );

    // Initially shows chart, not raw
    expect(screen.getByText('val')).toBeDefined(); // sparkline title
    expect(container.textContent).not.toContain('"timestamp"');

    // Click "Show raw" toggle
    fireEvent.click(screen.getByLabelText('Show raw'));

    // Now shows raw text
    expect(screen.getByText(raw)).toBeDefined();
    expect(screen.getByLabelText('Show chart')).toBeDefined();

    // Click "Show chart" toggle to go back
    fireEvent.click(screen.getByLabelText('Show chart'));
    expect(screen.getByText('val')).toBeDefined();
  });

  it('has no toggle when no viz is detected', () => {
    render(<ToolStatusCard message={makeMsg({ toolResult: 'plain text' })} />);
    expect(screen.queryByLabelText('Show raw')).toBeNull();
    expect(screen.queryByLabelText('Show chart')).toBeNull();
  });

  it('renders without capability badge when not set', () => {
    render(<ToolStatusCard message={makeMsg({ capability: undefined })} />);
    expect(screen.queryByText('harvest')).toBeNull();
    expect(screen.getByText('metrics_query')).toBeDefined();
  });

  it('renders status colors correctly for each state', () => {
    const { rerender } = render(
      <ToolStatusCard message={makeMsg({ toolStatus: 'executing' })} />,
    );
    expect(screen.getByText('executing')).toBeDefined();

    rerender(<ToolStatusCard message={makeMsg({ toolStatus: 'failed' })} />);
    expect(screen.getByText('failed')).toBeDefined();
  });
});
