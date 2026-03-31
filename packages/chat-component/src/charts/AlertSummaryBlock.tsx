import { Paper, Text, Group, Badge } from '@mantine/core';
import type { AlertSummaryData } from './chartTypes';

interface AlertSummaryBlockProps {
  data: AlertSummaryData;
  onAction?: (message: string) => void;
}

const severityColors: Record<string, string> = {
  critical: 'red',
  warning: 'orange',
  info: 'blue',
};

export function AlertSummaryBlock({ data, onAction }: AlertSummaryBlockProps) {
  const counts = data.data;

  const handleClick = (severity: string) => {
    onAction?.(`Show me the ${severity} alerts`);
  };

  return (
    <Paper p="sm" radius="sm" withBorder>
      {data.title && (
        <Text fw={500} fz="sm" mb="xs">
          {data.title}
        </Text>
      )}
      <Group gap="sm">
        {Object.entries(counts).filter(([severity]) => severity !== 'ok').map(([severity, count]) => {
          const isClickable = onAction && count > 0;
          return (
            <Badge
              key={severity}
              color={severityColors[severity] ?? 'gray'}
              variant="light"
              size="lg"
              style={{ cursor: isClickable ? 'pointer' : undefined }}
              onClick={isClickable ? () => handleClick(severity) : undefined}
              role={isClickable ? 'button' : undefined}
              aria-label={`${severity}: ${count} alert${count === 1 ? '' : 's'}`}
            >
              {severity}: {count}
            </Badge>
          );
        })}
      </Group>
    </Paper>
  );
}
