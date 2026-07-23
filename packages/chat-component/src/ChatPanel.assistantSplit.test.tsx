import { useEffect, useRef, type ComponentProps } from 'react';
import { fireEvent, render, screen, waitFor, act } from '../test-utils';
import { ChatPanel, type ChatPanelHandle } from './ChatPanel';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';

beforeEach(() => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
  vi.clearAllMocks();
  localStorage.clear();
});

afterEach(() => {
  localStorage.clear();
});

function DockedWithCanvas({
  ...props
}: ComponentProps<typeof ChatPanel>) {
  const ref = useRef<ChatPanelHandle>(null);
  useEffect(() => {
    ref.current?.openHostCanvasTab({
      tabId: 'nav',
      title: 'Nav',
      kind: 'nav-view',
      evictable: false,
    });
  }, []);
  return (
    <ChatPanel
      ref={ref}
      opened
      onClose={() => {}}
      variant="docked"
      {...props}
    />
  );
}

function mockSplitRowLayout(container: HTMLElement, rowWidth = 1000, panelWidth = 400) {
  const row = container.querySelector('[class*="drawerBody"]') as HTMLElement | null;
  const panel = container.querySelector('[class*="drawerBody"] [class*="panel"]') as HTMLElement | null;
  if (row) {
    vi.spyOn(row, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      width: rowWidth,
      height: 600,
      top: 0,
      left: 0,
      right: rowWidth,
      bottom: 600,
      toJSON: () => ({}),
    } as DOMRect);
  }
  if (panel) {
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      width: panelWidth,
      height: 600,
      top: 0,
      left: 0,
      right: panelWidth,
      bottom: 600,
      toJSON: () => ({}),
    } as DOMRect);
  }
  return { row, panel };
}

