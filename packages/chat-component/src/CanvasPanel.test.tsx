import { render, screen } from '../test-utils';
import userEvent from '@testing-library/user-event';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { useEffect } from 'react';
import { CanvasPanel } from './CanvasPanel';
import type { CanvasTab } from './useChatPanel';

// Spy invoked once per mount of a mocked renderer, so tests can assert that a
// content change remounts (rather than re-renders) the renderer.
const mountSpy = vi.hoisted(() => vi.fn());

// Mock the chart components so we don't need full chart rendering.
vi.mock('./charts', () => ({
  ObjectDetailBlock: ({ json }: { json: string }) => {
    useEffect(() => {
      mountSpy('object-detail');
    }, []);
    return <div data-testid="object-detail">{json}</div>;
  },
  DashboardBlock: ({ json }: { json: string }) => {
    useEffect(() => {
      mountSpy('dashboard');
    }, []);
    return <div data-testid="dashboard">{json}</div>;
  },
}));

const volumeTab: CanvasTab = {
  tabId: 'volume::vol1::on SVM svm1',
  title: 'vol1',
  kind: 'volume',
  qualifier: 'on SVM svm1',
  content: { type: 'object-detail', kind: 'volume', name: 'vol1', sections: [] },
};

const clusterTab: CanvasTab = {
  tabId: 'cluster::cls1::',
  title: 'cls1',
  kind: 'cluster',
  qualifier: '',
  content: { type: 'object-detail', kind: 'cluster', name: 'cls1', sections: [] },
};

const dashboardTab: CanvasTab = {
  tabId: 'dashboard::Provision Plan::',
  title: 'Provision Plan',
  kind: 'dashboard',
  qualifier: '',
  content: { title: 'Provision Plan', panels: [] },
};

describe('CanvasPanel', () => {
  const onTabChange = vi.fn();
  const onTabClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders nothing when tabs are empty', () => {
    render(
      <CanvasPanel
        tabs={[]}
        activeTab={null}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    // No tab content rendered — only Mantine global styles exist.
    expect(screen.queryByRole('tab')).toBeNull();
  });

  it('renders tabs with titles', () => {
    render(
      <CanvasPanel
        tabs={[volumeTab, clusterTab]}
        activeTab={volumeTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(screen.getByText('vol1')).toBeDefined();
    expect(screen.getByText('cls1')).toBeDefined();
  });

  it('renders object-detail content for active tab', () => {
    render(
      <CanvasPanel
        tabs={[volumeTab]}
        activeTab={volumeTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(screen.getByTestId('object-detail')).toBeDefined();
  });

  it('renders dashboard content when content has panels', () => {
    render(
      <CanvasPanel
        tabs={[dashboardTab]}
        activeTab={dashboardTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(screen.getByTestId('dashboard')).toBeDefined();
  });

  it('has close buttons for each tab', () => {
    render(
      <CanvasPanel
        tabs={[volumeTab, clusterTab]}
        activeTab={volumeTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(screen.getByLabelText('Close vol1')).toBeDefined();
    expect(screen.getByLabelText('Close cls1')).toBeDefined();
  });

  it('does not nest native buttons inside tab buttons', () => {
    render(
      <CanvasPanel
        tabs={[volumeTab, clusterTab]}
        activeTab={volumeTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );

    for (const tab of screen.getAllByRole('tab')) {
      expect(tab.tagName).toBe('BUTTON');
      expect(tab.querySelectorAll('button')).toHaveLength(0);
    }

    const closeVol1 = screen.getByLabelText('Close vol1');
    expect(closeVol1.tagName).toBe('SPAN');
    expect(closeVol1).toHaveAttribute('role', 'button');
  });

  it('keeps tab close separate from tab activation', async () => {
    const user = userEvent.setup();
    render(
      <CanvasPanel
        tabs={[volumeTab, clusterTab]}
        activeTab={volumeTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );

    await user.click(screen.getByLabelText('Close vol1'));
    expect(onTabClose).toHaveBeenCalledWith(volumeTab.tabId);
    expect(onTabChange).not.toHaveBeenCalled();

    onTabClose.mockClear();
    await user.click(screen.getByRole('tab', { name: /cls1/i }));
    expect(onTabChange).toHaveBeenCalledWith(clusterTab.tabId);
    expect(onTabClose).not.toHaveBeenCalled();
  });

  it('remounts the renderer when a tab\'s content is replaced (resets form state)', () => {
    const { rerender } = render(
      <CanvasPanel
        tabs={[dashboardTab]}
        activeTab={dashboardTab.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(mountSpy).toHaveBeenCalledTimes(1);

    // Same tabId, new content (an in-place re-render after an action).
    const updated: CanvasTab = {
      ...dashboardTab,
      content: { title: 'Provision Plan', panels: [{ type: 'callout', title: 'x', body: 'y' }] },
    };
    rerender(
      <CanvasPanel
        tabs={[updated]}
        activeTab={updated.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    // key={json} changed → renderer remounted (not just re-rendered).
    expect(mountSpy).toHaveBeenCalledTimes(2);

    // Re-rendering with identical content does NOT remount.
    rerender(
      <CanvasPanel
        tabs={[updated]}
        activeTab={updated.tabId}
        onTabChange={onTabChange}
        onTabClose={onTabClose}
      />
    );
    expect(mountSpy).toHaveBeenCalledTimes(2);
  });

  describe('hideSingleTab', () => {
    it('default (prop off) + 1 tab still shows the tablist', () => {
      render(
        <CanvasPanel
          tabs={[volumeTab]}
          activeTab={volumeTab.tabId}
          onTabChange={onTabChange}
          onTabClose={onTabClose}
        />,
      );
      expect(screen.getByRole('tablist')).toBeDefined();
      expect(screen.getByRole('tab', { name: /vol1/i })).toBeDefined();
      expect(screen.getByTestId('object-detail')).toBeDefined();
    });

    it('hides the tablist with exactly one tab but still shows content', () => {
      render(
        <CanvasPanel
          tabs={[volumeTab]}
          activeTab={volumeTab.tabId}
          onTabChange={onTabChange}
          onTabClose={onTabClose}
          hideSingleTab
        />,
      );
      expect(screen.queryByRole('tablist')).toBeNull();
      expect(screen.queryByRole('tab')).toBeNull();
      expect(screen.getByTestId('object-detail')).toBeDefined();
    });

    it('shows the tablist once two tabs are open', () => {
      render(
        <CanvasPanel
          tabs={[volumeTab, clusterTab]}
          activeTab={volumeTab.tabId}
          onTabChange={onTabChange}
          onTabClose={onTabClose}
          hideSingleTab
        />,
      );
      expect(screen.getByRole('tablist')).toBeDefined();
      expect(screen.getByRole('tab', { name: /vol1/i })).toBeDefined();
      expect(screen.getByRole('tab', { name: /cls1/i })).toBeDefined();
    });
  });
});
