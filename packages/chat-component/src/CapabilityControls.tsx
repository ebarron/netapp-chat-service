import { Popover, Stack, Text, Group, SegmentedControl, Badge, ActionIcon, Switch, Divider } from '@mantine/core';
import { IconSettings } from '@tabler/icons-react';
import type { Capability } from './useChatPanel';

interface CapabilityControlsProps {
  capabilities: Capability[];
  onUpdate: (id: string, state: string) => void;
  disabled?: boolean;
  showTraces?: boolean;
  onShowTracesChange?: (value: boolean) => void;
}

const STATE_OPTIONS = [
  { label: 'Off', value: 'off' },
  { label: 'Ask', value: 'ask' },
  { label: 'Allow', value: 'allow' },
];

/**
 * CapabilityControls renders the settings popover with Off/Ask/Allow
 * toggles per MCP capability.
 * Design ref: docs/chatbot-design-spec.md §7.3
 */
export function CapabilityControls({ capabilities, onUpdate, disabled, showTraces, onShowTracesChange }: CapabilityControlsProps) {
  return (
    <Popover position="bottom-end" width={300} withArrow withinPortal={false}>
      <Popover.Target>
        <ActionIcon
          variant="subtle"
          size="lg"
          aria-label="Capability settings"
          disabled={disabled}
        >
          <IconSettings size={18} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown>
        <Text fw={600} fz="sm" mb="xs">
          Capabilities
        </Text>
        <Stack gap="sm">
          {capabilities.map((cap) => (
            <div key={cap.id}>
              <Group gap="xs" mb={4}>
                <Text fz="sm" fw={500}>
                  {cap.name}
                </Text>
                {!cap.available && (
                  <Badge size="xs" color="gray" variant="outline">
                    unavailable
                  </Badge>
                )}
                {cap.tools_count > 0 && (
                  <Badge size="xs" color="blue" variant="light">
                    {cap.tools_count} tools
                  </Badge>
                )}
              </Group>
              <Text fz="xs" c="dimmed" mb={4}>
                {cap.description}
              </Text>
              <SegmentedControl
                size="xs"
                fullWidth
                value={cap.state}
                onChange={(v) => onUpdate(cap.id, v)}
                data={STATE_OPTIONS}
                disabled={!cap.available}
              />
            </div>
          ))}
          {capabilities.length === 0 && (
            <Text fz="xs" c="dimmed">
              No capabilities available.
            </Text>
          )}
          <Divider my="xs" />
          <Group justify="space-between">
            <Text fz="sm">Show tool traces</Text>
            <Switch
              size="xs"
              checked={showTraces}
              onChange={(e) => onShowTracesChange?.(e.currentTarget.checked)}
              aria-label="Show tool traces"
            />
          </Group>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  );
}
