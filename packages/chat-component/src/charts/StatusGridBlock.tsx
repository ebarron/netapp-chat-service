import { Paper, Text, SimpleGrid, Badge, Group, ThemeIcon } from '@mantine/core';
import { IconCheck, IconAlertTriangle, IconAlertCircle, IconQuestionMark } from '@tabler/icons-react';
import type { StatusGridData } from './chartTypes';

interface StatusGridBlockProps {
  data: StatusGridData;
}

const statusConfig = {
  ok: { color: 'teal', Icon: IconCheck },
  warning: { color: 'yellow', Icon: IconAlertTriangle },
  critical: { color: 'red', Icon: IconAlertCircle },
  unknown: { color: 'gray', Icon: IconQuestionMark },
} as const;

export function StatusGridBlock({ data }: StatusGridBlockProps) {
  return (
    <Paper p="sm" radius="sm" withBorder>
      <Text fw={500} fz="sm" mb="xs">
        {data.title}
      </Text>
      <SimpleGrid cols={{ base: 2, sm: 3, md: 4 }} spacing="xs" role="list" aria-label="Status items">
        {data.items.map((item) => {
          const config = statusConfig[item.status];
          return (
            <Group key={item.name} gap="xs" wrap="nowrap" role="listitem" aria-label={`${item.name}: ${item.status}`}>
              <ThemeIcon size="sm" color={config.color} variant="light">
                <config.Icon size={14} />
              </ThemeIcon>
              <div>
                <Text fz="xs" fw={500}>
                  {item.name}
                </Text>
                {item.detail && (
                  <Text fz="xs" c="dimmed">
                    {item.detail}
                  </Text>
                )}
              </div>
            </Group>
          );
        })}
      </SimpleGrid>
    </Paper>
  );
}
