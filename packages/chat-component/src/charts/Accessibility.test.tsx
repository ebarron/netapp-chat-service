import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { AreaChartBlock } from './AreaChartBlock';
import { BarChartBlock } from './BarChartBlock';
import { SparklineBlock } from './SparklineBlock';
import { GaugeBlock } from './GaugeBlock';
import { StatBlock } from './StatBlock';
import { StatusGridBlock } from './StatusGridBlock';
import { AlertSummaryBlock } from './AlertSummaryBlock';
import { AlertListBlock } from './AlertListBlock';
import { CalloutBlock } from './CalloutBlock';
import { ProposalBlock } from './ProposalBlock';
import { ActionButtonBlock } from './ActionButtonBlock';
import { ResourceTableBlock } from './ResourceTableBlock';
import type {
  AreaChartData,
  BarChartData,
  SparklineData,
  GaugeData,
  StatData,
  StatusGridData,
  AlertSummaryData,
  AlertListData,
  CalloutData,
  ProposalData,
  ActionButtonData,
  ResourceTableData,
} from './chartTypes';

describe('Accessibility — ARIA attributes', () => {
  it('AreaChartBlock has role=img and aria-label', () => {
    const data: AreaChartData = {
      type: 'area',
      title: 'Throughput',
      xKey: 'time',
      series: [{ key: 'iops', label: 'IOPS' }],
      data: [{ time: '1', iops: 10 }],
    };
    render(<AreaChartBlock data={data} />);
    const el = screen.getByRole('img', { name: 'Area chart: Throughput' });
    expect(el).toBeDefined();
  });

  it('BarChartBlock has role=img and aria-label', () => {
    const data: BarChartData = {
      type: 'bar',
      title: 'Node Comparison',
      xKey: 'node',
      series: [{ key: 'value', label: 'Value' }],
      data: [{ node: 'A', value: 10 }],
    };
    render(<BarChartBlock data={data} />);
    const el = screen.getByRole('img', { name: 'Bar chart: Node Comparison' });
    expect(el).toBeDefined();
  });

  it('SparklineBlock has role=img and aria-label', () => {
    const data: SparklineData = {
      type: 'sparkline',
      title: 'CPU Trend',
      data: [10, 20, 30],
    };
    render(<SparklineBlock data={data} />);
    const el = screen.getByRole('img', { name: 'Sparkline: CPU Trend' });
    expect(el).toBeDefined();
  });

  it('SparklineBlock without title uses fallback aria-label', () => {
    const data: SparklineData = {
      type: 'sparkline',
      data: [10, 20, 30],
    };
    render(<SparklineBlock data={data} />);
    const el = screen.getByRole('img', { name: 'Sparkline: trend' });
    expect(el).toBeDefined();
  });

  it('GaugeBlock has role=meter with aria-value attributes', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Disk Used',
      value: 75,
      max: 100,
      unit: '%',
    };
    render(<GaugeBlock data={data} />);
    const el = screen.getByRole('meter');
    expect(el).toBeDefined();
    expect(el.getAttribute('aria-valuenow')).toBe('75');
    expect(el.getAttribute('aria-valuemin')).toBe('0');
    expect(el.getAttribute('aria-valuemax')).toBe('100');
    expect(el.getAttribute('aria-label')).toBe('Disk Used: 75% of 100%');
  });

  it('GaugeBlock without unit omits unit in aria-label', () => {
    const data: GaugeData = {
      type: 'gauge',
      title: 'Nodes',
      value: 3,
      max: 4,
    };
    render(<GaugeBlock data={data} />);
    const el = screen.getByRole('meter');
    expect(el.getAttribute('aria-label')).toBe('Nodes: 3 of 4');
  });

  it('StatBlock has aria-label with title and value', () => {
    const data: StatData = {
      type: 'stat',
      title: 'Clusters Online',
      value: '4 / 4',
    };
    render(<StatBlock data={data} />);
    const el = screen.getByLabelText('Clusters Online: 4 / 4');
    expect(el).toBeDefined();
  });

  it('StatusGridBlock items have role=listitem with aria-labels', () => {
    const data: StatusGridData = {
      type: 'status-grid',
      title: 'Volume Health',
      items: [
        { name: 'vol1', status: 'ok' },
        { name: 'vol2', status: 'critical', detail: 'Full' },
      ],
    };
    render(<StatusGridBlock data={data} />);
    const list = screen.getByRole('list', { name: 'Status items' });
    expect(list).toBeDefined();
    const items = screen.getAllByRole('listitem');
    expect(items.length).toBe(2);
    expect(items[0].getAttribute('aria-label')).toBe('vol1: ok');
    expect(items[1].getAttribute('aria-label')).toBe('vol2: critical');
  });

  it('AlertSummaryBlock badges have aria-labels', () => {
    const data: AlertSummaryData = {
      type: 'alert-summary',
      title: 'Alerts',
      data: { critical: 2, warning: 5 },
    };
    render(<AlertSummaryBlock data={data} />);
    expect(screen.getByLabelText('critical: 2 alerts')).toBeDefined();
    expect(screen.getByLabelText('warning: 5 alerts')).toBeDefined();
  });

  it('AlertSummaryBlock singular alert label', () => {
    const data: AlertSummaryData = {
      type: 'alert-summary',
      title: 'Alerts',
      data: { critical: 1 },
    };
    render(<AlertSummaryBlock data={data} />);
    expect(screen.getByLabelText('critical: 1 alert')).toBeDefined();
  });

  it('AlertListBlock has role=list with aria-labeled items', () => {
    const data: AlertListData = {
      type: 'alert-list',
      title: 'Recent Alerts',
      items: [
        { severity: 'critical', message: 'Disk full', time: '10:00' },
        { severity: 'warning', message: 'High CPU', time: '10:05' },
      ],
    };
    render(<AlertListBlock data={data} />);
    const list = screen.getByRole('list', { name: 'Alerts' });
    expect(list).toBeDefined();
    const items = screen.getAllByRole('listitem');
    expect(items.length).toBe(2);
    expect(items[0].getAttribute('aria-label')).toBe('critical: Disk full');
  });

  it('CalloutBlock has role=note', () => {
    const data: CalloutData = {
      type: 'callout',
      title: 'Important',
      body: 'Check your volumes.',
    };
    render(<CalloutBlock data={data} />);
    const el = screen.getByRole('note', { name: 'Important' });
    expect(el).toBeDefined();
  });

  it('ProposalBlock has role=note with aria-label', () => {
    const data: ProposalData = {
      type: 'proposal',
      title: 'Create Volume',
      command: 'vol create -vserver svm1 -volume data01 -size 2TB',
    };
    render(<ProposalBlock data={data} />);
    const el = screen.getByRole('note', { name: 'Proposal: Create Volume' });
    expect(el).toBeDefined();
  });

  it('ActionButtonBlock has role=group', () => {
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Approve', action: 'message', message: 'yes' },
        { label: 'Deny', action: 'message', message: 'no' },
      ],
    };
    render(<ActionButtonBlock data={data} />);
    const group = screen.getByRole('group', { name: 'Actions' });
    expect(group).toBeDefined();
  });

  it('ResourceTableBlock table has aria-label', () => {
    const data: ResourceTableData = {
      type: 'resource-table',
      title: 'Volume List',
      columns: ['Name', 'Size'],
      rows: [{ name: 'vol1', Name: 'vol1', Size: '1TB' }],
    };
    render(<ResourceTableBlock data={data} />);
    const table = screen.getByRole('table', { name: 'Volume List' });
    expect(table).toBeDefined();
  });
});
