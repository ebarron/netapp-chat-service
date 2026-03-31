import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { CalloutBlock } from './CalloutBlock';
import type { CalloutData } from './chartTypes';

describe('CalloutBlock', () => {
  it('renders title and body', () => {
    const data: CalloutData = {
      type: 'callout',
      title: 'Recommendation',
      body: 'Consider expanding aggr1 before it reaches 90%.',
    };
    render(<CalloutBlock data={data} />);
    expect(screen.getByText('Recommendation')).toBeDefined();
    expect(screen.getByText('Consider expanding aggr1 before it reaches 90%.')).toBeDefined();
  });

  it('renders with an icon prefix', () => {
    const data: CalloutData = {
      type: 'callout',
      icon: '💡',
      title: 'Tip',
      body: 'Use dedup to save space.',
    };
    render(<CalloutBlock data={data} />);
    expect(screen.getByText('💡 Tip')).toBeDefined();
  });

  it('renders without icon', () => {
    const data: CalloutData = {
      type: 'callout',
      title: 'Note',
      body: 'All systems operational.',
    };
    render(<CalloutBlock data={data} />);
    expect(screen.getByText('Note')).toBeDefined();
    expect(screen.getByText('All systems operational.')).toBeDefined();
  });

  it('renders markdown in body (tables, bold, lists)', () => {
    const data: CalloutData = {
      type: 'callout',
      title: 'Recommendation',
      body: `For your **100GB NFS volume**, I recommend **NVME-a250**:

| Candidate | Type | Free |
|-----------|------|------|
| NVME-a250 | NVMe | 87% |
| a700s | SAS | 80% |

**Why NVME-a250?**

- NVMe storage — lowest latency
- 87% free capacity`,
    };
    render(<CalloutBlock data={data} />);

    // Bold should render as <strong>, not raw **
    expect(screen.queryByText('**100GB NFS volume**')).toBeNull();
    expect(screen.getByText(/100GB NFS volume/)).toBeDefined();

    // Table cells should render
    expect(screen.getAllByText('NVME-a250').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('87%')).toBeDefined();

    // List items should render
    expect(screen.getByText(/lowest latency/)).toBeDefined();
    expect(screen.getByText(/87% free capacity/)).toBeDefined();
  });
});
