import React from 'react';
import { Group, Button, Tooltip } from '@mantine/core';
import type { ActionButtonData, ActionButtonItem } from './chartTypes';
import { applyQualifier } from './ObjectDetailBlock';
import type { InteractiveSubmissionState } from '../useChatPanel';

interface ActionButtonBlockProps {
  data: ActionButtonData;
  onAction?: (message: string) => void;
  onExecute?: (tool: string, params?: Record<string, unknown>) => void;
  readOnly?: boolean;
  interactiveSubmissionState?: InteractiveSubmissionState;
  /** Card-level qualifier from the parent object-detail block. */
  cardQualifier?: string;
}

export function ActionButtonBlock({
  data,
  onAction,
  onExecute,
  readOnly,
  interactiveSubmissionState = 'idle',
  cardQualifier,
}: ActionButtonBlockProps) {
  const buttons = Array.isArray(data.buttons)
    ? data.buttons.filter((button): button is ActionButtonItem => (
        typeof button === 'object'
        && button !== null
        && typeof button.label === 'string'
      ))
    : [];

  const handleClick = (btn: ActionButtonItem) => {
    const action = resolveAction(btn);
    if (action === 'message' && btn.message) {
      onAction?.(applyQualifier(btn.message, btn.qualifier, cardQualifier));
    } else if (action === 'execute' && btn.tool) {
      onExecute?.(btn.tool, btn.params);
    }
  };

  return (
    <Group gap="xs" role="group" aria-label="Actions">
      {buttons.map((btn, i) => {
        const action = resolveAction(btn);
        const readOnlyDisabled = (action === 'execute' || !!btn.requiresReadWrite) && !!readOnly;
        const queueDisabled = action === 'message' && interactiveSubmissionState !== 'idle';
        const isDisabled = readOnlyDisabled || queueDisabled;
        const button = (
          <Button
            size="compact-sm"
            variant={btn.variant === 'outline' ? 'outline' : 'filled'}
            disabled={isDisabled}
            onClick={() => handleClick(btn)}
          >
            {btn.label}
          </Button>
        );
        if (readOnlyDisabled && btn.requiresReadWrite) {
          return <Tooltip key={i} label="Enable read-write mode to use this action">{button}</Tooltip>;
        }
        return <React.Fragment key={i}>{button}</React.Fragment>;
      })}
    </Group>
  );
}

/** Infer the button action from explicit field or implicit fields. */
function resolveAction(btn: ActionButtonItem): string | undefined {
  if (btn.action) return btn.action;
  if (btn.tool) return 'execute';
  if (btn.message) return 'message';
  return undefined;
}
