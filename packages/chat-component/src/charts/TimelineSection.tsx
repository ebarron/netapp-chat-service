import { useState } from 'react';
import { Text, Group, ThemeIcon, Stack, Button } from '@mantine/core';
import { IconCircleFilled, IconBell, IconBolt } from '@tabler/icons-react';
import type { TimelineData, TimelineEvent } from './chartTypes';

interface TimelineSectionProps {
  data: TimelineData;
  onAction?: (message: string) => void;
}

const COLLAPSE_THRESHOLD = 10;

const severityColors: Record<string, string> = {
  critical: 'red',
  warning: 'yellow',
  info: 'blue',
  ok: 'green',
};

const iconMap: Record<string, typeof IconCircleFilled> = {
  notification: IconBell,
  action: IconBolt,
};

function EventIcon({ event }: { event: TimelineEvent }) {
  const Ico = (event.icon && iconMap[event.icon]) || IconCircleFilled;
  const color = event.severity ? severityColors[event.severity] ?? 'gray' : 'gray';
  return (
    <ThemeIcon size="xs" color={color} variant="light" radius="xl">
      <Ico size={8} />
    </ThemeIcon>
  );
}

export function TimelineSection({ data }: TimelineSectionProps) {
  const [expanded, setExpanded] = useState(false);
  const events = data.events;
  const shouldCollapse = events.length > COLLAPSE_THRESHOLD;
  const visible = shouldCollapse && !expanded ? events.slice(0, COLLAPSE_THRESHOLD) : events;
  const remaining = events.length - COLLAPSE_THRESHOLD;

  return (
    <Stack gap={4}>
      {visible.map((event, i) => (
        <Group key={i} gap="xs" wrap="nowrap" align="flex-start">
          <EventIcon event={event} />
          <Text fz="xs" c="dimmed" style={{ whiteSpace: 'nowrap', minWidth: 40 }}>
            {event.time}
          </Text>
          <Text fz="xs" style={{ flex: 1 }}>
            {event.label}
          </Text>
        </Group>
      ))}
      {shouldCollapse && !expanded && (
        <Button
          variant="subtle"
          size="compact-xs"
          onClick={() => setExpanded(true)}
        >
          Show {remaining} more
        </Button>
      )}
    </Stack>
  );
}
