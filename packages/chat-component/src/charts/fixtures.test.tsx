/**
 * Fixture validation tests — ensures the three lighthouse interest fixtures
 * parse correctly and render without errors. These serve as regression baselines.
 * Ref: spec §11.1 Task 1.8
 */
import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { DashboardBlock } from './DashboardBlock';
import { ObjectDetailBlock } from './ObjectDetailBlock';
import { parseDashboard, parseObjectDetail } from './chartTypes';

import {
  morningCoffee,
  morningCoffeeV2,
  resourceStatus,
  volumeProvision,
  alertDetail,
  volumeDetail,
  clusterDetail,
  volumeList,
} from './fixtures';

describe('Lighthouse fixture: morning-coffee', () => {
  const json = JSON.stringify(morningCoffee);

  it('parses as valid DashboardData', () => {
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Good Morning — Fleet Overview');
    expect(result!.panels.length).toBe(7);
  });

  it('renders the full dashboard', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Good Morning — Fleet Overview')).toBeDefined();
    expect(screen.getByText('Alert Counts')).toBeDefined();
    expect(screen.getByText('Clusters Online')).toBeDefined();
    expect(screen.getByText('3 / 3')).toBeDefined();
    expect(screen.getByText('Aggregate Usage — Last 7 Days')).toBeDefined();
    expect(screen.getByText('Volumes Needing Attention')).toBeDefined();
    expect(screen.getByText('vol_logs')).toBeDefined();
    expect(screen.getByText('Recent Alerts')).toBeDefined();
    expect(screen.getByText('💡 Recommendation')).toBeDefined();
    expect(screen.getByText('Show all alerts')).toBeDefined();
  });

  it('propagates click actions', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    screen.getByText('critical: 1').click();
    expect(onAction).toHaveBeenCalledWith('Show me the critical alerts');
    screen.getByText('vol_logs').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about vol_logs on cluster cluster-east');
    screen.getByText('Check capacities').click();
    expect(onAction).toHaveBeenCalledWith('Show all volumes over 80% capacity');
  });
});

describe('Lighthouse fixture: resource-status', () => {
  const json = JSON.stringify(resourceStatus);

  it('parses as valid DashboardData', () => {
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Resource Status — vol_prod_db01');
    expect(result!.panels.length).toBe(8);
  });

  it('renders the full dashboard', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Resource Status — vol_prod_db01')).toBeDefined();
    expect(screen.getByText('Volume State')).toBeDefined();
    expect(screen.getByText('Online')).toBeDefined();
    expect(screen.getByText('Capacity Used')).toBeDefined();
    expect(screen.getByText('78%')).toBeDefined();
    expect(screen.getByText('Avg Latency')).toBeDefined();
    expect(screen.getByText('1.8 ms')).toBeDefined();
    expect(screen.getByText('-12%')).toBeDefined();
    expect(screen.getByText('IOPS — Last 24 Hours')).toBeDefined();
    expect(screen.getByText('Throughput — Last 24 Hours')).toBeDefined();
    expect(screen.getByText('Related Components')).toBeDefined();
    expect(screen.getByText('Summary')).toBeDefined();
  });
});

describe('Lighthouse fixture: volume-provision', () => {
  const json = JSON.stringify(volumeProvision);

  it('parses as valid DashboardData', () => {
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Volume Provisioning — 2 TB NFS High-Performance');
    expect(result!.panels.length).toBe(6);
  });

  it('renders the full dashboard', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Volume Provisioning — 2 TB NFS High-Performance')).toBeDefined();
    expect(screen.getByText('📋 Provisioning Requirements')).toBeDefined();
    expect(screen.getByText('Candidate Aggregates')).toBeDefined();
    expect(screen.getByText('aggr_ssd_east01')).toBeDefined();
    expect(screen.getByText('Available Space by Aggregate')).toBeDefined();
    expect(screen.getByText('Recommended CLI Command')).toBeDefined();
    expect(screen.getByText('⚠️ Pre-Flight Checks')).toBeDefined();
    expect(screen.getByLabelText('Volume Name')).toBeDefined();
    expect(screen.getByText('Provision on cluster-east')).toBeDefined();
  });

  it('submit button disabled in readOnly mode', () => {
    render(<DashboardBlock json={json} readOnly />);
    expect((screen.getByLabelText('Volume Name') as HTMLInputElement).value).toBe('vol_app_new');
    expect(screen.getByText('Provision on cluster-east').closest('button')?.disabled).toBe(true);
  });

  it('pre-fills volume name and enables submit', () => {
    render(<DashboardBlock json={json} />);
    expect((screen.getByLabelText('Volume Name') as HTMLInputElement).value).toBe('vol_app_new');
    expect(screen.getByText('Provision on cluster-east').closest('button')?.disabled).toBe(false);
  });
});

