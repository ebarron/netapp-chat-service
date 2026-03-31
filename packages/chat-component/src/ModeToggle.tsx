import { Group, SegmentedControl, Text, Tooltip } from '@mantine/core';
import { IconLock, IconPencil } from '@tabler/icons-react';
import type { ChatMode } from './useChatPanel';

interface ModeToggleProps {
  mode: ChatMode;
  onChange: (mode: ChatMode) => void;
  timeLeft: number | null;
  disabled?: boolean;
}

/** Format milliseconds as "Xm Ys". */
function formatTimeLeft(ms: number): string {
  const totalSec = Math.ceil(ms / 1000);
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  if (min > 0) return `${min}m ${sec}s`;
  return `${sec}s`;
}

/**
 * ModeToggle renders the read-only / read-write toggle at the top of the panel.
 * Read-write mode has an auto-disable timer (10 min).
 * Design ref: docs/chatbot-design-spec.md §6.4
 */
export function ModeToggle({ mode, onChange, timeLeft, disabled }: ModeToggleProps) {
  return (
    <Group gap="xs" justify="center" py={4}>
      <SegmentedControl
        size="xs"
        value={mode}
        onChange={(v) => onChange(v as ChatMode)}
        disabled={disabled}
        data={[
          {
            label: (
              <Tooltip label="Information retrieval only">
                <Group gap={4} wrap="nowrap">
                  <IconLock size={14} />
                  <span>Read-Only</span>
                </Group>
              </Tooltip>
            ),
            value: 'read-only',
          },
          {
            label: (
              <Tooltip label="All tools available — write ops require confirmation">
                <Group gap={4} wrap="nowrap">
                  <IconPencil size={14} />
                  <span>Read/Write</span>
                </Group>
              </Tooltip>
            ),
            value: 'read-write',
          },
        ]}
      />
      {mode === 'read-write' && timeLeft != null && timeLeft > 0 && (
        <Text fz="xs" c="dimmed">
          {formatTimeLeft(timeLeft)}
        </Text>
      )}
    </Group>
  );
}
