import { render, screen, waitFor } from '../test-utils';
import { ChatPanel } from './ChatPanel';
import { vi, describe, it, expect, beforeEach } from 'vitest';

describe('ChatPanel variant prop (C1)', () => {
  const onClose = vi.fn();
  beforeEach(() => vi.clearAllMocks());

  it('variant="docked" renders a persistent panel even when opened={false}', async () => {
    const { container } = render(<ChatPanel opened={false} onClose={onClose} variant="docked" />);
    // Persistent: content is present despite opened=false.
    expect(await screen.findByText('AI Assistant')).toBeDefined();
    expect(await screen.findByPlaceholderText('Type a message...')).toBeDefined();
    // Rendered as the docked container, not a Mantine Drawer dialog.
    expect(container.querySelector('[class*="docked"]')).not.toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('default variant (drawer) is unchanged: closed renders nothing', () => {
    render(<ChatPanel opened={false} onClose={onClose} />);
    expect(screen.queryByText('AI Assistant')).toBeNull();
  });

  it('default variant (drawer) opened renders the slide-over dialog (byte-for-byte parity)', () => {
    const { container } = render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.getByText('AI Assistant')).toBeDefined();
    // Drawer path — not the docked container.
    expect(container.querySelector('[class*="docked"]')).toBeNull();
  });

  it('variant="drawer" is explicit-equivalent to the default', () => {
    render(<ChatPanel opened={false} onClose={onClose} variant="drawer" />);
    expect(screen.queryByText('AI Assistant')).toBeNull();
  });
});
