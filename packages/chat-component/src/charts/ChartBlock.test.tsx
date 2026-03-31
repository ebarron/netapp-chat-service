import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { ChartBlock } from './ChartBlock';

describe('ChartBlock', () => {
  it('renders an area chart', () => {
    const json = JSON.stringify({
      type: 'area',
      title: 'Capacity Trend',
      xKey: 'time',
      series: [{ key: 'used', label: 'Used' }],
      data: [{ time: 'Mon', used: 72 }],
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('Capacity Trend')).toBeDefined();
  });

  it('renders a bar chart', () => {
    const json = JSON.stringify({
      type: 'bar',
      title: 'IOPS Compare',
      xKey: 'vol',
      series: [{ key: 'iops', label: 'IOPS' }],
      data: [{ vol: 'v1', iops: 500 }],
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('IOPS Compare')).toBeDefined();
  });

  it('renders a stat', () => {
    const json = JSON.stringify({
      type: 'stat',
      title: 'Total Capacity',
      value: '10 TB',
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('Total Capacity')).toBeDefined();
    expect(screen.getByText('10 TB')).toBeDefined();
  });

  it('renders a gauge', () => {
    const json = JSON.stringify({
      type: 'gauge',
      title: 'Disk Usage',
      value: 75,
      max: 100,
      unit: '%',
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('Disk Usage')).toBeDefined();
  });

  it('renders a callout', () => {
    const json = JSON.stringify({
      type: 'callout',
      title: 'Note',
      body: 'Everything is fine.',
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('Note')).toBeDefined();
    expect(screen.getByText('Everything is fine.')).toBeDefined();
  });

  it('renders a proposal', () => {
    const json = JSON.stringify({
      type: 'proposal',
      title: 'CLI Command',
      command: 'vol show -instance',
    });
    render(<ChartBlock json={json} />);
    expect(screen.getByText('CLI Command')).toBeDefined();
  });

  it('renders alert-summary with onAction', () => {
    const onAction = vi.fn();
    const json = JSON.stringify({
      type: 'alert-summary',
      data: { critical: 1, warning: 2, info: 0, ok: 10 },
    });
    render(<ChartBlock json={json} onAction={onAction} />);
    expect(screen.getByText('critical: 1')).toBeDefined();
    screen.getByText('critical: 1').click();
    expect(onAction).toHaveBeenCalledWith('Show me the critical alerts');
  });

  it('renders action-button with callbacks', () => {
    const onAction = vi.fn();
    const onExecute = vi.fn();
    const json = JSON.stringify({
      type: 'action-button',
      buttons: [
        { label: 'Go', action: 'message', message: 'go now' },
        { label: 'Run', action: 'execute', tool: 'tool1' },
      ],
    });
    render(<ChartBlock json={json} onAction={onAction} onExecute={onExecute} />);
    screen.getByText('Go').click();
    expect(onAction).toHaveBeenCalledWith('go now');
    screen.getByText('Run').click();
    expect(onExecute).toHaveBeenCalledWith('tool1', undefined);
  });

  it('falls back to code block for invalid JSON', () => {
    render(<ChartBlock json="not valid json {{{" />);
    expect(screen.getByText('not valid json {{{')).toBeDefined();
  });

  it('falls back to code block for unknown type', () => {
    const json = JSON.stringify({ type: 'pie', data: [] });
    render(<ChartBlock json={json} />);
    // Should render as raw code
    expect(screen.getByText(json)).toBeDefined();
  });
});
