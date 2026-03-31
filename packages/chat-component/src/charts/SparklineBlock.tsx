import { Sparkline } from '@mantine/charts';
import { Paper, Text } from '@mantine/core';
import type { SparklineData } from './chartTypes';

interface SparklineBlockProps {
  data: SparklineData;
}

export function SparklineBlock({ data }: SparklineBlockProps) {
  const isEmpty = data.data.length === 0;

  return (
    <Paper p="sm" radius="sm" withBorder style={{ minWidth: 0 }} role="img" aria-label={`Sparkline: ${data.title ?? 'trend'}`}>
      {data.title && (
        <Text fw={500} fz="sm" mb="xs">
          {data.title}
        </Text>
      )}
      {isEmpty ? (
        <Text fz="xs" c="dimmed" ta="center" py="sm">No data available</Text>
      ) : (
        <Sparkline
          h={60}
          w="100%"
          data={data.data}
          color={data.color ?? 'blue'}
          curveType="monotone"
          fillOpacity={0.2}
          strokeWidth={2}
        />
      )}
    </Paper>
  );
}
