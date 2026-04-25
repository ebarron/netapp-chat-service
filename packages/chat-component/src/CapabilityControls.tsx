import { Popover, Stack, Text, Group, SegmentedControl, Badge, ActionIcon, Switch, Divider, Progress, Tooltip, Alert } from '@mantine/core';
import { IconSettings, IconAlertTriangle } from '@tabler/icons-react';
import type { Capability, ChatMode, ToolBudgets } from './useChatPanel';

interface CapabilityControlsProps {
  capabilities: Capability[];
  onUpdate: (id: string, state: string) => void;
  disabled?: boolean;
  showTraces?: boolean;
  onShowTracesChange?: (value: boolean) => void;
  /** Current chat mode. Determines which budget is displayed. */
  mode?: ChatMode;
  /** Per-mode tool budgets from /chat/capabilities. */
  toolBudgets?: ToolBudgets | null;
  /** Last error from a capability/mode change attempt (e.g. budget exceeded). */
  capabilityError?: string | null;
  /** Clear the displayed error. */
  onClearCapabilityError?: () => void;
}

const STATE_OPTIONS = [
  { label: 'Off', value: 'off' },
  { label: 'Ask', value: 'ask' },
  { label: 'Allow', value: 'allow' },
];

/**
 * CapabilityControls renders the settings popover with Off/Ask/Allow
 * toggles per MCP capability, plus a tool-budget bar at the top so the
 * user can see how close they are to the LLM's hard 128-tool limit.
 *
 * Toggles that would push the budget over the cap in the current mode
 * are disabled with a visual marker. The server is the source of truth
 * and will reject the change with a clear message that the parent
 * surfaces via `capabilityError`.
 *
 * Design ref: docs/chatbot-design-spec.md §7.3, §8.2
 */
export function CapabilityControls({
  capabilities,
  onUpdate,
  disabled,
  showTraces,
  onShowTracesChange,
  mode = 'read-only',
  toolBudgets,
  capabilityError,
  onClearCapabilityError,
}: CapabilityControlsProps) {
  const budget = toolBudgets ? (mode === 'read-write' ? toolBudgets.read_write : toolBudgets.read_only) : null;
  const pct = budget ? Math.min(100, (budget.used / budget.max) * 100) : 0;
  const overBudget = !!budget && budget.used > budget.max;
  const nearBudget = !!budget && !overBudget && pct >= 80;
  const barColor = overBudget ? 'red' : nearBudget ? 'yellow' : 'blue';

  /**
   * Predicts whether toggling a capability ON would exceed the budget in
   * the current mode. Used to disable the Ask/Allow options for capabilities
   * whose tools alone would push us over the cap.
   */
  const wouldExceed = (cap: Capability): boolean => {
    if (!budget) return false;
    if (cap.state !== 'off') return false; // already counted
    const add = mode === 'read-write' ? cap.tools_count : cap.read_only_tools_count;
    return budget.used + add > budget.max;
  };

  return (
    <Popover position="bottom-end" width={320} withArrow withinPortal={false}>
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

        {budget && (
          <div style={{ marginBottom: 12 }} aria-label="Tool budget">
            <Group justify="space-between" mb={4}>
              <Text fz="xs" c="dimmed">
                Tool budget ({mode})
              </Text>
              <Text fz="xs" c={overBudget ? 'red' : 'dimmed'}>
                {budget.used} / {budget.max}
              </Text>
            </Group>
            <Progress value={pct} color={barColor} size="sm" radius="xs" aria-label="Tool budget usage" />
            {overBudget && (
              <Text fz="xs" c="red" mt={4}>
                Over the {budget.max}-tool limit. The LLM will reject requests until you disable a capability.
              </Text>
            )}
          </div>
        )}

        {capabilityError && (
          <Alert
            color="red"
            variant="light"
            mb="xs"
            icon={<IconAlertTriangle size={16} />}
            withCloseButton={!!onClearCapabilityError}
            onClose={onClearCapabilityError}
          >
            <Text fz="xs">{capabilityError}</Text>
          </Alert>
        )}

        <Stack gap="sm">
          {capabilities.map((cap) => {
            const blocked = wouldExceed(cap);
            const segOptions = STATE_OPTIONS.map((opt) => ({
              ...opt,
              disabled: blocked && opt.value !== 'off',
            }));
            return (
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
                    <Tooltip
                      label={`${cap.read_only_tools_count} read-only of ${cap.tools_count} total`}
                      withinPortal={false}
                    >
                      <Badge size="xs" color="blue" variant="light">
                        {cap.tools_count} tools ({cap.read_only_tools_count} ro)
                      </Badge>
                    </Tooltip>
                  )}
                  {blocked && (
                    <Badge size="xs" color="red" variant="light">
                      over budget
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
                  data={segOptions}
                  disabled={!cap.available}
                />
              </div>
            );
          })}
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
