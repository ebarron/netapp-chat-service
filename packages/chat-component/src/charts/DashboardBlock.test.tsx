import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { DashboardBlock } from './DashboardBlock';

// A realistic morning-coffee style dashboard fixture
const morningCoffeeJSON = JSON.stringify({
  title: 'Good Morning — Fleet Overview',
  panels: [
    {
      type: 'alert-summary',
      title: 'Alert Counts',
      width: 'half',
      data: { critical: 1, warning: 3, info: 2, ok: 48 },
    },
    {
      type: 'stat',
      title: 'Clusters Online',
      width: 'half',
      value: '4 / 4',
      subtitle: 'All healthy',
      trend: 'flat',
    },
    {
      type: 'area',
      title: 'Aggregate Usage Trend',
      width: 'full',
      xKey: 'day',
      series: [{ key: 'used_pct', label: 'Avg Used %', color: 'blue' }],
      data: [
        { day: 'Mon', used_pct: 62 },
        { day: 'Tue', used_pct: 63 },
        { day: 'Wed', used_pct: 64 },
        { day: 'Thu', used_pct: 65 },
        { day: 'Fri', used_pct: 66 },
      ],
    },
    {
      type: 'resource-table',
      title: 'Volumes Needing Attention',
      width: 'full',
      columns: ['Name', 'Used', 'Status'],
      rows: [
        { name: 'vol_logs', Name: 'vol_logs', Used: '95%', Status: 'Critical' },
        { name: 'vol_data02', Name: 'vol_data02', Used: '87%', Status: 'Warning' },
      ],
    },
    {
      type: 'callout',
      title: 'Recommendation',
      width: 'full',
      icon: '💡',
      body: 'vol_logs is at 95%. Consider expanding or adding a new volume.',
    },
    {
      type: 'action-button',
      width: 'full',
      buttons: [
        { label: 'Show all alerts', action: 'message', message: 'Show all active alerts' },
        { label: 'Check capacities', action: 'message', message: 'Show all volumes over 80%' },
      ],
    },
  ],
});

