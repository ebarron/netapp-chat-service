import { render, screen } from '../test-utils';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { CanvasPanel } from './CanvasPanel';
import type { CanvasTab } from './useChatPanel';

// Mock the chart components so we don't need full chart rendering.
vi.mock('./charts', () => ({
  ObjectDetailBlock: ({ json }: { json: string }) => (
    <div data-testid="object-detail">{json}</div>
  ),
  DashboardBlock: ({ json }: { json: string }) => (
    <div data-testid="dashboard">{json}</div>
  ),
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
});
