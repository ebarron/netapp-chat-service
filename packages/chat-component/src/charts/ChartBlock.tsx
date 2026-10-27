import { Code } from '@mantine/core';
import { parseChart } from './chartTypes';
import './suppressRechartsWarning';
import { AreaChartBlock } from './AreaChartBlock';
import { BarChartBlock } from './BarChartBlock';
import { SparklineBlock } from './SparklineBlock';
import { GaugeBlock } from './GaugeBlock';
import { StatBlock } from './StatBlock';
import { StatusGridBlock } from './StatusGridBlock';
import { AlertSummaryBlock } from './AlertSummaryBlock';
import { ResourceTableBlock } from './ResourceTableBlock';
import { AlertListBlock } from './AlertListBlock';
import { CalloutBlock } from './CalloutBlock';
import { ProposalBlock } from './ProposalBlock';
import { ActionButtonBlock } from './ActionButtonBlock';
import { ActionFormBlock } from './ActionFormBlock';
import { PanelExtensionSlot } from '../PanelExtensions';
import type { CalloutData, PanelData } from './chartTypes';

interface ChartBlockProps {
  json: string;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
}

/**
 * Dispatches a standalone chart JSON to the correct renderer.
 * Falls back to a plain code block on parse failure.
 */
export function ChartBlock({ json, onAction, onExecute, readOnly }: ChartBlockProps) {
  const data = parseChart(json);
  if (!data) {
    return <Code block>{json}</Code>;
  }
  return <PanelRenderer data={data} onAction={onAction} onExecute={onExecute} readOnly={readOnly} />;
}

interface PanelRendererProps {
  data: PanelData;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
}

function ExtensionAwareCallout({
  data,
  onAction,
  readOnly,
}: {
  data: CalloutData;
  onAction?: (message: string) => void;
  readOnly?: boolean;
}) {
  // Keep opaque extension metadata out of the Markdown fallback renderer.
  const fallbackData: CalloutData = {
    type: 'callout',
    title: data.title,
    body: data.body,
    width: data.width,
    icon: data.icon,
  };
  const fallback = <CalloutBlock data={fallbackData} />;
  if (!data.extension) return fallback;
  return (
    <PanelExtensionSlot
      extension={data.extension}
      fallback={fallback}
      onAction={onAction}
      readOnly={readOnly}
    />
  );
}

/** Renders a single panel by type. Exported for use by DashboardBlock too. */
export function PanelRenderer({ data, onAction, onExecute, readOnly }: PanelRendererProps) {
  switch (data.type) {
    case 'area':
      return <AreaChartBlock data={data} />;
    case 'bar':
      return <BarChartBlock data={data} />;
    case 'sparkline':
      return <SparklineBlock data={data} />;
    case 'gauge':
      return <GaugeBlock data={data} />;
    case 'stat':
      return <StatBlock data={data} />;
    case 'status-grid':
      return <StatusGridBlock data={data} />;
    case 'alert-summary':
      return <AlertSummaryBlock data={data} onAction={onAction} />;
    case 'resource-table':
      return <ResourceTableBlock data={data} onAction={onAction} />;
    case 'alert-list':
      return <AlertListBlock data={data} onAction={onAction} />;
    case 'callout':
      return (
        <ExtensionAwareCallout
          data={data}
          onAction={onAction}
          readOnly={readOnly}
        />
      );
    case 'proposal':
      return <ProposalBlock data={data} />;
    case 'action-button':
      return <ActionButtonBlock data={data} onAction={onAction} onExecute={onExecute} readOnly={readOnly} />;
    case 'action-form':
      return <ActionFormBlock data={data} onAction={onAction} readOnly={readOnly} />;
    default:
      return null;
  }
}
