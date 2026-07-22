import { useState } from 'react';
import { render, screen, waitFor, act } from '../test-utils';
import userEvent from '@testing-library/user-event';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { ChatAppShell, type ChatAppShellDestination } from './ChatAppShell';

// Generic (non-NABox) destinations prove the shell is app-agnostic.
const DESTS: ChatAppShellDestination[] = [
  { id: 'overview', label: 'Overview', route: '/overview' },
  { id: 'alerting', label: 'Alerting', route: '/alerting' },
  { id: 'network', label: 'Network', route: '/settings/network' },
];

beforeEach(() => {
  // Canvas region only renders on wide viewports (>=1024px).
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280 });
  vi.clearAllMocks();
});

function Harness({
  initialActive = null,
  onActiveChange,
}: {
  initialActive?: string | null;
  onActiveChange?: (id: string) => void;
}) {
  const [active, setActive] = useState<string | null>(initialActive);
  return (
    <ChatAppShell
      destinations={DESTS}
      activeDestinationId={active}
      onActiveDestinationChange={(d) => {
        onActiveChange?.(d.id);
        setActive(d.id);
      }}
      buildSummary={(d) => ({ kind: 'nav-view', name: d.label, qualifier: d.route })}
      renderHeader={({ toggleNav }) => (
        <button type="button" data-testid="hamburger" onClick={toggleNav}>
          menu
        </button>
      )}
      renderNavMenu={({ openNav }) => (
        <div>
          {DESTS.map((d) => (
            <button key={d.id} type="button" data-testid={`nav-${d.id}`} onClick={() => openNav(d.id)}>
              {d.label}
            </button>
          ))}
          <button type="button" data-testid="nav-by-route" onClick={() => openNav('/alerting')}>
            byroute
          </button>
        </div>
      )}
      renderDestination={({ destination, publishSummary }) => (
        <div data-testid="page">
          <span data-testid="page-label">{destination.label}</span>
          <button type="button" data-testid="publish" onClick={() => publishSummary({ digest: 'live' })}>
            publish
          </button>
        </div>
      )}
    />
  );
}

async function openMenu() {
  await userEvent.click(screen.getByTestId('hamburger'));
}

describe('ChatAppShell (generic docked shell)', () => {
  it('renders the header slot and shows no destination page until one is opened', async () => {
    render(<Harness />);
    expect(await screen.findByTestId('hamburger')).toBeDefined();
    // Greeting state: no reserved nav tab, no host page.
    expect(screen.queryByTestId('page')).toBeNull();
  });

  it('opening a destination portals the host page and syncs the URL (one tab)', async () => {
    const onActiveChange = vi.fn();
    render(<Harness onActiveChange={onActiveChange} />);
    await openMenu();
    await userEvent.click(screen.getByTestId('nav-alerting'));

    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Alerting'));
    expect(onActiveChange).toHaveBeenCalledWith('alerting');
    // Exactly one reserved nav tab in the canvas strip.
    expect(screen.getAllByRole('tab')).toHaveLength(1);
  });

  it('open-or-replace: selecting another destination replaces the single nav tab', async () => {
    render(<Harness />);
    await openMenu();
    await userEvent.click(screen.getByTestId('nav-alerting'));
    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Alerting'));

    await openMenu();
    await userEvent.click(screen.getByTestId('nav-network'));
    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Network'));
    // Still ONE tab — never accumulate.
    expect(screen.getAllByRole('tab')).toHaveLength(1);
  });

  it('resolves an open target by route as well as by id', async () => {
    render(<Harness />);
    await openMenu();
    await userEvent.click(screen.getByTestId('nav-by-route'));
    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Alerting'));
  });

  it('a controlled activeDestinationId (deep link) opens the tab on mount', async () => {
    render(<Harness initialActive="network" />);
    await waitFor(() => expect(screen.getByTestId('page-label').textContent).toBe('Network'));
    expect(screen.getAllByRole('tab')).toHaveLength(1);
  });

  it('manual close of the nav tab returns to greeting and does NOT auto-reopen', async () => {
    render(<Harness />);
    await openMenu();
    await userEvent.click(screen.getByTestId('nav-alerting'));
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());

    // Close the reserved nav tab via its tab close button.
    await userEvent.click(screen.getByRole('button', { name: /Close Alerting/i }));

    // Page is gone and stays gone (activeDestinationId unchanged ⇒ no reopen).
    await waitFor(() => expect(screen.queryByTestId('page')).toBeNull());
    // Give the effect a chance to (incorrectly) refire.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(screen.queryByTestId('page')).toBeNull();
  });

  it('exposes a working publishSummary to the destination page (no crash)', async () => {
    render(<Harness initialActive="alerting" />);
    await waitFor(() => expect(screen.getByTestId('publish')).toBeDefined());
    await userEvent.click(screen.getByTestId('publish'));
    // Page still present after publishing a live summary.
    expect(screen.getByTestId('page-label').textContent).toBe('Alerting');
  });

  it('forwards resizableAssistant to the docked split handle', async () => {
    render(
      <ChatAppShell
        destinations={DESTS}
        activeDestinationId="alerting"
        renderHeader={() => <div>header</div>}
        renderNavMenu={() => null}
        renderDestination={({ destination }) => (
          <div data-testid="page">{destination.label}</div>
        )}
        resizableAssistant
      />,
    );
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    expect(screen.getByTestId('assistant-split-handle')).toBeDefined();
  });

  it('forwards assistantWidth to the split row CSS variable', async () => {
    const { container } = render(
      <ChatAppShell
        destinations={DESTS}
        activeDestinationId="alerting"
        assistantWidth={512}
        renderHeader={() => <div>header</div>}
        renderNavMenu={() => null}
        renderDestination={({ destination }) => (
          <div data-testid="page">{destination.label}</div>
        )}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('512px');
  });
});
