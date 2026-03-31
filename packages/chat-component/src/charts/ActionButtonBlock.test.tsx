import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { ActionButtonBlock } from './ActionButtonBlock';
import type { ActionButtonData } from './chartTypes';

describe('ActionButtonBlock', () => {
  it('renders all buttons', () => {
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Drill Down', action: 'message', message: 'Show details' },
        { label: 'Execute', action: 'execute', tool: 'some_tool', variant: 'primary' },
      ],
    };
    render(<ActionButtonBlock data={data} />);
    expect(screen.getByText('Drill Down')).toBeDefined();
    expect(screen.getByText('Execute')).toBeDefined();
  });

  it('calls onAction for message-mode buttons', () => {
    const onAction = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Ask More', action: 'message', message: 'Tell me more' },
      ],
    };
    render(<ActionButtonBlock data={data} onAction={onAction} />);
    screen.getByText('Ask More').click();
    expect(onAction).toHaveBeenCalledWith('Tell me more');
  });

  it('calls onExecute for execute-mode buttons', () => {
    const onExecute = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Run It', action: 'execute', tool: 'my_tool', params: { key: 'val' } },
      ],
    };
    render(<ActionButtonBlock data={data} onExecute={onExecute} />);
    screen.getByText('Run It').click();
    expect(onExecute).toHaveBeenCalledWith('my_tool', { key: 'val' });
  });

  it('disables execute buttons in read-only mode', () => {
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Execute', action: 'execute', tool: 'tool1' },
        { label: 'Chat', action: 'message', message: 'hi' },
      ],
    };
    render(<ActionButtonBlock data={data} readOnly />);
    expect(screen.getByText('Execute').closest('button')?.disabled).toBe(true);
    expect(screen.getByText('Chat').closest('button')?.disabled).toBe(false);
  });

  it('renders outline variant', () => {
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Outline Btn', action: 'message', message: 'x', variant: 'outline' },
      ],
    };
    render(<ActionButtonBlock data={data} />);
    expect(screen.getByText('Outline Btn')).toBeDefined();
  });

  it('handles empty buttons array', () => {
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [],
    };
    const { container } = render(<ActionButtonBlock data={data} />);
    expect(container.querySelectorAll('button')).toHaveLength(0);
  });

  it('defaults action to message when action is missing but message is present', () => {
    const onAction = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        // LLM sometimes omits the action field
        { label: 'Show next 10', message: 'Show me volumes ranked 11-20 by capacity' } as ActionButtonData['buttons'][number],
      ],
    };
    render(<ActionButtonBlock data={data} onAction={onAction} />);
    screen.getByText('Show next 10').click();
    expect(onAction).toHaveBeenCalledWith('Show me volumes ranked 11-20 by capacity');
  });

  it('disables buttons with tool field but no explicit action in read-only mode', () => {
    // Claude often omits action:"execute" but includes tool + params
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Provision on cluster1', tool: 'ontap_volume_create', params: { size: '100GB' } } as ActionButtonData['buttons'][number],
        { label: 'Show other options', message: 'Show me other clusters' } as ActionButtonData['buttons'][number],
      ],
    };
    render(<ActionButtonBlock data={data} readOnly />);
    expect(screen.getByText('Provision on cluster1').closest('button')?.disabled).toBe(true);
    expect(screen.getByText('Show other options').closest('button')?.disabled).toBe(false);
  });

  it('infers execute action from tool field and calls onExecute', () => {
    const onExecute = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Do It', tool: 'my_tool', params: { x: 1 } } as ActionButtonData['buttons'][number],
      ],
    };
    render(<ActionButtonBlock data={data} onExecute={onExecute} />);
    screen.getByText('Do It').click();
    expect(onExecute).toHaveBeenCalledWith('my_tool', { x: 1 });
  });

  it('disables requiresReadWrite message buttons in read-only mode', () => {
    const onAction = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Monitor this Volume', action: 'message', message: 'Enable monitoring for volume vol1', requiresReadWrite: true },
        { label: 'Show Snapshots', action: 'message', message: 'Show snapshots for vol1' },
      ],
    };
    render(<ActionButtonBlock data={data} onAction={onAction} readOnly />);
    expect(screen.getByText('Monitor this Volume').closest('button')?.disabled).toBe(true);
    expect(screen.getByText('Show Snapshots').closest('button')?.disabled).toBe(false);
  });

  it('enables requiresReadWrite buttons when not in read-only mode', () => {
    const onAction = vi.fn();
    const data: ActionButtonData = {
      type: 'action-button',
      buttons: [
        { label: 'Monitor this Volume', action: 'message', message: 'Enable monitoring for volume vol1', requiresReadWrite: true },
      ],
    };
    render(<ActionButtonBlock data={data} onAction={onAction} />);
    screen.getByText('Monitor this Volume').click();
    expect(onAction).toHaveBeenCalledWith('Enable monitoring for volume vol1');
  });
});
