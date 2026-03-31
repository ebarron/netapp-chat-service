import { render, screen, fireEvent } from '../test-utils';
import { ActionConfirmation } from './ActionConfirmation';
import { vi, describe, it, expect } from 'vitest';
import type { PendingApproval } from './useChatPanel';

describe('ActionConfirmation', () => {
  const mockApproval: PendingApproval = {
    approvalId: 'test-1',
    capability: 'harvest',
    tool: 'metrics_query',
    params: { query: 'volume_capacity' },
    description: 'harvest → metrics_query',
  };

  const onApprove = vi.fn();
  const onDeny = vi.fn();

  it('renders nothing when approval is null', () => {
    render(
      <ActionConfirmation approval={null} onApprove={onApprove} onDeny={onDeny} />
    );
    expect(screen.queryByText('Confirm Action')).toBeNull();
  });

  it('renders dialog with approval details', () => {
    render(
      <ActionConfirmation approval={mockApproval} onApprove={onApprove} onDeny={onDeny} />
    );
    expect(screen.getByText('Confirm Action')).toBeDefined();
    expect(screen.getByText('harvest → metrics_query')).toBeDefined();
    expect(screen.getByText('harvest')).toBeDefined();
    expect(screen.getByText('metrics_query')).toBeDefined();
  });

  it('renders parameters as JSON', () => {
    render(
      <ActionConfirmation approval={mockApproval} onApprove={onApprove} onDeny={onDeny} />
    );
    expect(screen.getByText(/"query": "volume_capacity"/)).toBeDefined();
  });

  it('calls onApprove when Confirm is clicked', () => {
    render(
      <ActionConfirmation approval={mockApproval} onApprove={onApprove} onDeny={onDeny} />
    );
    fireEvent.click(screen.getByText('Confirm'));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it('calls onDeny when Cancel is clicked', () => {
    render(
      <ActionConfirmation approval={mockApproval} onApprove={onApprove} onDeny={onDeny} />
    );
    fireEvent.click(screen.getByText('Cancel'));
    expect(onDeny).toHaveBeenCalledOnce();
  });

  it('shows Confirm and Cancel buttons', () => {
    render(
      <ActionConfirmation approval={mockApproval} onApprove={onApprove} onDeny={onDeny} />
    );
    expect(screen.getByText('Confirm')).toBeDefined();
    expect(screen.getByText('Cancel')).toBeDefined();
  });
});
