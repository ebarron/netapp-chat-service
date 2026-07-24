import { render, screen } from '../test-utils';
import { ChatPanel } from './ChatPanel';
import { vi, describe, it, expect, beforeEach } from 'vitest';

describe('ChatPanel variant prop (C1)', () => {
  const onClose = vi.fn();
  beforeEach(() => vi.clearAllMocks());

  it('variant="docked" renders a persistent panel even when opened={false}', async () => {
    const { container } = render(<ChatPanel opened={false} onClose={onClose} variant="docked" />);
    // Persistent: content is present despite opened=false.
    expect(await screen.findByPlaceholderText('Type a message...')).toBeDefined();
    // No default title / header bar.
    expect(screen.queryByText('AI Assistant')).toBeNull();
    expect(container.querySelector('[class*="fullPageHeader"]')).toBeNull();
    // Rendered as the docked container, not a Mantine Drawer dialog.
    expect(container.querySelector('[class*="docked"]')).not.toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('variant="docked" renders the header bar when title is provided', async () => {
    const { container } = render(
      <ChatPanel opened onClose={onClose} variant="docked" title="Planning Console" />,
    );
    expect(await screen.findByText('Planning Console')).toBeDefined();
    expect(container.querySelector('[class*="fullPageHeader"]')).not.toBeNull();
  });

  it('variant="docked" renders the header bar when only subtitle is provided', async () => {
    const { container } = render(
      <ChatPanel opened onClose={onClose} variant="docked" subtitle="Beta" />,
    );
    expect(await screen.findByText('Beta')).toBeDefined();
    expect(container.querySelector('[class*="fullPageHeader"]')).not.toBeNull();
  });

  it('default variant (drawer) is unchanged: closed renders nothing', () => {
    render(<ChatPanel opened={false} onClose={onClose} />);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByLabelText('Send')).toBeNull();
  });

  it('default variant (drawer) opened renders the slide-over dialog without a default title', () => {
    const { container } = render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.getByRole('dialog')).toBeDefined();
    expect(screen.queryByText('AI Assistant')).toBeNull();
    // Drawer path — not the docked container.
    expect(container.querySelector('[class*="docked"]')).toBeNull();
    // Close affordance is preserved even with no title (Drawer portals outside container).
    expect(document.querySelector('.mantine-Drawer-close')).not.toBeNull();
  });

  it('drawer renders title when provided', () => {
    render(<ChatPanel opened={true} onClose={onClose} title="NAbox Assistant" />);
    expect(screen.getByText('NAbox Assistant')).toBeDefined();
  });

  it('variant="drawer" is explicit-equivalent to the default', () => {
    render(<ChatPanel opened={false} onClose={onClose} variant="drawer" />);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
