import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { BarChartBlock } from './BarChartBlock';
import type { BarChartData } from './chartTypes';

const fixture: BarChartData = {
  type: 'bar',
  title: 'IOPS by Volume',
  xKey: 'volume',
  series: [
    { key: 'read', label: 'Read IOPS', color: 'blue' },
    { key: 'write', label: 'Write IOPS', color: 'violet' },
  ],
  data: [
    { volume: 'vol01', read: 1200, write: 800 },
    { volume: 'vol02', read: 900, write: 600 },
    { volume: 'vol03', read: 1500, write: 1100 },
  ],
};

describe('BarChartBlock', () => {
  it('renders the title', () => {
    render(<BarChartBlock data={fixture} />);
    expect(screen.getByText('IOPS by Volume')).toBeDefined();
  });

  it('renders with empty data', () => {
    const empty: BarChartData = {
      type: 'bar',
      title: 'No Data',
      xKey: 'x',
      series: [],
      data: [],
    };
    render(<BarChartBlock data={empty} />);
    expect(screen.getByText('No Data')).toBeDefined();
    expect(screen.getByText('No data available')).toBeDefined();
  });
});