// --- Object-Detail Fixture Tests (§8.4) ---

describe('Object-detail fixture: alertDetail', () => {
  const json = JSON.stringify(alertDetail);

  it('parses as valid ObjectDetailData', () => {
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('alert');
    expect(result!.name).toBe('InstanceDown — node-east-01');
    expect(result!.status).toBe('critical');
    expect(result!.sections.length).toBe(7);
  });

  it('renders the full object-detail', () => {
    render(<ObjectDetailBlock json={json} />);
    expect(screen.getByRole('article', { name: 'InstanceDown — node-east-01' })).toBeDefined();
    expect(screen.getByText('Firing since 2025-06-14 09:32 UTC (4h 28m)')).toBeDefined();
    expect(screen.getByText('Alert Details')).toBeDefined();
    expect(screen.getByText('Metric Trends')).toBeDefined();
    expect(screen.getByText('Active Alerts')).toBeDefined();
    expect(screen.getByText('Timeline')).toBeDefined();
    expect(screen.getByText('Actions')).toBeDefined();
    expect(screen.getByText('Recommended Actions')).toBeDefined();
    expect(screen.getByText('Notifications Sent')).toBeDefined();
  });

  it('propagates click actions', () => {
    const onAction = vi.fn();
    render(<ObjectDetailBlock json={json} onAction={onAction} />);
    screen.getByText('Investigate Node').click();
    expect(onAction).toHaveBeenCalledWith("What's happening on node-east-01?");
  });

  it('propagates execute actions', () => {
    const onExecute = vi.fn();
    render(<ObjectDetailBlock json={json} onExecute={onExecute} />);
    screen.getByText('Silence Alert (4h)').click();
    expect(onExecute).toHaveBeenCalledWith('silence_alert', { alertname: 'InstanceDown', duration: '4h' });
  });
});

describe('Object-detail fixture: volumeDetail', () => {
  const json = JSON.stringify(volumeDetail);

  it('parses as valid ObjectDetailData', () => {
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('volume');
    expect(result!.name).toBe('vol_prod_db01');
    expect(result!.status).toBe('warning');
    expect(result!.sections.length).toBe(5);
  });

  it('renders the full object-detail', () => {
    render(<ObjectDetailBlock json={json} />);
    expect(screen.getByText('vol_prod_db01')).toBeDefined();
    expect(screen.getByText('Volume Properties')).toBeDefined();
    expect(screen.getByText('Capacity Trend — Last 30 Days')).toBeDefined();
    expect(screen.getByText('IOPS — Last 24 Hours')).toBeDefined();
    expect(screen.getByText('Active Alerts')).toBeDefined();
    expect(screen.getByText('Actions')).toBeDefined();
  });

  it('propagates click actions', () => {
    const onAction = vi.fn();
    render(<ObjectDetailBlock json={json} onAction={onAction} />);
    screen.getByText('Show Snapshots').click();
    expect(onAction).toHaveBeenCalledWith('Show snapshots for vol_prod_db01 on SVM svm_prod on cluster cluster-east');
  });
});

