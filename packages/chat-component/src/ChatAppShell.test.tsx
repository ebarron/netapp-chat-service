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
  localStorage.clear();
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

/** Shell with a hamburger + nav menu marker, used by C7/C8 layout-mode tests. */
function LayoutShell(
  props: Partial<Parameters<typeof ChatAppShell>[0]> = {},
) {
  return (
    <ChatAppShell
      destinations={DESTS}
      renderHeader={({ toggleNav }) => (
        <button type="button" data-testid="hamburger" onClick={toggleNav}>
          menu
        </button>
      )}
      renderNavMenu={() => <div data-testid="nav-menu">menu-tree</div>}
      renderDestination={({ destination }) => (
        <div data-testid="page">{destination.label}</div>
      )}
      {...props}
    />
  );
}

describe('ChatAppShell C7–C8 layout modes', () => {
  it('default path (no new props): overlay nav + assistant-left, no docked/collapse DOM', async () => {
    const { container } = render(<LayoutShell />);
    expect(await screen.findByTestId('hamburger')).toBeDefined();

    // No docked nav column, no placement reordering, no collapse affordances.
    expect(screen.queryByTestId('nav-docked-column')).toBeNull();
    expect(container.querySelector('[class*="assistantPlacementEnd"]')).toBeNull();
    expect(screen.queryByTestId('assistant-reveal-rail')).toBeNull();
    expect(screen.queryByTestId('assistant-hide-button')).toBeNull();

    // Overlay nav is closed until the hamburger opens the Drawer.
    expect(screen.queryByTestId('nav-menu')).toBeNull();
    await userEvent.click(screen.getByTestId('hamburger'));
    expect(await screen.findByTestId('nav-menu')).toBeDefined();
  });

  it('C7 docked nav renders a persistent column and mounts no Drawer', async () => {
    render(<LayoutShell navMode="docked" />);
    // Column is present (expanded) on mount; menu lives inside it.
    const column = await screen.findByTestId('nav-docked-column');
    expect(column.contains(screen.getByTestId('nav-menu'))).toBe(true);
    // No overlay Drawer dialog is ever mounted in docked mode.
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('C7 hamburger toggleNav collapses/expands the docked column', async () => {
    render(<LayoutShell navMode="docked" />);
    expect(await screen.findByTestId('nav-docked-column')).toBeDefined();

    await userEvent.click(screen.getByTestId('hamburger'));
    expect(screen.queryByTestId('nav-docked-column')).toBeNull();

    await userEvent.click(screen.getByTestId('hamburger'));
    expect(await screen.findByTestId('nav-docked-column')).toBeDefined();
  });

  it('C7 docked openNav keeps the column open (does not force-close)', async () => {
    render(
      <LayoutShell
        navMode="docked"
        activeDestinationId="alerting"
        renderNavMenu={({ openNav }) => (
          <button type="button" data-testid="nav-alerting" onClick={() => openNav('overview')}>
            go
          </button>
        )}
      />,
    );
    const column = await screen.findByTestId('nav-docked-column');
    await userEvent.click(screen.getByTestId('nav-alerting'));
    // Column still present after a selection.
    expect(column.isConnected).toBe(true);
    expect(screen.getByTestId('nav-docked-column')).toBeDefined();
  });

  it('C7 navDockedWidth sizes the column (defaults to navOverlayWidth)', async () => {
    const { rerender } = render(<LayoutShell navMode="docked" />);
    let column = await screen.findByTestId('nav-docked-column');
    expect(column.style.flex).toContain('260px');

    rerender(<LayoutShell navMode="docked" navDockedWidth={320} />);
    column = await screen.findByTestId('nav-docked-column');
    expect(column.style.flex).toContain('320px');
  });

  it('C8a assistantPlacement="end" reorders the split; width stays applied', async () => {
    const { container } = render(
      <LayoutShell activeDestinationId="alerting" assistantPlacement="end" assistantWidth={400} />,
    );
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    expect(container.querySelector('[class*="assistantPlacementEnd"]')).not.toBeNull();
    const row = container.querySelector('[class*="drawerBody"]') as HTMLElement;
    expect(row.style.getPropertyValue('--chat-assistant-width').trim()).toBe('400px');
  });

  it('C8b uncontrolled collapse shows a reveal rail with documented roles', async () => {
    render(<LayoutShell activeDestinationId="alerting" defaultAssistantCollapsed />);
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());

    const rail = screen.getByRole('button', { name: 'Show assistant' });
    expect(rail.getAttribute('aria-expanded')).toBe('false');

    // Reveal expands (uncontrolled): rail disappears.
    await userEvent.click(rail);
    expect(screen.queryByTestId('assistant-reveal-rail')).toBeNull();

    // Hide affordance collapses again.
    await userEvent.click(screen.getByRole('button', { name: 'Hide assistant' }));
    expect(screen.getByTestId('assistant-reveal-rail')).toBeDefined();
  });

  it('C8b controlled collapse fires onAssistantCollapsedChange without self-updating', async () => {
    const onChange = vi.fn();
    render(
      <LayoutShell
        activeDestinationId="alerting"
        assistantCollapsed
        onAssistantCollapsedChange={onChange}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());

    await userEvent.click(screen.getByRole('button', { name: 'Show assistant' }));
    expect(onChange).toHaveBeenCalledWith(false);
    // Controlled: still collapsed because the parent didn't change the prop.
    expect(screen.getByTestId('assistant-reveal-rail')).toBeDefined();
  });

  it('C8b persistAssistantCollapsedKey round-trips through localStorage', async () => {
    localStorage.setItem('shell.collapsed', '1');
    const { unmount } = render(
      <LayoutShell activeDestinationId="alerting" persistAssistantCollapsedKey="shell.collapsed" />,
    );
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());
    // Restored collapsed from storage.
    expect(await screen.findByTestId('assistant-reveal-rail')).toBeDefined();

    // Expanding writes the new value back.
    await userEvent.click(screen.getByRole('button', { name: 'Show assistant' }));
    expect(localStorage.getItem('shell.collapsed')).toBe('0');
    unmount();
    localStorage.removeItem('shell.collapsed');
  });

  it('C8b collapse preserves chat state (ChatPanel is not unmounted)', async () => {
    render(<LayoutShell activeDestinationId="alerting" defaultAssistantCollapsed={false} />);
    await waitFor(() => expect(screen.getByTestId('page')).toBeDefined());

    const input = screen.getByPlaceholderText('Type a message...') as HTMLTextAreaElement;
    await userEvent.type(input, 'draft in flight');
    expect(input.value).toBe('draft in flight');

    // Collapse, then expand — the same input keeps its draft (no remount).
    await userEvent.click(screen.getByRole('button', { name: 'Hide assistant' }));
    await userEvent.click(screen.getByRole('button', { name: 'Show assistant' }));
    expect((screen.getByPlaceholderText('Type a message...') as HTMLTextAreaElement).value).toBe(
      'draft in flight',
    );
  });
});
