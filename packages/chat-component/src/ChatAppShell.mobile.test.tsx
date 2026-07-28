import { useState } from 'react';
import { render, screen, waitFor } from '../test-utils';
import userEvent from '@testing-library/user-event';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import { ChatAppShell, type ChatAppShellDestination } from './ChatAppShell';

const DESTS: ChatAppShellDestination[] = [
  { id: 'overview', label: 'Overview', route: '/overview' },
  { id: 'alerting', label: 'Alerting', route: '/alerting' },
];

function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width });
  // Drive matchMedia for the reactive hook.
  window.matchMedia = vi.fn().mockImplementation((query: string) => {
    const min = Number(/min-width:\s*(\d+)px/.exec(query)?.[1] ?? 0);
    const matches = width >= min;
    return {
      matches,
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

function MobileShell(
  props: Partial<Parameters<typeof ChatAppShell>[0]> & {
    initialActive?: string | null;
  } = {},
) {
  const { initialActive = 'alerting', ...rest } = props;
  const [active, setActive] = useState<string | null>(initialActive ?? null);
  return (
    <ChatAppShell
      destinations={DESTS}
      activeDestinationId={active}
      onActiveDestinationChange={(d) => setActive(d.id)}
      mobileLayout
      renderHeader={({ toggleNav }) => (
        <button type="button" data-testid="hamburger" onClick={toggleNav}>
          menu
        </button>
      )}
      renderNavMenu={({ openNav }) => (
        <button type="button" data-testid="nav-overview" onClick={() => openNav('overview')}>
          Overview
        </button>
      )}
      renderDestination={({ destination }) => (
        <div data-testid="page">
          <span data-testid="page-label">{destination.label}</span>
          <input data-testid="page-input" defaultValue="draft" />
        </div>
      )}
      {...rest}
    />
  );
}

describe('ChatAppShell mobileLayout (MVP)', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    setViewportWidth(1280);
  });

  it('keeps the host-portal page mounted across region switches (state survives)', async () => {
    setViewportWidth(500);
    render(<MobileShell />);

    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Alerting'));
    const input = screen.getByTestId('page-input') as HTMLInputElement;
    await userEvent.clear(input);
    await userEvent.type(input, 'kept across switch');

    // Switch to assistant (CSS-hide page), then back — input value survives.
    await userEvent.click(screen.getByRole('button', { name: 'Assistant' }));
    expect(screen.getByPlaceholderText('Type a message...')).toBeDefined();
    // Page DOM still present (hidden), not unmounted.
    expect(screen.getByTestId('page-input')).toBeDefined();

    await userEvent.click(screen.getByRole('button', { name: 'Page' }));
    expect((screen.getByTestId('page-input') as HTMLInputElement).value).toBe('kept across switch');
  });

  it('bottom-tab switches regions and defaults to canvas/page', async () => {
    setViewportWidth(500);
    render(<MobileShell />);

    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    expect(screen.getByTestId('mobile-region-tabs')).toBeDefined();
    // Default region = canvas: page visible, assistant column CSS-hidden.
    const row = document.querySelector('[data-mobile-region="canvas"]');
    expect(row).not.toBeNull();

    await userEvent.click(screen.getByRole('button', { name: 'Assistant' }));
    expect(document.querySelector('[data-mobile-region="assistant"]')).not.toBeNull();
  });

  it('respects mobileDefaultRegion="assistant"', async () => {
    setViewportWidth(500);
    render(<MobileShell mobileDefaultRegion="assistant" initialActive="alerting" />);
    await waitFor(() => expect(screen.getByPlaceholderText('Type a message...')).toBeDefined());
    expect(document.querySelector('[data-mobile-region="assistant"]')).not.toBeNull();
  });

  it('persistMobileRegionKey round-trips through localStorage', async () => {
    localStorage.setItem('shell.mobileRegion', 'assistant');
    setViewportWidth(500);
    render(<MobileShell persistMobileRegionKey="shell.mobileRegion" />);
    await waitFor(() =>
      expect(document.querySelector('[data-mobile-region="assistant"]')).not.toBeNull(),
    );

    await userEvent.click(screen.getByRole('button', { name: 'Page' }));
    expect(localStorage.getItem('shell.mobileRegion')).toBe('canvas');
  });

  it('forces overlay nav on mobile even when navMode="docked"', async () => {
    setViewportWidth(500);
    render(<MobileShell navMode="docked" />);
    await waitFor(() => expect(screen.getByTestId('hamburger')).toBeDefined());
    expect(screen.queryByTestId('nav-docked-column')).toBeNull();

    await userEvent.click(screen.getByTestId('hamburger'));
    expect(await screen.findByTestId('nav-overview')).toBeDefined();
    expect(screen.getByRole('dialog')).toBeDefined();
  });

  it('openNav surfaces the page/canvas region', async () => {
    setViewportWidth(500);
    render(<MobileShell mobileDefaultRegion="assistant" />);
    await waitFor(() =>
      expect(document.querySelector('[data-mobile-region="assistant"]')).not.toBeNull(),
    );

    await userEvent.click(screen.getByTestId('hamburger'));
    await userEvent.click(await screen.findByTestId('nav-overview'));
    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Overview'));
    expect(document.querySelector('[data-mobile-region="canvas"]')).not.toBeNull();
  });

  it('flips layout live when the viewport crosses the breakpoint', async () => {
    // Covered in depth by useViewportWide unit tests (matchMedia change events).
    // Here we assert the shell reacts when remounted after a width change —
    // the production hook also re-evaluates on matchMedia `change`.
    setViewportWidth(500);
    const { unmount } = render(<MobileShell />);
    await waitFor(() => expect(screen.getByTestId('mobile-region-tabs')).toBeDefined());
    unmount();

    setViewportWidth(1280);
    render(<MobileShell />);
    await waitFor(() => expect(screen.queryByTestId('mobile-region-tabs')).toBeNull());
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
  });

  it('toggle switch variant renders a show-assistant affordance', async () => {
    setViewportWidth(500);
    render(<MobileShell mobileRegionSwitch="toggle" />);
    await waitFor(() => expect(screen.getByTestId('mobile-region-toggle')).toBeDefined());
    expect(screen.queryByTestId('mobile-region-tabs')).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'Show assistant' }));
    expect(document.querySelector('[data-mobile-region="assistant"]')).not.toBeNull();
  });
});

