import { Modal, Text, Group, Button, Code, Stack, Badge } from '@mantine/core';
import { IconAlertTriangle } from '@tabler/icons-react';
import type { PendingApproval } from './useChatPanel';

interface ActionConfirmationProps {
  approval: PendingApproval | null;
  onApprove: () => void;
  onDeny: () => void;
}

/**
 * ActionConfirmation renders the approval dialog for tool calls
 * in Ask mode.
 * Design ref: docs/chatbot-design-spec.md §6.5
 */
export function ActionConfirmation({ approval, onApprove, onDeny }: ActionConfirmationProps) {
  if (!approval) return null;

  return (
    <Modal
      opened={!!approval}
      onClose={onDeny}
      title={
        <Group gap="xs">
          <IconAlertTriangle size={18} color="var(--mantine-color-yellow-6)" />
          <Text fw={600}>Confirm Action</Text>
        </Group>
      }
      centered
      size="md"
    >
      <Stack gap="sm">
        <Group gap="xs">
          <Text fz="sm" fw={500}>
            Operation:
          </Text>
          <Text fz="sm">{approval.description}</Text>
        </Group>

        <Group gap="xs">
          <Text fz="sm" fw={500}>
            Capability:
          </Text>
          <Badge size="sm" variant="light">
            {approval.capability}
          </Badge>
        </Group>

        <Group gap="xs">
          <Text fz="sm" fw={500}>
            Tool:
          </Text>
          <Code>{approval.tool}</Code>
        </Group>

        {(approval.params !== undefined) && (
          <div>
            <Text fz="sm" fw={500} mb={4}>
              Parameters:
            </Text>
            <Code block>{JSON.stringify(approval.params, null, 2)}</Code>
          </div>
        )}

        <Group justify="flex-end" mt="md">
          <Button variant="default" onClick={onDeny}>
            Cancel
          </Button>
          <Button color="blue" onClick={onApprove}>
            Confirm
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
