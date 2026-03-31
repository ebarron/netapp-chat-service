import { Paper, Text, Group } from '@mantine/core';
import { IconTrendingUp, IconTrendingDown, IconMinus } from '@tabler/icons-react';
import type { StatData } from './chartTypes';

interface StatBlockProps {
  data: StatData;
}

const trendIcons = {
  up: IconTrendingUp,
  down: IconTrendingDown,
  flat: IconMinus,
} as const;

const trendColors = {
  up: 'teal',
  down: 'red',
  flat: 'dimmed',
} as const;

export function StatBlock({ data }: StatBlockProps) {
  const TrendIcon = data.trend ? trendIcons[data.trend] : null;
  const trendColor = data.trend ? trendColors[data.trend] : undefined;

  return (
    <Paper p="sm" radius="sm" withBorder aria-label={`${data.title}: ${data.value}`}>
      <Text fw={500} fz="sm" c="dimmed">
        {data.title}
      </Text>
      <Text fw={700} fz="xl" mt={2}>
        {data.value}
      </Text>
      {data.subtitle && (
        <Text fz="xs" c="dimmed">
          {data.subtitle}
        </Text>
      )}
      {TrendIcon && (
        <Group gap={4} mt={4}>
          <TrendIcon size={14} color={`var(--mantine-color-${trendColor}-6)`} />
          {data.trendValue && (
            <Text fz="xs" c={trendColor}>
              {data.trendValue}
            </Text>
          )}
        </Group>
      )}
    </Paper>
  );
}
