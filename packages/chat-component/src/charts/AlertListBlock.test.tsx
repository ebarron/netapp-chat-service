import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { AlertListBlock } from './AlertListBlock';
import type { AlertListData } from './chartTypes';

const fixture: AlertListData = {
  type: 'alert-list',
  title: 'Active Alerts',
  items: [
    { severity: 'critical', message: 'vol_logs offline', time: '2 min ago' },
    { severity: 'warning', message: 'aggr1 at 87%', time: '15 min ago' },
    { severity: 'info', message: 'Firmware update available', time: '1 hr ago' },
  ],
};

describe('AlertListBlock', () => {
  it('renders title', () => {
    render(<AlertListBlock data={fixture} />);
    expect(screen.getByText('Active Alerts')).toBeDefined();
  });

  it('renders all alert messages', () => {
    render(<AlertListBlock data={fixture} />);
    expect(screen.getByText('vol_logs offline')).toBeDefined();
    expect(screen.getByText('aggr1 at 87%')).toBeDefined();
    expect(screen.getByText('Firmware update available')).toBeDefined();
  });

  it('renders timestamps', () => {
    render(<AlertListBlock data={fixture} />);
    expect(screen.getByText('2 min ago')).toBeDefined();
    expect(screen.getByText('15 min ago')).toBeDefined();
  });

  it('renders without title', () => {
    const noTitle: AlertListData = {
      type: 'alert-list',
      items: [{ severity: 'warning', message: 'Test alert', time: 'now' }],
    };
    render(<AlertListBlock data={noTitle} />);
    expect(screen.getByText('Test alert')).toBeDefined();
  });

  it('renders empty items list', () => {
    const empty: AlertListData = {
      type: 'alert-list',
      title: 'No Alerts',
      items: [],
    };
    render(<AlertListBlock data={empty} />);
    expect(screen.getByText('No Alerts')).toBeDefined();
    expect(screen.getByText('No alerts')).toBeDefined();
  });

  it('calls onAction with alert message when item is clicked', async () => {
    const onAction = vi.fn();
    render(<AlertListBlock data={fixture} onAction={onAction} />);
    await userEvent.click(screen.getByRole('listitem', { name: /vol_logs offline/ }));
    expect(onAction).toHaveBeenCalledWith('Tell me about the vol_logs offline alert');
  });

  it('does not throw when clicked without onAction', async () => {
    render(<AlertListBlock data={fixture} />);
    await userEvent.click(screen.getByRole('listitem', { name: /aggr1 at 87%/ }));
    // no onAction — should not throw
  });
});