describe('DashboardBlock', () => {
  it('renders the dashboard title', () => {
    render(<DashboardBlock json={morningCoffeeJSON} />);
    expect(screen.getByText('Good Morning — Fleet Overview')).toBeDefined();
  });

  it('renders all panel types within the dashboard', () => {
    render(<DashboardBlock json={morningCoffeeJSON} />);
    // Alert summary
    expect(screen.getByText('critical: 1')).toBeDefined();
    // Stat
    expect(screen.getByText('Clusters Online')).toBeDefined();
    expect(screen.getByText('4 / 4')).toBeDefined();
    // Area chart title
    expect(screen.getByText('Aggregate Usage Trend')).toBeDefined();
    // Resource table
    expect(screen.getByText('Volumes Needing Attention')).toBeDefined();
    expect(screen.getByText('vol_logs')).toBeDefined();
    // Callout
    expect(screen.getByText('💡 Recommendation')).toBeDefined();
    // Action buttons
    expect(screen.getByText('Show all alerts')).toBeDefined();
  });

  it('propagates onAction from clickable panels', () => {
    const onAction = vi.fn();
    render(<DashboardBlock json={morningCoffeeJSON} onAction={onAction} />);

    // Click an alert badge
    screen.getByText('critical: 1').click();
    expect(onAction).toHaveBeenCalledWith('Show me the critical alerts');

    // Click a resource table row
    screen.getByText('vol_logs').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about vol_logs');

    // Click an action button
    screen.getByText('Show all alerts').click();
    expect(onAction).toHaveBeenCalledWith('Show all active alerts');
  });

  it('falls back to code block for invalid JSON', () => {
    render(<DashboardBlock json="not json" />);
    expect(screen.getByText('not json')).toBeDefined();
  });

  it('falls back to code block for missing title', () => {
    const json = JSON.stringify({ panels: [] });
    render(<DashboardBlock json={json} />);
    expect(screen.getByText(json)).toBeDefined();
  });

  it('skips unknown panel types gracefully', () => {
    const json = JSON.stringify({
      title: 'Mixed Dashboard',
      panels: [
        { type: 'stat', title: 'Valid', value: '42' },
        { type: 'unknown-panel', data: {} },
        { type: 'callout', title: 'Also Valid', body: 'OK' },
      ],
    });
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Mixed Dashboard')).toBeDefined();
    expect(screen.getByText('Valid')).toBeDefined();
    expect(screen.getByText('Also Valid')).toBeDefined();
  });

  it('renders panels with different widths', () => {
    const json = JSON.stringify({
      title: 'Width Test',
      panels: [
        { type: 'stat', title: 'Full', value: '1', width: 'full' },
        { type: 'stat', title: 'Half A', value: '2', width: 'half' },
        { type: 'stat', title: 'Half B', value: '3', width: 'half' },
        { type: 'stat', title: 'Third A', value: '4', width: 'third' },
        { type: 'stat', title: 'Third B', value: '5', width: 'third' },
        { type: 'stat', title: 'Third C', value: '6', width: 'third' },
      ],
    });
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Full')).toBeDefined();
    expect(screen.getByText('Half A')).toBeDefined();
    expect(screen.getByText('Half B')).toBeDefined();
    expect(screen.getByText('Third A')).toBeDefined();
    expect(screen.getByText('Third B')).toBeDefined();
    expect(screen.getByText('Third C')).toBeDefined();
  });

  it('renders empty panels array', () => {
    const json = JSON.stringify({ title: 'Empty Dashboard', panels: [] });
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('Empty Dashboard')).toBeDefined();
  });

  it('passes readOnly to action buttons', () => {
    const json = JSON.stringify({
      title: 'Read-Only Dashboard',
      panels: [
        {
          type: 'action-button',
          buttons: [
            { label: 'Exec', action: 'execute', tool: 'tool1' },
            { label: 'Chat', action: 'message', message: 'hi' },
          ],
        },
      ],
    });
    render(<DashboardBlock json={json} readOnly />);
    expect(screen.getByText('Exec').closest('button')?.disabled).toBe(true);
    expect(screen.getByText('Chat').closest('button')?.disabled).toBe(false);
  });

  it('uses CSS Grid layout with panelGrid class', () => {
    const json = JSON.stringify({
      title: 'Grid Layout Test',
      panels: [
        { type: 'stat', title: 'Half A', value: '1', width: 'half' },
        { type: 'stat', title: 'Half B', value: '2', width: 'half' },
        { type: 'stat', title: 'Full', value: '3', width: 'full' },
      ],
    });
    const { container } = render(<DashboardBlock json={json} />);
    // All panels share a single grid container (no nested SimpleGrids)
    const gridDiv = container.querySelector('[class*="panelGrid"]');
    expect(gridDiv).toBeDefined();
    expect(gridDiv?.children.length).toBe(3);
  });

  it('applies max-width on the dashboard container', () => {
    const json = JSON.stringify({ title: 'Max Width Test', panels: [] });
    const { container } = render(<DashboardBlock json={json} />);
    const dashboard = container.querySelector('[class*="dashboard"]');
    expect(dashboard).toBeDefined();
  });

  it('adds aria-label region for accessibility', () => {
    render(<DashboardBlock json={morningCoffeeJSON} />);
    const region = screen.getByRole('region', { name: 'Good Morning — Fleet Overview' });
    expect(region).toBeDefined();
  });

  it('renders toggle badge and calls onAction when clicked', () => {
    const json = JSON.stringify({
      title: 'Fleet Health',
      toggle: { label: 'Show Detailed', message: 'show me a per cluster view of my fleet' },
      panels: [],
    });
    const onAction = vi.fn();
    render(<DashboardBlock json={json} onAction={onAction} />);
    const badge = screen.getByText('Show Detailed');
    expect(badge).toBeDefined();
    badge.click();
    expect(onAction).toHaveBeenCalledWith('show me a per cluster view of my fleet');
  });

  it('does not render toggle badge when toggle is absent', () => {
    const json = JSON.stringify({ title: 'No Toggle', panels: [] });
    render(<DashboardBlock json={json} />);
    expect(screen.getByText('No Toggle')).toBeDefined();
    // No badge rendered
    expect(screen.queryByText('Show Detailed')).toBeNull();
    expect(screen.queryByText('Show Summary')).toBeNull();
  });
});
