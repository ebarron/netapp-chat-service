import { createRef, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { render, screen, waitFor, act } from '../test-utils';
import { ChatPanel, type ChatPanelHandle } from './ChatPanel';
import { vi, describe, it, expect, beforeEach } from 'vitest';

// These tests use a GENERIC fake host (no NABox pages/routes) to prove the
// portal host-content slot (C2–C4) is app-unaware: the component exposes a
// mount node and the host portals its own tree in.

beforeEach(() => {
  // Canvas region only renders on wide viewports (>=1024px).
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
  vi.clearAllMocks();
});

/** A generic host that portals its own content into the exposed mount node. */
function GenericFakeHost() {
  const ref = useRef<ChatPanelHandle>(null);
  const [el, setEl] = useState<HTMLElement | null>(null);
  useEffect(() => {
    ref.current?.openHostCanvasTab({ tabId: 'nav', title: 'Nav', kind: 'nav-view', evictable: false });
  }, []);
  return (
    <>
      <ChatPanel
        ref={ref}
        opened
        onClose={() => {}}
        variant="docked"
        onHostTabPortal={(tabId, node) => {
          if (tabId === 'nav') setEl(node);
        }}
      />
      {el && createPortal(<div data-testid="host-page">HOST PAGE CONTENT</div>, el)}
    </>
  );
}

describe('ChatPanel host-content portal slot (C2–C4)', () => {
  it('exposes a mount node the host portals its own content into', async () => {
    render(<GenericFakeHost />);
    const hostPage = await screen.findByTestId('host-page');
    expect(hostPage.textContent).toBe('HOST PAGE CONTENT');
    // The host content lives inside the component's canvas host-tab mount node.
    expect(hostPage.closest('[data-canvas-host-tab="nav"]')).not.toBeNull();
  });

  it('reports the mount element on open and null on close (portal lifecycle)', async () => {
    const onHostTabPortal = vi.fn();
    const ref = createRef<ChatPanelHandle>();
    render(
      <ChatPanel ref={ref} opened onClose={() => {}} variant="docked" onHostTabPortal={onHostTabPortal} />,
    );

    act(() => {
      ref.current!.openHostCanvasTab({ tabId: 'nav', title: 'Nav', evictable: false });
    });

    await waitFor(() => {
      const call = onHostTabPortal.mock.calls.find((c) => c[0] === 'nav' && c[1] instanceof HTMLElement);
      expect(call).toBeTruthy();
    });

    act(() => {
      ref.current!.closeCanvasTab('nav');
    });

    await waitFor(() => {
      const nullCall = onHostTabPortal.mock.calls.find((c) => c[0] === 'nav' && c[1] === null);
      expect(nullCall).toBeTruthy();
    });
  });
});
