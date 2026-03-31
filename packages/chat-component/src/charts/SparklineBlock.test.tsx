import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { SparklineBlock } from './SparklineBlock';
import type { SparklineData } from './chartTypes';

describe('SparklineBlock', () => {
  it('renders with a title', () => {
    const data: SparklineData = {
      type: 'sparkline',
      title: 'Latency Trend',
      data: [1, 3, 2, 5, 4, 6],
      color: 'teal',
    };
    render(<SparklineBlock data={data} />);
    expect(screen.getByText('Latency Trend')).toBeDefined();
  });

  it('renders without a title', () => {
    const data: SparklineData = {
      type: 'sparkline',
      data: [10, 20, 30],
    };
    const { container } = render(<SparklineBlock data={data} />);
    // No title rendered, but the component still mounts
    expect(container.querySelector('.mantine-Paper-root')).toBeDefined();
  });

  it('renders with empty data array', () => {
    const data: SparklineData = {
      type: 'sparkline',
      title: 'Empty',
      data: [],
    };
    render(<SparklineBlock data={data} />);
    expect(screen.getByText('Empty')).toBeDefined();
    expect(screen.getByText('No data available')).toBeDefined();
  });
});