describe('ChatPanel assistant/canvas split', () => {
  it('preserves legacy DOM when no split props are used', async () => {
    const { container } = render(<DockedWithCanvas />);
    await screen.findByRole('tab', { name: /Nav/i });

    const row = container.querySelector('[class*="drawerBody"]');
    expect(row?.className.includes('assistantSized')).toBe(false);
    expect(screen.queryByTestId('assistant-split-handle')).toBeNull();
    expect(row?.getAttribute('style')).toBeNull();
  });

  it('applies assistantWidth via CSS custom property in sized mode', async () => {
    const { container } = render(<DockedWithCanvas assistantWidth={480} />);
    await screen.findByRole('tab', { name: /Nav/i });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    expect(row.className.includes('assistantSized')).toBe(true);
    expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('480px');
  });

  it('renders an accessible split handle when resizableAssistant is enabled', async () => {
    render(<DockedWithCanvas resizableAssistant />);
    await screen.findByRole('tab', { name: /Nav/i });

    const handle = screen.getByTestId('assistant-split-handle');
    expect(handle.getAttribute('role')).toBe('separator');
    expect(handle.getAttribute('aria-orientation')).toBe('vertical');
  });

  it('drag-resizes the assistant column and notifies the host in pixels', async () => {
    const onAssistantWidthChange = vi.fn();
    const { container } = render(
      <DockedWithCanvas resizableAssistant onAssistantWidthChange={onAssistantWidthChange} />,
    );
    await screen.findByRole('tab', { name: /Nav/i });
    mockSplitRowLayout(container);

    const handle = screen.getByTestId('assistant-split-handle');
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1, buttons: 1 });
    fireEvent.pointerMove(handle, { clientX: 140, pointerId: 1, buttons: 1 });
    fireEvent.pointerUp(handle, { clientX: 140, pointerId: 1 });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    await waitFor(() => {
      expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('440px');
    });
    expect(onAssistantWidthChange).toHaveBeenCalled();
    expect(onAssistantWidthChange.mock.calls.at(-1)?.[0]).toBe(440);
  });

  it('restores persisted assistant width from localStorage on mount', async () => {
    localStorage.setItem('test-assistant-width', '520');
    const { container } = render(
      <DockedWithCanvas
        resizableAssistant
        persistAssistantWidthKey="test-assistant-width"
      />,
    );
    await screen.findByRole('tab', { name: /Nav/i });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    await waitFor(() => {
      expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('520px');
    });
  });

  it('double-click resets to the default assistant width', async () => {
    const onAssistantWidthChange = vi.fn();
    const { container } = render(
      <DockedWithCanvas
        resizableAssistant
        onAssistantWidthChange={onAssistantWidthChange}
      />,
    );
    await screen.findByRole('tab', { name: /Nav/i });
    mockSplitRowLayout(container);

    const handle = screen.getByTestId('assistant-split-handle');
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1, buttons: 1 });
    fireEvent.pointerMove(handle, { clientX: 200, pointerId: 1, buttons: 1 });
    fireEvent.pointerUp(handle, { clientX: 200, pointerId: 1 });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    expect(row.style.getPropertyValue('--chat-assistant-width').trim()).not.toBe('');

    fireEvent.doubleClick(handle);
    expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('');
    expect(onAssistantWidthChange).toHaveBeenCalled();
  });

  it('supports keyboard nudging on the split handle', async () => {
    const onAssistantWidthChange = vi.fn();
    const { container } = render(
      <DockedWithCanvas
        resizableAssistant
        defaultAssistantWidth={400}
        onAssistantWidthChange={onAssistantWidthChange}
      />,
    );
    await screen.findByRole('tab', { name: /Nav/i });
    mockSplitRowLayout(container, 1000, 400);

    const handle = screen.getByTestId('assistant-split-handle');
    act(() => {
      handle.focus();
    });
    fireEvent.keyDown(handle, { key: 'ArrowRight' });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    await waitFor(() => {
      expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('416px');
    });
    expect(onAssistantWidthChange).toHaveBeenCalledWith(416);
  });

  it('no placement/collapse DOM when those props are omitted', async () => {
    const { container } = render(<DockedWithCanvas />);
    await screen.findByRole('tab', { name: /Nav/i });

    expect(container.querySelector('[class*="assistantPlacementEnd"]')).toBeNull();
    expect(container.querySelector('[class*="assistantCollapsed"]')).toBeNull();
    expect(screen.queryByTestId('assistant-reveal-rail')).toBeNull();
    expect(screen.queryByTestId('assistant-hide-button')).toBeNull();
  });

  it('C8a placement="end" adds the ordering class and inverts drag direction', async () => {
    const onAssistantWidthChange = vi.fn();
    const { container } = render(
      <DockedWithCanvas
        resizableAssistant
        assistantPlacement="end"
        onAssistantWidthChange={onAssistantWidthChange}
      />,
    );
    await screen.findByRole('tab', { name: /Nav/i });
    expect(container.querySelector('[class*="assistantPlacementEnd"]')).not.toBeNull();
    mockSplitRowLayout(container);

    // Assistant is on the right: dragging LEFT (negative delta) grows it.
    const handle = screen.getByTestId('assistant-split-handle');
    fireEvent.pointerDown(handle, { clientX: 200, pointerId: 1, buttons: 1 });
    fireEvent.pointerMove(handle, { clientX: 160, pointerId: 1, buttons: 1 });
    fireEvent.pointerUp(handle, { clientX: 160, pointerId: 1 });

    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    await waitFor(() => {
      // startWidth 400 + (-40 * -1) = 440.
      expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('440px');
    });
    expect(onAssistantWidthChange.mock.calls.at(-1)?.[0]).toBe(440);
  });

  it('C8b collapsed hides the resize handle and shows the reveal rail', async () => {
    render(<DockedWithCanvas resizableAssistant defaultAssistantCollapsed />);
    await screen.findByRole('tab', { name: /Nav/i });

    // No drag handle while collapsed; reveal rail present with documented roles.
    expect(screen.queryByTestId('assistant-split-handle')).toBeNull();
    const rail = screen.getByRole('button', { name: 'Show assistant' });
    expect(rail.getAttribute('aria-expanded')).toBe('false');
  });
});
