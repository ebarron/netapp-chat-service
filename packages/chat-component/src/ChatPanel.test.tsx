import { render, screen, createMockChatAPI } from '../test-utils';
import { ChatPanel } from './ChatPanel';
import { vi, describe, it, expect, beforeEach } from 'vitest';

describe('ChatPanel', () => {
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders when opened', () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.getByText('AI Assistant')).toBeDefined();
  });

  it('renders custom title', () => {
    render(<ChatPanel opened={true} onClose={onClose} title="NAbox Assistant" />);
    expect(screen.getByText('NAbox Assistant')).toBeDefined();
  });

  it('shows suggested prompts when empty', async () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(await screen.findByText("What's the health of my fleet?")).toBeDefined();
    expect(screen.getByText('Show volumes over 80% capacity')).toBeDefined();
    expect(
      screen.getByText(/interacting with a chat bot supported by artificial intelligence/i)
    ).toBeDefined();
  });

  it('has an input textarea', async () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(await screen.findByPlaceholderText('Type a message...')).toBeDefined();
  });

  it('has a send button', () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.getByLabelText('Send')).toBeDefined();
  });

  it('has a clear button', () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.getByLabelText('Clear')).toBeDefined();
  });

  it('does not render content when closed', () => {
    render(<ChatPanel opened={false} onClose={onClose} />);
    expect(screen.queryByText('AI Assistant')).toBeNull();
  });

  it('shows not configured alert when AI is not set up', async () => {
    const api = createMockChatAPI({
      get: vi.fn().mockResolvedValue({ configured: false }),
    });

    render(<ChatPanel opened={true} onClose={onClose} />, { api });

    expect(screen.getByText('AI Assistant')).toBeDefined();
  });

  it('does not render CanvasPanel on narrow viewports (jsdom default width is 0)', () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(document.querySelector('[class*="canvasRegion"]')).toBeNull();
    expect(screen.queryByRole('tablist')).toBeNull();
  });

  describe('defaultMode prop wiring', () => {
    it('starts in read-write mode by default (no prop)', async () => {
      render(<ChatPanel opened={true} onClose={onClose} />);
      const rw = await screen.findByDisplayValue('read-write');
      expect((rw as HTMLInputElement).checked).toBe(true);
    });

    it('starts in read-only mode when defaultMode="read-only" is passed', async () => {
      render(<ChatPanel opened={true} onClose={onClose} defaultMode="read-only" />);
      const ro = await screen.findByDisplayValue('read-only');
      expect((ro as HTMLInputElement).checked).toBe(true);
    });

    it('starts in read-write mode when defaultMode="read-write" is passed explicitly', async () => {
      render(<ChatPanel opened={true} onClose={onClose} defaultMode="read-write" />);
      const rw = await screen.findByDisplayValue('read-write');
      expect((rw as HTMLInputElement).checked).toBe(true);
    });
  });
}); 
