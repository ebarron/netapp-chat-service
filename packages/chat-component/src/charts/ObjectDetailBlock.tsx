import { Code, Paper, Text, Group, Badge, Stack, Divider } from '@mantine/core';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { parseObjectDetail } from './chartTypes';
import { PanelRenderer } from './ChartBlock';
import { ActionButtonBlock } from './ActionButtonBlock';
import { PropertiesSection } from './PropertiesSection';
import { TimelineSection } from './TimelineSection';
import type {
  ObjectDetailSection,
  PropertiesData,
  TimelineData,
  AlertListData,
  ActionButtonData,
  ResourceTableData,
  PanelData,
} from './chartTypes';

interface ObjectDetailBlockProps {
  json: string;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
}

/**
 * Append an identity qualifier to a message when not already present.
 * Used to ensure follow-up prompts carry enough context to uniquely identify a resource.
 *
 * @param itemQualifier - Per-item override. Use "" to suppress qualifier entirely.
 *                        When undefined, falls back to cardQualifier.
 * @param cardQualifier - Card-level qualifier from the object-detail block.
 */
export function applyQualifier(
  message: string,
  itemQualifier: string | undefined,
  cardQualifier: string | undefined,
): string {
  // Per-item qualifier takes precedence. Empty string means "no qualifier needed".
  const qualifier = itemQualifier !== undefined ? itemQualifier : cardQualifier;
  if (!qualifier) return message;
  if (message.toLowerCase().includes(qualifier.toLowerCase())) return message;
  return `${message} ${qualifier}`;
}

/**
 * Wrap onAction to append the card-level qualifier when not already present.
 * Used for sections that don't support per-item qualifiers (chart, timeline, etc.).
 */
function enrichOnAction(
  onAction: ((message: string) => void) | undefined,
  qualifier: string | undefined,
): ((message: string) => void) | undefined {
  if (!onAction || !qualifier) return onAction;
  return (message: string) => {
    onAction(applyQualifier(message, undefined, qualifier));
  };
}

const statusColors: Record<string, string> = {
  critical: 'red',
  warning: 'yellow',
  ok: 'green',
  info: 'blue',
};

function SectionRenderer({
  section,
  onAction,
  onExecute,
  readOnly,
  cardQualifier,
}: {
  section: ObjectDetailSection;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
  cardQualifier?: string;
}) {
  const { layout, data } = section;

  switch (layout) {
    case 'properties':
      return <PropertiesSection data={data as PropertiesData} onAction={onAction} cardQualifier={cardQualifier} />;

    case 'chart': {
      const chartData = data as Record<string, unknown>;
      return (
        <PanelRenderer
          data={chartData as unknown as PanelData}
          onAction={onAction}
          onExecute={onExecute}
          readOnly={readOnly}
        />
      );
    }

    case 'alert-list':
      return (
        <PanelRenderer
          data={{ type: 'alert-list', ...(data as object) } as AlertListData}
          onAction={onAction}
        />
      );

    case 'timeline':
      return <TimelineSection data={data as TimelineData} onAction={onAction} />;

    case 'actions':
      return (
        <ActionButtonBlock
          data={{ type: 'action-button', ...(data as object) } as ActionButtonData}
          onAction={onAction}
          onExecute={onExecute}
          readOnly={readOnly}
          cardQualifier={cardQualifier}
        />
      );

    case 'text': {
      const body = (data as { body?: string }).body ?? '';
      return (
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {body}
        </ReactMarkdown>
      );
    }

    case 'table':
      return (
        <PanelRenderer
          data={{ type: 'resource-table', ...(data as object) } as ResourceTableData}
          onAction={onAction}
        />
      );

    default:
      return null;
  }
}

export function ObjectDetailBlock({ json, onAction, onExecute, readOnly }: ObjectDetailBlockProps) {
  const detail = parseObjectDetail(json);
  if (!detail) {
    return <Code block>{json}</Code>;
  }

  const badgeColor = statusColors[detail.status ?? ''] ?? 'gray';
  // For sections that support per-item qualifiers (properties, actions),
  // we pass raw onAction + cardQualifier so they can apply per-item overrides.
  // For other sections, we use enrichedOnAction which always applies the card qualifier.
  const enrichedOnAction = enrichOnAction(onAction, detail.qualifier);

  return (
    <Paper p="md" radius="md" withBorder role="article" aria-label={detail.name}>
      {/* Identity header */}
      <Group gap="xs" mb={4}>
        {detail.status && (
          <Badge size="sm" color={badgeColor} variant="filled">
            {detail.status}
          </Badge>
        )}
        <Text fw={700} fz="lg">
          {detail.name}
        </Text>
      </Group>
      {detail.subtitle && (
        <Text fz="sm" c="dimmed" mb="sm">
          {detail.subtitle}
        </Text>
      )}

      {/* Sections */}
      <Stack gap="md">
        {detail.sections.map((section, i) => (
          <div key={i}>
            {i > 0 && <Divider mb="sm" />}
            <Text fw={600} fz="sm" mb="xs">
              {section.title}
            </Text>
            <SectionRenderer
              section={section}
              onAction={section.layout === 'properties' || section.layout === 'actions' ? onAction : enrichedOnAction}
              onExecute={onExecute}
              readOnly={readOnly}
              cardQualifier={detail.qualifier}
            />
          </div>
        ))}
      </Stack>
    </Paper>
  );
}