describe('Object-detail fixture: clusterDetail', () => {
  const json = JSON.stringify(clusterDetail);

  it('parses as valid ObjectDetailData', () => {
    const result = parseObjectDetail(json);
    expect(result).not.toBeNull();
    expect(result!.kind).toBe('cluster');
    expect(result!.name).toBe('cluster-east');
    expect(result!.status).toBe('ok');
    expect(result!.sections.length).toBe(6);
  });

  it('renders the full object-detail', () => {
    render(<ObjectDetailBlock json={json} />);
    expect(screen.getByText('cluster-east')).toBeDefined();
    expect(screen.getByText('Cluster Properties')).toBeDefined();
    expect(screen.getByText('Aggregate Usage')).toBeDefined();
    expect(screen.getByText('Cluster IOPS — Last 7 Days')).toBeDefined();
    expect(screen.getByText('Top Volumes by Usage')).toBeDefined();
    expect(screen.getByText('Active Alerts')).toBeDefined();
    expect(screen.getByText('Actions')).toBeDefined();
  });

  it('propagates click actions', () => {
    const onAction = vi.fn();
    render(<ObjectDetailBlock json={json} onAction={onAction} />);
    screen.getByText('Show All Volumes').click();
    expect(onAction).toHaveBeenCalledWith('Show all volumes on cluster-east');
  });
});

describe('Lighthouse fixture: morning-coffee-v2 (console-style)', () => {
  const json = JSON.stringify(morningCoffeeV2);

  it('parses as valid DashboardData', () => {
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Good Morning — Fleet Overview');
    expect(result!.panels.length).toBe(5);
  });

  it('renders all panels', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Good Morning — Fleet Overview')).toBeDefined();
    expect(screen.getByText('Alert Counts')).toBeDefined();
    expect(screen.getByText('Storage Capacity')).toBeDefined();
    expect(screen.getByText('Storage Performance')).toBeDefined();
    expect(screen.getByText('Top Volumes')).toBeDefined();
    expect(screen.getByText('vol_logs')).toBeDefined();
    expect(screen.getByText('💡 Recommendation')).toBeDefined();
  });

  it('capacity rows show compact format', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('67.2% (4.7 / 7.0 TiB)')).toBeDefined();
    expect(screen.getByText('59.1% (3.5 / 6.0 TiB)')).toBeDefined();
  });

  it('performance rows show used percentage', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('62.2%')).toBeDefined();
    expect(screen.getByText('39.9%')).toBeDefined();
  });

  it('volume rows include IOPS', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('1240')).toBeDefined();
    expect(screen.getByText('3420')).toBeDefined();
  });

  it('renders inline sparklines in Trend columns', () => {
    const { container } = render(<DashboardBlock json={json} />);
    // MiniSparkline renders SVGs with role="img"
    // 2 capacity (inline on Capacity) + 2 performance (inline on Used)
    // + 5 volumes Used % + 5 volumes IOPS = 14 total
    const sparklines = container.querySelectorAll('td svg[role="img"]');
    expect(sparklines.length).toBeGreaterThanOrEqual(14);
  });

  it('propagates click on cluster row', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    screen.getAllByText('cluster-east')[0].click();
    expect(onAction).toHaveBeenCalledWith('Show cluster cluster-east');
  });

  it('propagates click on volume row', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    screen.getByText('vol_logs').click();
    expect(onAction).toHaveBeenCalledWith(
      'Tell me about volume vol_logs on SVM svm_prod on cluster cluster-east'
    );
  });
});

describe('Object-list fixture: volumeList', () => {
  const json = JSON.stringify(volumeList);

  it('parses as valid DashboardData', () => {
    const result = parseDashboard(json);
    expect(result).not.toBeNull();
    expect(result!.title).toBe('Top 10 Volumes by Capacity');
    expect(result!.panels.length).toBe(2);
  });

  it('renders the volume list with sparklines', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Top 10 Volumes by Capacity')).toBeDefined();
    expect(screen.getByText('vol_logs')).toBeDefined();
    expect(screen.getByText('vol_data02')).toBeDefined();
    expect(screen.getByText('vol_prod_db01')).toBeDefined();
    // Sparklines rendered (capacity_trend + iops_trend per row = 10 total)
    const svgs = document.querySelectorAll('svg');
    expect(svgs.length).toBeGreaterThanOrEqual(10);
  });

  it('renders pagination button', () => {
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Show next 10')).toBeDefined();
  });

  it('pagination button triggers onAction', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    screen.getByText('Show next 10').click();
    expect(onAction).toHaveBeenCalledWith('Show me volumes ranked 11-20 by capacity');
  });

  it('row click triggers object detail', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    screen.getByText('vol_logs').click();
    expect(onAction).toHaveBeenCalledWith(
      'Tell me about volume vol_logs on SVM svm_prod on cluster cluster-east'
    );
  });
});
