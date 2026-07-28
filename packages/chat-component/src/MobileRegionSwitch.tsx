import { ActionIcon, Group, Text, Tooltip } from '@mantine/core';
import { IconLayoutSidebar, IconMessageChatbot } from '@tabler/icons-react';
import classes from './ChatPanel.module.css';

export type MobileRegion = 'canvas' | 'assistant';

interface MobileRegionSwitchProps {
  region: MobileRegion;
  onChange: (region: MobileRegion) => void;
  /** `'bottom-tab'` (default) or `'toggle'`. */
  variant?: 'bottom-tab' | 'toggle';
}

/**
 * Page ↔ Assistant region switch for the mobile single-column layout.
 * Bottom-tab is the MVP default (mobile expression of assistant-canvas).
 */
export function MobileRegionSwitch({
  region,
  onChange,
  variant = 'bottom-tab',
}: MobileRegionSwitchProps) {
  if (variant === 'toggle') {
    const next: MobileRegion = region === 'canvas' ? 'assistant' : 'canvas';
    const label = region === 'canvas' ? 'Show assistant' : 'Show page';
    return (
      <div className={classes.mobileToggleBar} data-testid="mobile-region-toggle">
        <Tooltip label={label}>
          <ActionIcon
            variant="light"
            size="lg"
            aria-label={label}
            onClick={() => onChange(next)}
          >
            {region === 'canvas' ? (
              <IconMessageChatbot size={18} />
            ) : (
              <IconLayoutSidebar size={18} />
            )}
          </ActionIcon>
        </Tooltip>
      </div>
    );
  }

  return (
    <nav className={classes.mobileBottomTabBar} aria-label="Mobile region" data-testid="mobile-region-tabs">
      <Group grow gap={0} wrap="nowrap" className={classes.mobileBottomTabGroup}>
        <button
          type="button"
          className={classes.mobileBottomTab}
          data-active={region === 'canvas' || undefined}
          aria-current={region === 'canvas' ? 'page' : undefined}
          aria-label="Page"
          onClick={() => onChange('canvas')}
        >
          <IconLayoutSidebar size={18} />
          <Text size="xs" fw={region === 'canvas' ? 600 : 400}>
            Page
          </Text>
        </button>
        <button
          type="button"
          className={classes.mobileBottomTab}
          data-active={region === 'assistant' || undefined}
          aria-current={region === 'assistant' ? 'page' : undefined}
          aria-label="Assistant"
          onClick={() => onChange('assistant')}
        >
          <IconMessageChatbot size={18} />
          <Text size="xs" fw={region === 'assistant' ? 600 : 400}>
            Assistant
          </Text>
        </button>
      </Group>
    </nav>
  );
}
