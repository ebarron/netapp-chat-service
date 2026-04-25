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
      read_only_tools_count: 8,
    },
    {
      id: 'ontap',
      name: 'ONTAP',
      description: 'Volume management',
      state: 'off',
      available: false,
      tools_count: 0,
      read_only_tools_count: 0,
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
      expect(screen.getByText('12 tools (8 ro)')).toBeDefined();
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

  it('renders the tool budget bar with current usage', async () => {
    const user = userEvent.setup();
    render(
      <CapabilityControls
        capabilities={mockCapabilities}
        onUpdate={onUpdate}
        mode="read-only"
        toolBudgets={{
          read_only: { used: 50, max: 128 },
          read_write: { used: 80, max: 128 },
        }}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('Tool budget (read-only)')).toBeDefined();
    });
    expect(screen.getByText('50 / 128')).toBeDefined();
  });

  it('shows over-budget warning when used > max', async () => {
    const user = userEvent.setup();
    render(
      <CapabilityControls
        capabilities={mockCapabilities}
        onUpdate={onUpdate}
        mode="read-only"
        toolBudgets={{
          read_only: { used: 135, max: 128 },
          read_write: { used: 200, max: 128 },
        }}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('135 / 128')).toBeDefined();
    });
    expect(screen.getByText(/Over the 128-tool limit/)).toBeDefined();
  });

  it('disables Ask/Allow when toggling a capability ON would exceed budget', async () => {
    const user = userEvent.setup();
    // ontap is StateOff with read_only_tools_count=80; budget is 50/128 used
    // → enabling ontap would push to 130, exceeding 128.
    const caps: Capability[] = [
      { ...mockCapabilities[0] },
      { ...mockCapabilities[1], available: true, tools_count: 80, read_only_tools_count: 80 },
    ];
    render(
      <CapabilityControls
        capabilities={caps}
        onUpdate={onUpdate}
        mode="read-only"
        toolBudgets={{
          read_only: { used: 50, max: 128 },
          read_write: { used: 50, max: 128 },
        }}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText('over budget')).toBeDefined();
    });
  });

  it('surfaces capabilityError to the user', async () => {
    const user = userEvent.setup();
    const onClear = vi.fn();
    render(
      <CapabilityControls
        capabilities={mockCapabilities}
        onUpdate={onUpdate}
        capabilityError="Enabling these capabilities would use 200 tools (max 128)."
        onClearCapabilityError={onClear}
      />
    );
    await user.click(screen.getByLabelText('Capability settings'));

    await waitFor(() => {
      expect(screen.getByText(/would use 200 tools/)).toBeDefined();
    });
  });
});
