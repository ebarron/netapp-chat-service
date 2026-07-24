import { ActionIcon, Code, CopyButton, Tooltip } from '@mantine/core';
import { IconCheck, IconCopy } from '@tabler/icons-react';

export interface CodeBlockProps {
  /** Raw code text to display and copy. */
  code: string;
  /** Optional language tag from a `language-xxx` className (informational). */
  language?: string;
}

/**
 * Block-level monospaced code box with a copy-to-clipboard affordance.
 * Built from existing Mantine primitives — no extra dependencies.
 */
export function CodeBlock({ code, language }: CodeBlockProps) {
  return (
    <div style={{ position: 'relative' }} data-language={language || undefined}>
      <CopyButton value={code} timeout={2000}>
        {({ copied, copy }) => (
          <Tooltip label={copied ? 'Copied' : 'Copy'} withArrow position="left">
            <ActionIcon
              aria-label="Copy code"
              color={copied ? 'teal' : 'gray'}
              variant="subtle"
              size="sm"
              onClick={copy}
              style={{ position: 'absolute', top: 6, right: 6, zIndex: 1 }}
            >
              {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
            </ActionIcon>
          </Tooltip>
        )}
      </CopyButton>
      <Code block pr={36}>
        {code}
      </Code>
    </div>
  );
}
