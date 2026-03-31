import { Text, Code, Badge, Button, Group } from '@mantine/core';
import { parseDashboard } from './chartTypes';
import type { PanelData, PanelWidth, ResourceTableData } from './chartTypes';
import { PanelRenderer } from './ChartBlock';
import classes from './DashboardBlock.module.css';

interface DashboardBlockProps {
  json: string;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
}

function spanForWidth(width?: PanelWidth): number {
  switch (width) {
    case 'third':
      return 1;
    case 'half':
      return 2;
    case 'full':
    default:
      return 4;
  }
}

/**
 * Renders a composite dashboard block with a title and a responsive CSS Grid of panels.
 * Uses a 4-column grid that collapses to 1 column on narrow viewports.
 * Falls back to a plain code block on parse failure.
 */
export function DashboardBlock({ json, onAction, onExecute, readOnly }: DashboardBlockProps) {
  const dashboard = parseDashboard(json);
  if (!dashboard) {
    return <Code block>{json}</Code>;
  }

  // Auto-inject pagination when LLM omits the action-button panel
  const hasActionButton = dashboard.panels.some((p: PanelData) => p.type === 'action-button');
  const resourceTable = dashboard.panels.find((p: PanelData) => p.type === 'resource-table') as ResourceTableData | undefined;
  const rowCount = resourceTable?.rows?.length ?? 0;
  const showAutoPagination = !hasActionButton && rowCount >= 10 && rowCount % 5 === 0;

  return (
    <div className={classes.dashboard} role="region" aria-label={dashboard.title}>
      <div className={classes.titleRow}>
        <Text fw={600} fz="md">
          {dashboard.title}
        </Text>
        {dashboard.toggle && onAction && (
          <Badge
            variant="light"
            size="sm"
            style={{ cursor: 'pointer' }}
            onClick={() => onAction(dashboard.toggle!.message)}
          >
            {dashboard.toggle.label}
          </Badge>
        )}
      </div>
      <div className={classes.panelGrid}>
        {dashboard.panels.map((panel, pi) => {
          const span = spanForWidth(panel.width);
          return (
            <div key={pi} style={{ gridColumn: `span ${span}` }}>
              <PanelRenderer
                data={panel}
                onAction={onAction}
                onExecute={onExecute}
                readOnly={readOnly}
              />
            </div>
          );
        })}
      </div>
      {showAutoPagination && onAction && (
        <Group gap="xs" mt="xs">
          <Button
            size="compact-sm"
            variant="filled"
            onClick={() => onAction(`Show me the next ${rowCount} results`)}
          >
            Show next {rowCount}
          </Button>
        </Group>
      )}
    </div>
  );
}
