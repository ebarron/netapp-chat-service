import { RingProgress, Text, Paper, Group, Stack } from '@mantine/core';
import type { GaugeData } from './chartTypes';

interface GaugeBlockProps {
  data: GaugeData;
}

function gaugeColor(value: number, thresholds?: { warning: number; critical: number }): string {
  if (!thresholds) return 'blue';
  if (value >= thresholds.critical) return 'red';
  if (value >= thresholds.warning) return 'yellow';
  return 'teal';
}

export function GaugeBlock({ data }: GaugeBlockProps) {
  const pct = data.max > 0 ? (data.value / data.max) * 100 : 0;
  const color = gaugeColor(data.value, data.thresholds);

  return (
    <Paper p="sm" radius="sm" withBorder role="meter" aria-valuenow={data.value} aria-valuemin={0} aria-valuemax={data.max} aria-label={`${data.title}: ${data.value}${data.unit ?? ''} of ${data.max}${data.unit ?? ''}`}>
      <Group justify="center" gap="md">
        <RingProgress
          size={120}
          thickness={12}
          roundCaps
          sections={[{ value: pct, color }]}
          label={
            <Stack gap={0} align="center">
              <Text fw={700} fz="lg" ta="center">
                {data.value}
                {data.unit ? data.unit : ''}
              </Text>
            </Stack>
          }
        />
        <Stack gap={2}>
          <Text fw={500} fz="sm">
            {data.title}
          </Text>
          <Text fz="xs" c="dimmed">
            {data.value} / {data.max}
            {data.unit ? ` ${data.unit}` : ''}
          </Text>
        </Stack>
      </Group>
    </Paper>
  );
}
