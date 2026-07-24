import { render, screen, createMockChatAPI, waitFor } from '../test-utils';
import { ChatPanel } from './ChatPanel';
import { vi, describe, it, expect, beforeEach } from 'vitest';

describe('ChatPanel', () => {
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders when opened with no default title', () => {
    render(<ChatPanel opened={true} onClose={onClose} />);
    expect(screen.queryByText('AI Assistant')).toBeNull();
    expect(screen.getByLabelText('Send')).toBeDefined();
  });

  it('renders custom title', () => {
    render(<ChatPanel opened={true} onClose={onClose} title="NAbox Assistant" />);
    expect(screen.getByText('NAbox Assistant')).toBeDefined();
  });

  it('renders subtitle when provided without a title', () => {
    render(<ChatPanel opened={true} onClose={onClose} subtitle="Beta" />);
    expect(screen.getByText('Beta')).toBeDefined();
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
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByLabelText('Send')).toBeNull();
  });

  it('shows not configured alert when AI is not set up', async () => {
    const api = createMockChatAPI({
      get: vi.fn().mockResolvedValue({ configured: false }),
    });

    render(<ChatPanel opened={true} onClose={onClose} />, { api });

    expect(await screen.findByText('AI Not Configured')).toBeDefined();
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

/** Build a minimal SSE Response that completes immediately, for stream() mocks. */
function makeSSEResponse(): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('event: done\ndata: {"session_id":"s1"}\n\n'));
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

const flush = () => new Promise((r) => setTimeout(r, 25));

// Spec: docs/host-prompt-injection.md §6
describe('host-driven prompt injection', () => {
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('1. auto-sends pendingPrompt once when opened + idle', async () => {
    const stream = vi.fn().mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });
    render(
      <ChatPanel opened={true} onClose={onClose} pendingPrompt="explain rule X" />,
      { api }
    );
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(1));
    expect((stream.mock.calls[0][1] as { message: string }).message).toBe('explain rule X');
  });

  it('2. does not resend on an unrelated re-render', async () => {
    const stream = vi.fn().mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });
    const { rerender } = render(
      <ChatPanel opened={true} onClose={onClose} pendingPrompt="X" />,
      { api }
    );
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(1));
    rerender(<ChatPanel opened={true} onClose={onClose} pendingPrompt="X" />);
    await flush();
    expect(stream).toHaveBeenCalledTimes(1);
  });

  it('3. calls onPromptConsumed after sending', async () => {
    const stream = vi.fn().mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });
    const onPromptConsumed = vi.fn();
    render(
      <ChatPanel
        opened={true}
        onClose={onClose}
        pendingPrompt="X"
        onPromptConsumed={onPromptConsumed}
      />,
      { api }
    );
    await waitFor(() => expect(onPromptConsumed).toHaveBeenCalledTimes(1));
    expect(stream).toHaveBeenCalledTimes(1);
  });

  it('4. defers the send while streaming, then sends once idle', async () => {
    let resolveFirst!: (r: Response) => void;
    const firstStream = new Promise<Response>((r) => {
      resolveFirst = r;
    });
    const stream = vi
      .fn()
      .mockReturnValueOnce(firstStream)
      .mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });

    const { rerender } = render(
      <ChatPanel opened={true} onClose={onClose} pendingPrompt="first" />,
      { api }
    );
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(1));
    expect((stream.mock.calls[0][1] as { message: string }).message).toBe('first');

    // While the first turn is still streaming, change the prompt: must defer.
    rerender(<ChatPanel opened={true} onClose={onClose} pendingPrompt="second" />);
    await flush();
    expect(stream).toHaveBeenCalledTimes(1);

    // Finish the first turn -> idle -> deferred prompt is sent.
    resolveFirst(makeSSEResponse());
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(2));
    expect((stream.mock.calls[1][1] as { message: string }).message).toBe('second');
  });

  it('5. re-sends identical text after the host clears the prompt', async () => {
    const stream = vi.fn().mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });
    const { rerender } = render(
      <ChatPanel opened={true} onClose={onClose} pendingPrompt="X" />,
      { api }
    );
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(1));
    // Host clears its state…
    rerender(<ChatPanel opened={true} onClose={onClose} pendingPrompt="" />);
    await flush();
    // …then deliberately asks the same thing again.
    rerender(<ChatPanel opened={true} onClose={onClose} pendingPrompt="X" />);
    await waitFor(() => expect(stream).toHaveBeenCalledTimes(2));
  });

  it('6. notifies onBusyChange on stream start and end', async () => {
    let resolveStream!: (r: Response) => void;
    const pending = new Promise<Response>((r) => {
      resolveStream = r;
    });
    const stream = vi.fn().mockReturnValueOnce(pending);
    const api = createMockChatAPI({ stream });
    const onBusyChange = vi.fn();

    render(
      <ChatPanel
        opened={true}
        onClose={onClose}
        pendingPrompt="X"
        onBusyChange={onBusyChange}
      />,
      { api }
    );
    await waitFor(() => expect(onBusyChange).toHaveBeenCalledWith(true));
    resolveStream(makeSSEResponse());
    await waitFor(() => expect(onBusyChange.mock.calls.at(-1)?.[0]).toBe(false));
  });

  it('7. is a no-op when none of the new props are provided', async () => {
    const stream = vi.fn().mockResolvedValue(makeSSEResponse());
    const api = createMockChatAPI({ stream });
    render(<ChatPanel opened={true} onClose={onClose} />, { api });
    await flush();
    expect(stream).not.toHaveBeenCalled();
  });
});