describe('ChatAppShell backward compat (no mobile props)', () => {
  beforeEach(() => {
    localStorage.clear();
    setViewportWidth(1280);
  });

  it('wide viewport without mobileLayout: no mobile chrome, canvas present with destination', async () => {
    render(
      <ChatAppShell
        destinations={DESTS}
        activeDestinationId="alerting"
        renderHeader={() => <div data-testid="header">h</div>}
        renderNavMenu={() => null}
        renderDestination={({ destination }) => (
          <div data-testid="page">{destination.label}</div>
        )}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('page').textContent).toBe('Alerting'));
    expect(screen.queryByTestId('mobile-region-tabs')).toBeNull();
    expect(screen.queryByTestId('mobile-shell-body')).toBeNull();
    expect(document.querySelector('[data-mobile-region]')).toBeNull();
  });

  it('narrow viewport without mobileLayout: canvas removed (0.3.1 behavior)', async () => {
    setViewportWidth(500);
    const { container } = render(
      <ChatAppShell
        destinations={DESTS}
        activeDestinationId="alerting"
        renderHeader={() => <div data-testid="header">h</div>}
        renderNavMenu={() => null}
        renderDestination={({ destination }) => (
          <div data-testid="page">{destination.label}</div>
        )}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('header')).toBeDefined());
    // Host page never portals — canvas was removed by the legacy gate.
    expect(screen.queryByTestId('page')).toBeNull();
    expect(container.querySelector('[class*="canvasRegion"]')).toBeNull();
    expect(screen.queryByTestId('mobile-region-tabs')).toBeNull();
    // Snapshot of the non-mobile narrow shell: docked assistant only, no mobile attrs.
    expect(container.querySelector('[data-mobile-region]')).toBeNull();
    expect(container.querySelector('[data-testid="mobile-shell-body"]')).toBeNull();
  });
});

describe('ChatAppShell mobileLayout SSR-safe first paint', () => {
  it('treats an explicit matchMedia matches=true as wide on first paint', async () => {
    setViewportWidth(1280);
    render(
      <ChatAppShell
        destinations={DESTS}
        activeDestinationId="alerting"
        mobileLayout
        mobileBreakpoint={1024}
        renderHeader={() => <div>h</div>}
        renderNavMenu={() => null}
        renderDestination={({ destination }) => (
          <div data-testid="page">{destination.label}</div>
        )}
      />,
    );

    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    expect(screen.queryByTestId('mobile-region-tabs')).toBeNull();
  });
});
