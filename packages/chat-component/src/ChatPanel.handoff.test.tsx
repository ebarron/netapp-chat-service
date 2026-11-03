import { render, screen, userEvent, waitFor, createMockChatAPI } from '../test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatPanel } from './ChatPanel';

function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width });
  window.matchMedia = vi.fn().mockImplementation((query: string) => {
    const min = Number(/min-width:\s*(\d+)px/.exec(query)?.[1] ?? 0);
    return {
      matches: width >= min,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as MediaQueryList;
  });
}

function canvasResponse(kind: 'form' | 'results'): Response {
  const handoff = kind === 'form'
    ? {
        kind,
        heading: 'Provision volume is ready.',
        action_label: 'Continue in the form.',
      }
    : {
        kind,
        heading: 'Fleet health is ready.',
        action_label: 'View results.',
      };
  const content = kind === 'form'
    ? {
        type: 'dashboard',
        title: 'Provision volume',
        panels: [{
          type: 'action-form',
          fields: [{
            key: 'volume_name',
            label: 'Volume Name',
            type: 'text',
            required: true,
          }],
          submit: {
            label: 'Provision',
            tool: 'create_volume',
            params: {},
          },
        }],
      }
    : {
        type: 'dashboard',
        title: 'Fleet health',
        panels: [{ type: 'callout', title: 'Healthy', body: 'All systems operational.' }],
      };
  const event = {
    tab_id: 'dashboard::destination::',
    title: kind === 'form' ? 'Provision volume' : 'Fleet health',
    kind: 'dashboard',
    qualifier: '',
    terminal_interactive: true,
    handoff,
    content,
  };
  const encoder = new TextEncoder();
  return new Response(new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(
        `event: canvas_open\ndata: ${JSON.stringify(event)}\n\n`,
      ));
      controller.enqueue(encoder.encode(
        'event: done\ndata: {"session_id":"handoff-session"}\n\n',
      ));
      controller.close();
    },
  }), {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

describe('ChatPanel deterministic Canvas handoff', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setViewportWidth(1280);
  });

  it('announces renderer copy and focuses an already-visible form control', async () => {
    const stream = vi.fn().mockResolvedValue(canvasResponse('form'));
    const api = createMockChatAPI({ stream });
    const user = userEvent.setup();
    render(
      <ChatPanel
        opened
        onClose={vi.fn()}
        pendingPrompt="provision"
        mobileLayout
      />,
      { api },
    );

    const handoff = await screen.findByRole('status', {
      name: /provision volume is ready/i,
    });
    expect(handoff.getAttribute('aria-live')).toBe('polite');
    const action = screen.getByRole('button', { name: 'Continue in the form.' });
    await user.click(action);

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByLabelText('Volume Name'));
    });
    expect(stream).toHaveBeenCalledTimes(1);
  });

  it('supports keyboard activation and focuses a results destination', async () => {
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(canvasResponse('results')),
    });
    const user = userEvent.setup();
    render(
      <ChatPanel
        opened
        onClose={vi.fn()}
        pendingPrompt="health"
        mobileLayout
      />,
      { api },
    );

    const action = await screen.findByRole('button', { name: 'View results.' });
    action.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => {
      expect(document.activeElement?.dataset.canvasTabId).toBe(
        'dashboard::destination::',
      );
    });
  });

  it('reveals the narrow Canvas region before focusing the form', async () => {
    setViewportWidth(600);
    const onMobileRegionChange = vi.fn();
    const api = createMockChatAPI({
      stream: vi.fn().mockResolvedValue(canvasResponse('form')),
    });
    const user = userEvent.setup();
    render(
      <ChatPanel
        opened
        onClose={vi.fn()}
        pendingPrompt="provision"
        mobileLayout
        mobileRegion="assistant"
        onMobileRegionChange={onMobileRegionChange}
      />,
      { api },
    );

    await user.click(await screen.findByRole('button', {
      name: 'Continue in the form.',
    }));
    expect(onMobileRegionChange).toHaveBeenCalledWith('canvas');
    await waitFor(() => {
      const split = document.querySelector('[data-mobile-region]');
      expect(split?.getAttribute('data-mobile-region')).toBe('canvas');
      expect(document.activeElement).toBe(screen.getByLabelText('Volume Name'));
    });
  });
});
