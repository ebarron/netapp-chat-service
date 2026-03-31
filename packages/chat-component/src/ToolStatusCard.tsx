import { useState } from 'react';
import { Badge, Group, Paper, Text, UnstyledButton } from '@mantine/core';
import { IconBolt, IconChartLine, IconCode } from '@tabler/icons-react';
import type { ChatMessage } from './useChatPanel';
import { SparklineBlock } from './charts/SparklineBlock';
import { GaugeBlock } from './charts/GaugeBlock';
import type { SparklineData, GaugeData } from './charts/chartTypes';
import classes from './ChatPanel.module.css';

/** Result of auto-detecting a visualization from a tool result string. */
export type DetectedViz =
  | { kind: 'sparkline'; data: SparklineData }
  | { kind: 'gauge'; data: GaugeData }
  | null;

/** Check if a value looks like a timestamp (ISO string or key name hint). */
function isTimestampKey(key: string): boolean {
  return /^(time|date|timestamp|created|updated|collected)(_at|stamp)?$/i.test(key);
}

/**
 * Detect whether a tool result string contains data suitable for automatic
 * visualization as a sparkline or gauge. Returns null when no pattern matches.
 */
export function detectToolViz(raw: string | undefined): DetectedViz {
  if (!raw) return null;

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }

  // --- Gauge: single object with value + max ---
  if (
    typeof parsed === 'object' &&
    parsed !== null &&
    !Array.isArray(parsed) &&
    typeof (parsed as Record<string, unknown>).value === 'number' &&
    typeof (parsed as Record<string, unknown>).max === 'number'
  ) {
    const obj = parsed as Record<string, unknown>;
    return {
      kind: 'gauge',
      data: {
        type: 'gauge',
        title: typeof obj.title === 'string' ? obj.title : '',
        value: obj.value as number,
        max: obj.max as number,
        unit: typeof obj.unit === 'string' ? obj.unit : undefined,
        thresholds:
          typeof obj.thresholds === 'object' && obj.thresholds !== null
            ? (obj.thresholds as GaugeData['thresholds'])
            : undefined,
      },
    };
  }

  // --- Sparkline: array of objects with a timestamp key + numeric field ---
  if (Array.isArray(parsed) && parsed.length >= 2) {
    const first = parsed[0];
    if (typeof first !== 'object' || first === null || Array.isArray(first)) return null;

    const keys = Object.keys(first as Record<string, unknown>);

    // Find a timestamp key
    const hasTimestamp = keys.some((k) => isTimestampKey(k));
    if (!hasTimestamp) return null;

    // Find the first numeric field (not the timestamp key)
    const numericKey = keys.find(
      (k) =>
        !isTimestampKey(k) && typeof (first as Record<string, unknown>)[k] === 'number',
    );
    if (!numericKey) return null;

    // Extract values — skip non-number entries
    const values: number[] = [];
    for (const item of parsed) {
      if (typeof item === 'object' && item !== null) {
        const v = (item as Record<string, unknown>)[numericKey];
        if (typeof v === 'number') values.push(v);
      }
    }
    if (values.length < 2) return null;

    return {
      kind: 'sparkline',
      data: {
        type: 'sparkline',
        title: numericKey,
        data: values,
      },
    };
  }

  return null;
}

/** Displays a tool call status card with optional auto-visualization. */
export function ToolStatusCard({ message }: { message: ChatMessage }) {
  const statusColor =
    message.toolStatus === 'executing'
      ? 'blue'
      : message.toolStatus === 'completed'
        ? 'green'
        : 'red';

  const viz = message.toolStatus === 'completed' ? detectToolViz(message.toolResult) : null;
  const [showRaw, setShowRaw] = useState(false);

  return (
    <Paper className={classes.toolCard} p="xs">
      <Group gap="xs">
        <IconBolt size={14} />
        {message.capability && (
          <Badge size="xs" variant="light">
            {message.capability}
          </Badge>
        )}
        <Text fz="xs" fw={500}>
          {message.toolName}
        </Text>
        <Badge size="xs" color={statusColor}>
          {message.toolStatus}
        </Badge>
        {viz && (
          <UnstyledButton
            onClick={() => setShowRaw((v) => !v)}
            aria-label={showRaw ? 'Show chart' : 'Show raw'}
            style={{ marginLeft: 'auto' }}
          >
            {showRaw ? (
              <IconChartLine size={14} color="var(--mantine-color-dimmed)" />
            ) : (
              <IconCode size={14} color="var(--mantine-color-dimmed)" />
            )}
          </UnstyledButton>
        )}
      </Group>

      {/* Visualization or raw text */}
      {viz && !showRaw ? (
        <div style={{ marginTop: 4 }}>
          {viz.kind === 'sparkline' && <SparklineBlock data={viz.data} />}
          {viz.kind === 'gauge' && <GaugeBlock data={viz.data} />}
        </div>
      ) : (
        message.toolResult && (
          <Text fz="xs" c="dimmed" mt={4} lineClamp={showRaw ? undefined : 3}>
            {message.toolResult}
          </Text>
        )
      )}
    </Paper>
  );
}
