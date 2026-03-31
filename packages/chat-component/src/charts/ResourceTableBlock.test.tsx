import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { ResourceTableBlock } from './ResourceTableBlock';
import type { ResourceTableData } from './chartTypes';

const fixture: ResourceTableData = {
  type: 'resource-table',
  title: 'Volume Status',
  columns: ['Name', 'Used', 'Status'],
  rows: [
    { name: 'vol_data01', Name: 'vol_data01', Used: '72%', Status: 'OK' },
    { name: 'vol_data02', Name: 'vol_data02', Used: '91%', Status: 'Warning' },
  ],
};

describe('ResourceTableBlock', () => {
  it('renders title and column headers', () => {
    render(<ResourceTableBlock data={fixture} />);
    expect(screen.getByText('Volume Status')).toBeDefined();
    expect(screen.getByText('Name')).toBeDefined();
    expect(screen.getByText('Used')).toBeDefined();
    expect(screen.getByText('Status')).toBeDefined();
  });

  it('renders row data', () => {
    render(<ResourceTableBlock data={fixture} />);
    expect(screen.getByText('vol_data01')).toBeDefined();
    expect(screen.getByText('91%')).toBeDefined();
    expect(screen.getByText('Warning')).toBeDefined();
  });

  it('calls onAction with inferred kind when a row is clicked', () => {
    const onAction = vi.fn();
    render(<ResourceTableBlock data={fixture} onAction={onAction} />);
    screen.getByText('vol_data02').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about vol_data02');
  });

  it('includes cluster and svm in action when present in row data', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Top Volumes',
      columns: ['Volume', 'Used %', 'Status'],
      rows: [
        { name: 'vdb_tools', Volume: 'vdb_tools', 'Used %': '33%', Status: 'ok', cluster: 'cls1', svm: 'svm1' },
      ],
    };
    const onAction = vi.fn();
    render(<ResourceTableBlock data={data} onAction={onAction} />);
    screen.getByText('vdb_tools').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about volume vdb_tools on SVM svm1 on cluster cls1');
  });

  it('renders empty table gracefully', () => {
    const empty: ResourceTableData = {
      type: 'resource-table',
      title: 'Empty Table',
      columns: ['Name'],
      rows: [],
    };
    render(<ResourceTableBlock data={empty} />);
    expect(screen.getByText('Empty Table')).toBeDefined();
  });

  it('resolves "Resource" column from name alias', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Top Volumes',
      columns: ['Resource', 'Key Metric', 'Status', 'Alerts'],
      rows: [
        { name: 'vol_prod_db01', metric: '33%', status: 'ok', alerts: 0 },
        { name: 'vol_backup', metric: '12%', status: 'ok', alerts: 0 },
      ],
    };
    render(<ResourceTableBlock data={data} />);
    expect(screen.getByText('vol_prod_db01')).toBeDefined();
    expect(screen.getByText('vol_backup')).toBeDefined();
    expect(screen.getByText('33%')).toBeDefined();
    expect(screen.getByText('12%')).toBeDefined();
  });

  it('resolves "Volume" and "Used %" columns from aliases', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Top Utilized Volumes',
      columns: ['Volume', 'Used %', 'Status', 'Alerts'],
      rows: [
        { name: 'vdb_tools', metric: '33%', status: 'ok', alerts: 0 },
        { name: 'vdb_s5_04', metric: '8%', status: 'ok', alerts: 0 },
      ],
    };
    render(<ResourceTableBlock data={data} />);
    expect(screen.getByText('vdb_tools')).toBeDefined();
    expect(screen.getByText('vdb_s5_04')).toBeDefined();
    expect(screen.getByText('33%')).toBeDefined();
  });

  it('renders inline sparklines for array cell values', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Capacity',
      columns: ['Cluster', 'Used', 'Trend'],
      rows: [
        { name: 'cls1', Cluster: 'cls1', Used: '67%', Trend: [60, 62, 64, 65, 67] as unknown as string },
        { name: 'cls2', Cluster: 'cls2', Used: '55%', Trend: [50, 52, 53, 54, 55] as unknown as string },
      ],
    };
    const { container } = render(<ResourceTableBlock data={data} />);
    // MiniSparkline renders real SVGs with role="img" — jsdom handles these fine
    const sparklines = container.querySelectorAll('td svg[role="img"]');
    expect(sparklines.length).toBe(2);
    // Text values should still render normally
    expect(screen.getByText('67%')).toBeDefined();
    expect(screen.getByText('55%')).toBeDefined();
  });

  it('renders per-column inline sparklines via {col}_trend hidden fields', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Top Volumes',
      columns: ['Volume', 'Used %', 'IOPS'],
      rows: [
        { name: 'vol1', Volume: 'vol1', 'Used %': '87.3%', IOPS: '1200', used_trend: [80, 83, 85, 87, 87] as unknown as string, iops_trend: [1000, 1050, 1100, 1150, 1200] as unknown as string },
        { name: 'vol2', Volume: 'vol2', 'Used %': '76.4%', IOPS: '900', used_trend: [70, 72, 74, 75, 76] as unknown as string, iops_trend: [800, 830, 860, 880, 900] as unknown as string },
      ],
    };
    const { container } = render(<ResourceTableBlock data={data} />);
    // 2 rows × 2 inline sparklines each (Used % + IOPS) = 4
    const sparklines = container.querySelectorAll('td svg[role="img"]');
    expect(sparklines.length).toBe(4);
    expect(screen.getByText('87.3%')).toBeDefined();
    expect(screen.getByText('1200')).toBeDefined();
  });

  it('falls back to generic trend on first % column', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Volumes',
      columns: ['Volume', 'Used %', 'Status'],
      rows: [
        { name: 'v1', Volume: 'v1', 'Used %': '80%', Status: 'OK', trend: [75, 76, 78, 79, 80] as unknown as string },
      ],
    };
    const { container } = render(<ResourceTableBlock data={data} />);
    const sparklines = container.querySelectorAll('td svg[role="img"]');
    expect(sparklines.length).toBe(1);
  });
});
