import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { AreaChartBlock } from './AreaChartBlock';
import type { AreaChartData } from './chartTypes';

const fixture: AreaChartData = {
  type: 'area',
  title: 'Used Capacity — Last 7 Days',
  xKey: 'time',
  yLabel: 'Used %',
  series: [
    { key: 'used_pct', label: 'Used %', color: 'blue' },
  ],
  data: [
    { time: 'Mon', used_pct: 72 },
    { time: 'Tue', used_pct: 74 },
    { time: 'Wed', used_pct: 73 },
    { time: 'Thu', used_pct: 76 },
    { time: 'Fri', used_pct: 78 },
  ],
};

describe('AreaChartBlock', () => {
  it('renders the title', () => {
    render(<AreaChartBlock data={fixture} />);
    expect(screen.getByText('Used Capacity — Last 7 Days')).toBeDefined();
  });

  it('renders without crashing with minimal data', () => {
    const minimal: AreaChartData = {
      type: 'area',
      title: 'Empty',
      xKey: 'x',
      series: [],
      data: [],
    };
    render(<AreaChartBlock data={minimal} />);
    expect(screen.getByText('Empty')).toBeDefined();
    expect(screen.getByText('No data available')).toBeDefined();
  });

  it('renders with multiple series', () => {
    const multi: AreaChartData = {
      ...fixture,
      title: 'Multi-Series',
      series: [
        { key: 'a', label: 'Series A' },
        { key: 'b', label: 'Series B', color: 'red' },
      ],
    };
    render(<AreaChartBlock data={multi} />);
    expect(screen.getByText('Multi-Series')).toBeDefined();
  });

  it('renders without annotations (no regression)', () => {
    render(<AreaChartBlock data={fixture} />);
    expect(screen.getByRole('img', { name: 'Area chart: Used Capacity — Last 7 Days' })).toBeDefined();
  });

  it('renders with one annotation', () => {
    const withAnnotation: AreaChartData = {
      ...fixture,
      title: 'With Threshold',
      annotations: [{ y: 80, label: 'Warning', color: 'red', style: 'dashed' }],
    };
    render(<AreaChartBlock data={withAnnotation} />);
    expect(screen.getByText('With Threshold')).toBeDefined();
  });

  it('renders with multiple annotations', () => {
    const withAnnotations: AreaChartData = {
      ...fixture,
      title: 'Multiple Thresholds',
      annotations: [
        { y: 80, label: 'Warning', color: 'yellow', style: 'dashed' },
        { y: 95, label: 'Critical', color: 'red', style: 'dashed' },
      ],
    };
    render(<AreaChartBlock data={withAnnotations} />);
    expect(screen.getByText('Multiple Thresholds')).toBeDefined();
  });
});
