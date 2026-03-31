import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { AlertSummaryBlock } from './AlertSummaryBlock';
import type { AlertSummaryData } from './chartTypes';

const fixture: AlertSummaryData = {
  type: 'alert-summary',
  title: 'Alert Overview',
  data: { critical: 2, warning: 5, info: 3, ok: 42 },
};

describe('AlertSummaryBlock', () => {
  it('renders severity counts excluding ok', () => {
    render(<AlertSummaryBlock data={fixture} />);
    expect(screen.getByText('Alert Overview')).toBeDefined();
    expect(screen.getByText('critical: 2')).toBeDefined();
    expect(screen.getByText('warning: 5')).toBeDefined();
    expect(screen.getByText('info: 3')).toBeDefined();
    expect(screen.queryByText('ok: 42')).toBeNull();
  });

  it('calls onAction when a severity badge is clicked', async () => {
    const onAction = vi.fn();
    render(<AlertSummaryBlock data={fixture} onAction={onAction} />);

    screen.getByText('critical: 2').click();
    expect(onAction).toHaveBeenCalledWith('Show me the critical alerts');
  });

  it('does not call onAction when zero-count badge is clicked', () => {
    const onAction = vi.fn();
    const zeroCounts: AlertSummaryData = {
      type: 'alert-summary',
      data: { critical: 0, warning: 0, info: 0, ok: 5 },
    };
    render(<AlertSummaryBlock data={zeroCounts} onAction={onAction} />);

    screen.getByText('critical: 0').click();
    expect(onAction).not.toHaveBeenCalled();
  });

  it('renders without title', () => {
    const noTitle: AlertSummaryData = {
      type: 'alert-summary',
      data: { critical: 0, warning: 0, info: 0, ok: 10 },
    };
    render(<AlertSummaryBlock data={noTitle} />);
    expect(screen.getByText('critical: 0')).toBeDefined();
    expect(screen.queryByText('ok: 10')).toBeNull();
  });

  it('does not crash when onAction is not provided', () => {
    render(<AlertSummaryBlock data={fixture} />);
    // Clicking should not throw
    screen.getByText('warning: 5').click();
  });
});
