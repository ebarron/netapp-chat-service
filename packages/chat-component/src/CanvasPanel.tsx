import { Tabs, CloseButton, ScrollArea, Text } from '@mantine/core';
import { DashboardBlock, ObjectDetailBlock } from './charts';
import type { CanvasTab } from './useChatPanel';
import classes from './ChatPanel.module.css';

interface CanvasPanelProps {
  tabs: CanvasTab[];
  activeTab: string | null;
  onTabChange: (tabId: string) => void;
  onTabClose: (tabId: string) => void;
  onAction?: (message: string) => void;
  readOnly?: boolean;
}

export function CanvasPanel({
  tabs,
  activeTab,
  onTabChange,
  onTabClose,
  onAction,
  readOnly,
}: CanvasPanelProps) {
  if (tabs.length === 0) return null;

  // Deduplicate tabs by tabId to avoid React key collisions.
  const uniqueTabs = tabs.filter(
    (tab, i, arr) => arr.findIndex((t) => t.tabId === tab.tabId) === i,
  );

  return (
    <div className={classes.canvasRegion}>
      <Tabs
        value={activeTab ?? undefined}
        onChange={(v) => v && onTabChange(v)}
        variant="outline"
        classNames={{ root: classes.canvasTabs }}
      >
        <Tabs.List>
          {uniqueTabs.map((tab) => (
            <Tabs.Tab
              key={tab.tabId}
              value={tab.tabId}
              rightSection={
                <CloseButton
                  size="xs"
                  onClick={(e) => {
                    e.stopPropagation();
                    onTabClose(tab.tabId);
                  }}
                  aria-label={`Close ${tab.title}`}
                />
              }
            >
              <Text size="xs" truncate maw={120}>
                {tab.title}
              </Text>
            </Tabs.Tab>
          ))}
        </Tabs.List>

        {uniqueTabs.map((tab) => (
          <Tabs.Panel key={tab.tabId} value={tab.tabId} style={{ flex: 1, minHeight: 0 }}>
            <ScrollArea style={{ height: '100%' }} p="sm">
              <CanvasTabContent tab={tab} onAction={onAction} readOnly={readOnly} />
            </ScrollArea>
          </Tabs.Panel>
        ))}
      </Tabs>
    </div>
  );
}

function CanvasTabContent({
  tab,
  onAction,
  readOnly,
}: {
  tab: CanvasTab;
  onAction?: (message: string) => void;
  readOnly?: boolean;
}) {
  const json = JSON.stringify(tab.content);
  const content = tab.content;

  // Dispatch to the appropriate renderer based on content type.
  if (content.type === 'object-detail' || content.kind) {
    return <ObjectDetailBlock json={json} onAction={onAction} readOnly={readOnly} />;
  }
  if (Array.isArray(content.panels)) {
    return <DashboardBlock json={json} onAction={onAction} readOnly={readOnly} />;
  }

  // Fallback: try object-detail first, then dashboard.
  return <ObjectDetailBlock json={json} onAction={onAction} readOnly={readOnly} />;
}
