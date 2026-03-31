import { Paper, Text, Table } from '@mantine/core';
import type { ResourceTableData } from './chartTypes';

interface ResourceTableBlockProps {
  data: ResourceTableData;
  onAction?: (message: string) => void;
}

// Well-known aliases: column display name (lowercase) → row property fallbacks
const COLUMN_ALIASES: Record<string, string[]> = {
  resource: ['name'],
  volume: ['name'],
  aggregate: ['name'],
  cluster: ['name'],
  'key metric': ['metric', 'keyMetric', 'key_metric'],
  'used %': ['metric', 'used', 'used_pct', 'keyMetric'],
  capacity: ['Capacity', 'capacity', 'Used', 'used', 'metric', 'used_pct'],
  trend: ['trend', 'sparkline'],
};

/** Check if a value is a numeric array suitable for sparkline rendering. */
function isSparklineData(v: unknown): v is number[] {
  return Array.isArray(v) && v.length > 1 && v.every((n) => typeof n === 'number');
}

/**
 * Tiny area sparkline with a fixed 0-based Y domain.
 * Uses max(100, dataMax) so percentage data (0–100) renders to scale,
 * while larger values (e.g. IOPS) auto-scale above 100.
 */
function MiniSparkline({ data, w = 80, h = 28 }: { data: number[]; w?: number; h?: number }) {
  const pad = 2;
  const cw = w - pad * 2;
  const ch = h - pad * 2;
  const ceil = Math.max(100, ...data);
  const points = data
    .map((v, i) => {
      const x = pad + (i / (data.length - 1)) * cw;
      const y = pad + ch - (v / ceil) * ch;
      return `${x},${y}`;
    })
    .join(' ');
  const baseline = pad + ch;
  const first = points.split(' ')[0];
  const last = points.split(' ').at(-1);
  return (
    <svg width={w} height={h} role="img" aria-label="trend">
      <polygon
        points={`${first!.split(',')[0]},${baseline} ${points} ${last!.split(',')[0]},${baseline}`}
        fill="var(--mantine-color-blue-4)"
        opacity={0.35}
      />
      <polyline points={points} fill="none" stroke="var(--mantine-color-blue-6)" strokeWidth={2} />
    </svg>
  );
}

/** Resolve a cell value from a row. Returns a number[] for sparkline columns, otherwise a string. */
function resolveCell(row: Record<string, unknown>, col: string): string | number[] {
  const exact = row[col] ?? row[col.toLowerCase()];
  if (isSparklineData(exact)) return exact;
  if (exact !== undefined && exact !== null) return String(exact);
  const aliases = COLUMN_ALIASES[col.toLowerCase()];
  if (aliases) {
    for (const alias of aliases) {
      const v = row[alias];
      if (isSparklineData(v)) return v;
      if (v !== undefined && v !== null) return String(v);
    }
  }
  return '';
}

/**
 * Normalize a column name to a trend-field key: lowercase, strip non-alpha,
 * collapse spaces → underscore. "Used %" → "used", "IOPS" → "iops".
 */
function trendKey(col: string): string {
  return col.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '');
}

/**
 * Look up a hidden inline-trend array for a given column in a row.
 * Checks `{col}_trend` (e.g. "capacity_trend", "iops_trend") and falls
 * back to plain "trend" for the first match.
 */
function findInlineTrend(row: Record<string, unknown>, col: string, visibleCols: string[]): number[] | null {
  // Don't inline if Trend is already a visible column
  if (visibleCols.some((c) => c.toLowerCase() === 'trend')) return null;

  const key = trendKey(col) + '_trend';
  const v = row[key];
  if (isSparklineData(v)) return v;

  // Check COLUMN_ALIASES: e.g. column "Used" → alias group "capacity" → "capacity_trend"
  for (const [group, members] of Object.entries(COLUMN_ALIASES)) {
    if (members.some((m) => m.toLowerCase() === col.toLowerCase()) || group === col.toLowerCase()) {
      const aliasKey = group + '_trend';
      if (aliasKey !== key) {
        const av = row[aliasKey];
        if (isSparklineData(av)) return av;
      }
    }
  }

  // Fallback: generic "trend" field attaches to the first % cell
  const generic = row.trend ?? row.Trend;
  if (isSparklineData(generic)) {
    // Only use the generic for the first percentage column
    const firstPctCol = visibleCols.find((c) => {
      const val = resolveCell(row, c);
      return typeof val === 'string' && val.includes('%');
    });
    if (firstPctCol === col) return generic as number[];
  }
  return null;
}

export function ResourceTableBlock({ data, onAction }: ResourceTableBlockProps) {
  const handleRowClick = (row: Record<string, unknown>) => {
    const name = String(row.name || '');
    if (!name || !onAction) return;
    const col0 = data.columns[0]?.toLowerCase() || '';
    const genericNames = new Set(['name', 'resource', '']);
    const kind = genericNames.has(col0) ? '' : col0;
    let prompt: string;
    if (kind === 'cluster') {
      prompt = `Show cluster ${name}`;
    } else {
      prompt = kind ? `Tell me about ${kind} ${name}` : `Tell me about ${name}`;
      const svm = row.svm || row.SVM;
      const cluster = row.cluster || row.Cluster;
      if (svm) prompt += ` on SVM ${svm}`;
      if (cluster) prompt += ` on cluster ${cluster}`;
    }
    onAction(prompt);
  };

  return (
    <Paper p="sm" radius="sm" withBorder>
      <Text fw={500} fz="sm" mb="xs">
        {data.title}
      </Text>
      <Table highlightOnHover fz="xs" aria-label={data.title}>
        <Table.Thead>
          <Table.Tr>
            {data.columns.map((col) => (
              <Table.Th key={col}>{col}</Table.Th>
            ))}
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {data.rows.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={data.columns.length}>
                <Text fz="xs" c="dimmed" ta="center">No data available</Text>
              </Table.Td>
            </Table.Tr>
          )}
          {data.rows.map((row, rowIdx) => {
            const r = row as Record<string, unknown>;

            return (
              <Table.Tr
                key={row.name ?? rowIdx}
                style={{ cursor: onAction ? 'pointer' : undefined }}
                onClick={() => handleRowClick(r)}
              >
                {data.columns.map((col) => {
                  const value = resolveCell(r, col);

                  // Dedicated sparkline column (e.g. visible Trend column)
                  if (isSparklineData(value)) {
                    return <Table.Td key={col}><MiniSparkline data={value} /></Table.Td>;
                  }

                  // Inline sparkline: render next to value when a hidden trend field exists
                  const trend = findInlineTrend(r, col, data.columns);
                  if (trend) {
                    return (
                      <Table.Td key={col}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                          {value}
                          <MiniSparkline data={trend} w={60} h={20} />
                        </span>
                      </Table.Td>
                    );
                  }

                  return <Table.Td key={col}>{value}</Table.Td>;
                })}
              </Table.Tr>
            );
          })}
        </Table.Tbody>
      </Table>
    </Paper>
  );
}
