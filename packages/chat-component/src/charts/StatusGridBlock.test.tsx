import { screen } from '@testing-library/react';
import { render } from '../../test-utils';
import { StatusGridBlock } from './StatusGridBlock';
import type { StatusGridData } from './chartTypes';
import { describe, it, expect } from 'vitest';

describe('StatusGridBlock', () => {
  const sampleData: StatusGridData = {
    type: 'status-grid',
    title: 'Cluster Health',
    items: [
      { name: 'Node-01', status: 'ok', detail: 'All good' },
      { name: 'Node-02', status: 'warning', detail: 'High CPU' },
      { name: 'Node-03', status: 'critical', detail: 'Disk full' },
      { name: 'Node-04', status: 'unknown' },
    ],
  };

  it('renders the title', () => {
    render(<StatusGridBlock data={sampleData} />);
    expect(screen.getByText('Cluster Health')).toBeInTheDocument();
  });

  it('renders all item names', () => {
    render(<StatusGridBlock data={sampleData} />);
    expect(screen.getByText('Node-01')).toBeInTheDocument();
    expect(screen.getByText('Node-02')).toBeInTheDocument();
    expect(screen.getByText('Node-03')).toBeInTheDocument();
    expect(screen.getByText('Node-04')).toBeInTheDocument();
  });

  it('renders detail text when present', () => {
    render(<StatusGridBlock data={sampleData} />);
    expect(screen.getByText('All good')).toBeInTheDocument();
    expect(screen.getByText('High CPU')).toBeInTheDocument();
    expect(screen.getByText('Disk full')).toBeInTheDocument();
  });

  it('renders without detail text', () => {
    const data: StatusGridData = {
      type: 'status-grid',
      title: 'Simple Grid',
      items: [{ name: 'Item-A', status: 'ok' }],
    };
    render(<StatusGridBlock data={data} />);
    expect(screen.getByText('Item-A')).toBeInTheDocument();
  });

  it('renders empty items list without crashing', () => {
    const data: StatusGridData = {
      type: 'status-grid',
      title: 'Empty Grid',
      items: [],
    };
    render(<StatusGridBlock data={data} />);
    expect(screen.getByText('Empty Grid')).toBeInTheDocument();
  });
});
