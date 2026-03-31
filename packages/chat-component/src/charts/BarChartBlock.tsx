import { BarChart } from '@mantine/charts';
import { Paper, Text } from '@mantine/core';
import type { BarChartData } from './chartTypes';

interface BarChartBlockProps {
  data: BarChartData;
}

export function BarChartBlock({ data }: BarChartBlockProps) {
  const isEmpty = data.data.length === 0;

  return (
    <Paper p="sm" radius="sm" withBorder style={{ minWidth: 0 }} role="img" aria-label={`Bar chart: ${data.title}`}>
      <Text fw={500} fz="sm" mb="xs">
        {data.title}
      </Text>
      {isEmpty ? (
        <Text fz="xs" c="dimmed" ta="center" py="xl">No data available</Text>
      ) : (
        <BarChart
          h={200}
          data={data.data}
          dataKey={data.xKey}
          series={data.series.map((s) => ({
            name: s.key,
            label: s.label,
            color: s.color ?? 'violet',
          }))}
          withTooltip
          tooltipAnimationDuration={200}
        />
      )}
    </Paper>
  );
}
