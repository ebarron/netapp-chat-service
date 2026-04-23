import { Popover, Stack, Text, Group, ActionIcon, Badge, Divider } from '@mantine/core';
import { IconBookmark } from '@tabler/icons-react';
import type { Capability } from './useChatPanel';

/** A single bookmark prompt entry. */
export interface BookmarkPrompt {
  /** Display label for the prompt. */
  label: string;
  /** The prompt text to send when clicked. */
  prompt: string;
  /** MCP IDs this bookmark requires (all must be enabled). Empty = always visible. */
  requiredMcps?: string[];
  /** Grouping key — bookmarks are grouped by this label. Defaults to "General". */
  group?: string;
}

interface BookmarkPromptsProps {
  bookmarks: BookmarkPrompt[];
  capabilities: Capability[];
  onSelect: (prompt: string) => void;
  disabled?: boolean;
}

function isBookmarkVisible(bookmark: BookmarkPrompt, capabilities: Capability[]): boolean {
  if (!bookmark.requiredMcps || bookmark.requiredMcps.length === 0) return true;
  return bookmark.requiredMcps.every((mcpId) => {
    const cap = capabilities.find((c) => c.id === mcpId);
    return cap && cap.available && cap.state !== 'off';
  });
}

/**
 * BookmarkPrompts renders a bookmarks popover with pre-defined prompts
 * grouped by MCP/category. Only shows bookmarks whose required MCPs
 * are loaded and enabled.
 */
export function BookmarkPrompts({ bookmarks, capabilities, onSelect, disabled }: BookmarkPromptsProps) {
  const visible = bookmarks.filter((b) => isBookmarkVisible(b, capabilities));
  if (visible.length === 0) return null;

  const grouped = new Map<string, BookmarkPrompt[]>();
  for (const b of visible) {
    const group = b.group || 'General';
    const list = grouped.get(group) || [];
    list.push(b);
    grouped.set(group, list);
  }

  return (
    <Popover position="bottom-end" width={340} withArrow withinPortal={false}>
      <Popover.Target>
        <ActionIcon
          variant="subtle"
          size="lg"
          aria-label="Bookmark prompts"
          disabled={disabled}
        >
          <IconBookmark size={18} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown>
        <Text fw={600} fz="sm" mb="xs">
          Saved Prompts
        </Text>
        <Stack gap="xs" style={{ maxHeight: 400, overflowY: 'auto' }}>
          {[...grouped.entries()].map(([group, items], gi) => (
            <div key={group}>
              {gi > 0 && <Divider my={4} />}
              <Group gap={4} mb={4}>
                <Text fz="xs" fw={600} c="dimmed" tt="uppercase">
                  {group}
                </Text>
                <Badge size="xs" variant="light">
                  {items.length}
                </Badge>
              </Group>
              {items.map((b) => (
                <Text
                  key={b.prompt}
                  fz="sm"
                  style={{
                    cursor: 'pointer',
                    padding: '4px 8px',
                    borderRadius: 4,
                    transition: 'background-color 150ms ease',
                  }}
                  onMouseEnter={(e) => {
                    (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--mantine-color-gray-1)';
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLElement).style.backgroundColor = '';
                  }}
                  onClick={() => onSelect(b.prompt)}
                >
                  {b.label}
                </Text>
              ))}
            </div>
          ))}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  );
}
