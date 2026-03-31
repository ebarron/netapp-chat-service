import { Paper, Text, Stack, Group, ThemeIcon } from '@mantine/core';
import { IconAlertTriangle, IconAlertCircle, IconInfoCircle } from '@tabler/icons-react';
import type { AlertListData } from './chartTypes';

interface AlertListBlockProps {
  data: AlertListData;
  onAction?: (message: string) => void;
}

const severityConfig = {
  critical: { color: 'red', Icon: IconAlertCircle },
  warning: { color: 'yellow', Icon: IconAlertTriangle },
  info: { color: 'blue', Icon: IconInfoCircle },
} as const;

export function AlertListBlock({ data, onAction }: AlertListBlockProps) {
  return (
    <Paper p="sm" radius="sm" withBorder>
      {data.title && (
        <Text fw={500} fz="sm" mb="xs">
          {data.title}
        </Text>
      )}
      <Stack gap="xs" role="list" aria-label="Alerts">
        {data.items.length === 0 && (
          <Text fz="xs" c="dimmed" ta="center" py="sm">No alerts</Text>
        )}
        {data.items.map((item, i) => {
          const config = severityConfig[item.severity];
          return (
            <Group
              key={i}
              gap="xs"
              wrap="nowrap"
              role="listitem"
              aria-label={`${item.severity}: ${item.message}`}
              style={{ cursor: onAction ? 'pointer' : undefined }}
              onClick={() => onAction?.(`Tell me about the ${item.message} alert`)}
            >
              <ThemeIcon size="sm" color={config.color} variant="light">
                <config.Icon size={14} />
              </ThemeIcon>
              <Text fz="xs" style={{ flex: 1 }}>
                {item.message}
              </Text>
              <Text fz="xs" c="dimmed" style={{ whiteSpace: 'nowrap' }}>
                {item.time}
              </Text>
            </Group>
          );
        })}
      </Stack>
    </Paper>
  );
}
