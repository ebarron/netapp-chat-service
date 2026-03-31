import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { StatBlock } from './StatBlock';
import type { StatData } from './chartTypes';

describe('StatBlock', () => {
  it('renders title and value', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Available Capacity',
      value: '1.2 TB',
    };
    render(<StatBlock data={data} />);
    expect(screen.getByText('Available Capacity')).toBeDefined();
    expect(screen.getByText('1.2 TB')).toBeDefined();
  });

  it('renders subtitle', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Throughput',
      value: '450 MB/s',
      subtitle: 'Average over last hour',
    };
    render(<StatBlock data={data} />);
    expect(screen.getByText('Average over last hour')).toBeDefined();
  });

  it('renders upward trend', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Growth',
      value: '3.5 TB',
      trend: 'up',
      trendValue: '+12%',
    };
    render(<StatBlock data={data} />);
    expect(screen.getByText('+12%')).toBeDefined();
  });

  it('renders downward trend', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Latency',
      value: '2.1 ms',
      trend: 'down',
      trendValue: '-8%',
    };
    render(<StatBlock data={data} />);
    expect(screen.getByText('-8%')).toBeDefined();
  });

  it('renders flat trend without trendValue', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Steady',
      value: '100',
      trend: 'flat',
    };
    render(<StatBlock data={data} />);
    expect(screen.getByText('Steady')).toBeDefined();
    expect(screen.getByText('100')).toBeDefined();
  });
});
