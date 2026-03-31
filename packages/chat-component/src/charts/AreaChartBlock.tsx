import { AreaChart } from '@mantine/charts';
import { Paper, Text } from '@mantine/core';
import { useMemo } from 'react';
import type { AreaChartData, ChartAnnotation } from './chartTypes';

interface AreaChartBlockProps {
  data: AreaChartData;
}

/** Format a timestamp (in ms) as a short date label. */
function formatTimestamp(ms: number, includeTime: boolean): string {
  const d = new Date(ms);
  if (isNaN(d.getTime())) return String(ms);
  if (includeTime) {
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) +
      ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false });
  }
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/**
 * Normalize x-axis values in chart data. LLMs sometimes emit unix timestamps,
 * ISO-8601 strings, or invented labels. This converts everything to readable
 * date labels and deduplicates adjacent identical labels.
 */
function normalizeXAxis(rows: Record<string, unknown>[], xKey: string): Record<string, unknown>[] {
  if (rows.length === 0) return rows;
  const first = rows[0][xKey];
  const last = rows[rows.length - 1][xKey];

  // Try to extract a numeric timestamp from each row's xKey value.
  const asTs = (v: unknown): number | null => {
    if (typeof v === 'number' && v > 1e8) return v > 1e12 ? v : v * 1000;
    if (typeof v === 'string') {
      // ISO-8601 or parseable date string.
      const ms = Date.parse(v);
      if (!isNaN(ms) && ms > 1e11) return ms;
    }
    return null;
  };

  const firstTs = asTs(first);
  const lastTs = asTs(last);

  if (firstTs != null && lastTs != null) {
    const spanDays = (lastTs - firstTs) / (1000 * 60 * 60 * 24);
    // Choose format based on data density.
    const includeTime = rows.length > spanDays * 2;
    const formatted = rows.map((r) => {
      const ts = asTs(r[xKey]);
      if (ts == null) return r;
      return { ...r, [xKey]: formatTimestamp(ts, includeTime) };
    });
    return deduplicateLabels(formatted, xKey);
  }

  // Non-timestamp strings — just deduplicate adjacent repeats.
  return deduplicateLabels(rows, xKey);
}

/**
 * Replace adjacent identical x-axis labels with empty strings so only the
 * first occurrence shows. This prevents "Mar 8 Mar 8 Mar 8 ..." clutter.
 */
function deduplicateLabels(rows: Record<string, unknown>[], xKey: string): Record<string, unknown>[] {
  let prev: unknown = null;
  return rows.map((r) => {
    const val = r[xKey];
    if (val === prev) return { ...r, [xKey]: '' };
    prev = val;
    return r;
  });
}

function toReferenceLines(annotations?: ChartAnnotation[]) {
  if (!annotations || annotations.length === 0) return undefined;
  return annotations.map((a) => ({
    y: a.y,
    label: a.label,
    color: a.color ?? 'red',
    strokeDasharray: a.style === 'dashed' ? '5 5' : undefined,
  }));
}

export function AreaChartBlock({ data }: AreaChartBlockProps) {
  const chartData = useMemo(() => normalizeXAxis(data.data, data.xKey), [data.data, data.xKey]);
  const isEmpty = chartData.length === 0;

  return (
    <Paper p="sm" radius="sm" withBorder style={{ minWidth: 0 }} role="img" aria-label={`Area chart: ${data.title}`}>
      <Text fw={500} fz="sm" mb="xs">
        {data.title}
      </Text>
      {isEmpty ? (
        <Text fz="xs" c="dimmed" ta="center" py="xl">No data available</Text>
      ) : (
        <AreaChart
          h={200}
          data={chartData}
          dataKey={data.xKey}
          series={data.series.map((s) => ({
            name: s.key,
            label: s.label,
            color: s.color ?? 'blue',
          }))}
          yAxisLabel={data.yLabel}
          curveType="monotone"
          withTooltip
          withDots={false}
          tooltipAnimationDuration={200}
          referenceLines={toReferenceLines(data.annotations)}
        />
      )}
    </Paper>
  );
}
