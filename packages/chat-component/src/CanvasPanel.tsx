import { Tabs, CloseButton, ScrollArea, Text } from '@mantine/core';
import { useEffect, useRef } from 'react';
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
  /**
   * Called with the portal mount node for a host-content tab (C2–C4) when it
   * mounts (`el`) and unmounts (`null`). The host renders its own page into
   * `el` via `ReactDOM.createPortal`. The component never imports host pages —
   * it only exposes this hole.
   */
  onHostTabPortal?: (tabId: string, el: HTMLElement | null) => void;
  /**
   * When true, hide the canvas tab strip (`Tabs.List`) while exactly one tab is
   * open. The tab's panel content still renders. With two or more tabs the strip
   * is shown as usual. Defaults to `false` (strip always shown when ≥1 tab).
   */
  hideSingleTab?: boolean;
}

export function CanvasPanel({
  tabs,
  activeTab,
  onTabChange,
  onTabClose,
  onAction,
  readOnly,
  onHostTabPortal,
  hideSingleTab = false,
}: CanvasPanelProps) {
  if (tabs.length === 0) return null;

  // Deduplicate tabs by tabId to avoid React key collisions.
  const uniqueTabs = tabs.filter(
    (tab, i, arr) => arr.findIndex((t) => t.tabId === tab.tabId) === i,
  );

  const showTabList = !(hideSingleTab && uniqueTabs.length === 1);

  return (
    <div className={classes.canvasRegion}>
      <Tabs
        value={activeTab ?? undefined}
        onChange={(v) => v && onTabChange(v)}
        variant="outline"
        classNames={{ root: classes.canvasTabs }}
      >
        {showTabList && (
          <Tabs.List>
            {uniqueTabs.map((tab) => (
              <Tabs.Tab
                key={tab.tabId}
                value={tab.tabId}
                rightSection={
                  <CloseButton
                    component="span"
                    role="button"
                    tabIndex={0}
                    size="xs"
                    onClick={(e) => {
                      e.stopPropagation();
                      onTabClose(tab.tabId);
                    }}
                    onKeyDown={(e) => {
                      if (e.key !== 'Enter' && e.key !== ' ') return;
                      e.preventDefault();
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
        )}

        {uniqueTabs.map((tab) =>
          tab.host ? (
            // Host-content tab (C2–C4): the component renders an empty mount
            // node and hands it to the host, which portals its own React tree
            // in. No ScrollArea wrapper — the host owns the body's layout.
            <Tabs.Panel key={tab.tabId} value={tab.tabId} style={{ flex: 1, minHeight: 0 }}>
              <HostTabMount tabId={tab.tabId} onHostTabPortal={onHostTabPortal} />
            </Tabs.Panel>
          ) : (
            <Tabs.Panel key={tab.tabId} value={tab.tabId} style={{ flex: 1, minHeight: 0 }}>
              <ScrollArea style={{ height: '100%' }} p="sm">
                <CanvasTabContent tab={tab} onAction={onAction} readOnly={readOnly} />
              </ScrollArea>
            </Tabs.Panel>
          ),
        )}
      </Tabs>
    </div>
  );
}

/**
 * Empty mount node for a host-content tab. Reports its DOM element to the host
 * via `onHostTabPortal` on mount and `null` on unmount, so the host can
 * `createPortal` its page into it. The component never renders host content
 * itself.
 */
function HostTabMount({
  tabId,
  onHostTabPortal,
}: {
  tabId: string;
  onHostTabPortal?: (tabId: string, el: HTMLElement | null) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    onHostTabPortal?.(tabId, ref.current);
    return () => onHostTabPortal?.(tabId, null);
    // Re-run only if the tab identity or callback changes.
  }, [tabId, onHostTabPortal]);
  return <div ref={ref} data-canvas-host-tab={tabId} style={{ height: '100%', minHeight: 0 }} />;
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
  //
  // `key={json}` remounts the renderer whenever the tab's content is replaced
  // (e.g. an in-place re-render after an action). This resets transient form
  // state — action-form text inputs return to their defaults instead of
  // retaining what the user typed for the previous render, and avoids
  // index-keyed panel reuse bleeding one form's input into another.
  if (content.type === 'object-detail' || content.kind) {
    return <ObjectDetailBlock key={json} json={json} onAction={onAction} readOnly={readOnly} />;
  }
  if (Array.isArray(content.panels)) {
    return <DashboardBlock key={json} json={json} onAction={onAction} readOnly={readOnly} />;
  }

  // Fallback: try object-detail first, then dashboard.
  return <ObjectDetailBlock key={json} json={json} onAction={onAction} readOnly={readOnly} />;
}
