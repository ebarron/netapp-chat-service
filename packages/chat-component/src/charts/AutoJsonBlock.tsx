/**
 * Generic JSON renderer — renders arbitrary JSON as visual elements when no
 * dedicated chart type matches. This is the last-resort fallback so the user
 * never sees a wall of raw JSON text.
 *
 * Rendering strategy:
 *   - Array of objects → table
 *   - Object with `items` array of objects → optional title + table
 *   - Flat/nested object → key-value property list
 *   - Primitive / array of primitives → plain text
 */
import { Code, Group, Paper, Stack, Table, Text } from '@mantine/core';
import { sanitizeJson } from '../inlineChartDetector';

interface AutoJsonBlockProps {
  json: string;
}

/* ------------------------------------------------------------------ */
/*  Formatting helpers                                                */
/* ------------------------------------------------------------------ */

/** camelCase / snake_case / kebab-case → Title Case */
function formatLabel(key: string): string {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/* ------------------------------------------------------------------ */
/*  Sub-components                                                    */
/* ------------------------------------------------------------------ */

/** Renders an array of objects as a striped table. */
function AutoTable({ rows }: { rows: Record<string, unknown>[] }) {
  const columns = [...new Set(rows.flatMap((r) => Object.keys(r)))];
  const capped = rows.slice(0, 200);

  return (
    <Table striped highlightOnHover fz="xs">
      <Table.Thead>
        <Table.Tr>
          {columns.map((col) => (
            <Table.Th key={col}>{formatLabel(col)}</Table.Th>
          ))}
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {capped.map((row, i) => (
          <Table.Tr key={i}>
            {columns.map((col) => (
              <Table.Td key={col}>{formatValue(row[col])}</Table.Td>
            ))}
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  );
}

/** Renders a flat/nested object as key-value pairs. */
function AutoProperties({ data }: { data: Record<string, unknown> }) {
  return (
    <Stack gap={4}>
      {Object.entries(data).map(([key, value]) => (
        <Group key={key} gap="xs" wrap="nowrap" align="flex-start">
          <Text fz="xs" fw={500} c="dimmed" style={{ minWidth: 120, flexShrink: 0 }}>
            {formatLabel(key)}
          </Text>
          <Text fz="xs" style={{ wordBreak: 'break-word' }}>
            {formatValue(value)}
          </Text>
        </Group>
      ))}
    </Stack>
  );
}

/** Recursive renderer that picks the right display for any value. */
function AutoRender({ data }: { data: unknown }) {
  // Array of objects → table
  if (Array.isArray(data) && data.length > 0 && typeof data[0] === 'object' && data[0] !== null) {
    return <AutoTable rows={data as Record<string, unknown>[]} />;
  }
  // Array of primitives
  if (Array.isArray(data)) {
    return <Text fz="sm">{data.map(String).join(', ')}</Text>;
  }
  // Object
  if (typeof data === 'object' && data !== null) {
    const rec = data as Record<string, unknown>;
    // Has items array of objects → extract title and render items as table
    if (Array.isArray(rec.items) && rec.items.length > 0 && typeof rec.items[0] === 'object') {
      return (
        <>
          {typeof rec.title === 'string' && (
            <Text fw={500} fz="sm" mb="xs">{rec.title}</Text>
          )}
          <AutoTable rows={rec.items as Record<string, unknown>[]} />
        </>
      );
    }
    // Flat/nested object → properties
    return <AutoProperties data={rec} />;
  }
  // Primitive
  return <Text fz="sm">{String(data)}</Text>;
}

/* ------------------------------------------------------------------ */
/*  Main component                                                    */
/* ------------------------------------------------------------------ */

export function AutoJsonBlock({ json }: AutoJsonBlockProps) {
  try {
    const data = JSON.parse(sanitizeJson(json));
    return (
      <Paper p="sm" radius="sm" withBorder>
        <AutoRender data={data} />
      </Paper>
    );
  } catch {
    return <Code block>{json}</Code>;
  }
}
