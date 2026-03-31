import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { GaugeBlock } from './GaugeBlock';
import type { GaugeData } from './chartTypes';

describe('GaugeBlock', () => {
  it('renders title and value', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Aggregate Used',
      value: 82,
      max: 100,
      unit: '%',
    };
    render(<GaugeBlock data={data} />);
    expect(screen.getByText('Aggregate Used')).toBeDefined();
    expect(screen.getByText('82%')).toBeDefined();
    expect(screen.getByText('82 / 100 %')).toBeDefined();
  });

  it('renders without unit', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Disk Count',
      value: 24,
      max: 48,
    };
    render(<GaugeBlock data={data} />);
    expect(screen.getByText('24')).toBeDefined();
    expect(screen.getByText('24 / 48')).toBeDefined();
  });

  it('renders with thresholds (critical)', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Critical Usage',
      value: 97,
      max: 100,
      unit: '%',
      thresholds: { warning: 80, critical: 95 },
    };
    render(<GaugeBlock data={data} />);
    expect(screen.getByText('Critical Usage')).toBeDefined();
  });

  it('renders with thresholds (warning)', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Warning Usage',
      value: 85,
      max: 100,
      thresholds: { warning: 80, critical: 95 },
    };
    render(<GaugeBlock data={data} />);
    expect(screen.getByText('Warning Usage')).toBeDefined();
  });

  it('handles zero max gracefully', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Zero Max',
      value: 0,
      max: 0,
    };
    render(<GaugeBlock data={data} />);
    expect(screen.getByText('Zero Max')).toBeDefined();
  });
});
