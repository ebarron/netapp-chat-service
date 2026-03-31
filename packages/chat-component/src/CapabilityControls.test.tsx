import { render, screen, waitFor, userEvent } from '../test-utils';
import { CapabilityControls } from './CapabilityControls';
import { vi, describe, it, expect } from 'vitest';
import type { Capability } from './useChatPanel';

describe('CapabilityControls', () => {
  const mockCapabilities: Capability[] = [
    {
      id: 'harvest',
      name: 'Harvest',
      description: 'Infrastructure metrics',
      state: 'ask',
      available: true,
      tools_count: 12,
    },
    {
      id: 'ontap',
      name: 'ONTAP',
      description: 'Volume management',
      state: 'off',
      available: false,
      tools_count: 0,
    },
  ];

  const onUpdate = vi.fn();

  it('renders settings button', () => {
    render(<CapabilityControls capabilities={mockCapabilities} onUpdate={onUpdate} />);
    expect(screen.getByLabelText('Capability settings')).toBeDefined();
  });

  it('shows capabilities when popover is opened', async () => {
    const user = userEvent.setup();
    render(<CapabilityControls capabilities={mockCapabilities} onUpdate={onUpdate} />);
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('Capabilities')).toBeDefined();
    });
    expect(screen.getByText('Harvest')).toBeDefined();
    expect(screen.getByText('ONTAP')).toBeDefined();
  });

  it('shows unavailable badge for offline MCPs', async () => {
    const user = userEvent.setup();
    render(<CapabilityControls capabilities={mockCapabilities} onUpdate={onUpdate} />);
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('unavailable')).toBeDefined();
    });
  });

  it('shows tools count badge', async () => {
    const user = userEvent.setup();
    render(<CapabilityControls capabilities={mockCapabilities} onUpdate={onUpdate} />);
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('12 tools')).toBeDefined();
    });
  });

  it('shows empty state when no capabilities', async () => {
    const user = userEvent.setup();
    render(<CapabilityControls capabilities={[]} onUpdate={onUpdate} />);
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('No capabilities available.')).toBeDefined();
    });
  });

  it('renders descriptions', async () => {
    const user = userEvent.setup();
    render(<CapabilityControls capabilities={mockCapabilities} onUpdate={onUpdate} />);
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('Infrastructure metrics')).toBeDefined();
    });
    expect(screen.getByText('Volume management')).toBeDefined();
  });

  it('shows the "Show tool traces" toggle', async () => {
    const user = userEvent.setup();
    render(
      <CapabilityControls
        capabilities={mockCapabilities}
        onUpdate={onUpdate}
        showTraces={true}
        onShowTracesChange={vi.fn()}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('Show tool traces')).toBeDefined();
    });
    expect(screen.getByLabelText('Show tool traces')).toBeDefined();
  });

  it('calls onShowTracesChange when toggle is clicked', async () => {
    const onTracesChange = vi.fn();
    const user = userEvent.setup();
    render(
      <CapabilityControls
        capabilities={mockCapabilities}
        onUpdate={onUpdate}
        showTraces={true}
        onShowTracesChange={onTracesChange}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByLabelText('Show tool traces')).toBeDefined();
    });

    await user.click(screen.getByLabelText('Show tool traces'));
    expect(onTracesChange).toHaveBeenCalledWith(false);
  });
});
