import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor, userEvent } from '../test-utils';
import { BookmarkPrompts } from './BookmarkPrompts';
import type { BookmarkPrompt } from './BookmarkPrompts';
import type { Capability } from './useChatPanel';

const makeCap = (id: string, available = true, state: Capability['state'] = 'allow'): Capability => ({
  id,
  name: id,
  description: `${id} description`,
  state,
  available,
  tools_count: 1,
  read_only_tools_count: 1,
});

const bookmarks: BookmarkPrompt[] = [
  { label: 'AFF vs FAS split', prompt: 'What is the AFF vs FAS split?', requiredMcps: ['puat'], group: 'PUAT' },
  { label: 'NAS adoption trend', prompt: 'Show NAS adoption trend', requiredMcps: ['puat'], group: 'PUAT' },
  { label: 'Search Confluence', prompt: 'Search confluence for docs', requiredMcps: ['confluence'], group: 'Confluence' },
  { label: 'General question', prompt: 'What can you help me with?', group: 'General' },
  { label: 'Cross-MCP query', prompt: 'Correlate JIRA and Confluence', requiredMcps: ['jira', 'confluence'], group: 'Cross-MCP' },
];

describe('BookmarkPrompts', () => {
  it('returns null when no bookmarks are visible', () => {
    const caps = [makeCap('puat', false)];
    const { container } = render(
      <BookmarkPrompts bookmarks={[bookmarks[0]]} capabilities={caps} onSelect={vi.fn()} />,
    );
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders the bookmark icon when bookmarks are visible', () => {
    const caps = [makeCap('puat')];
    render(<BookmarkPrompts bookmarks={bookmarks} capabilities={caps} onSelect={vi.fn()} />);
    expect(screen.getByLabelText('Bookmark prompts')).toBeDefined();
  });

  it('shows only bookmarks whose MCPs are enabled', async () => {
    const caps = [makeCap('puat'), makeCap('confluence', true, 'off')];
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<BookmarkPrompts bookmarks={bookmarks} capabilities={caps} onSelect={onSelect} />);

    await user.click(screen.getByLabelText('Bookmark prompts'));

    await waitFor(() => {
      expect(screen.getByText('AFF vs FAS split')).toBeDefined();
    });
    expect(screen.getByText('NAS adoption trend')).toBeDefined();

    // General (no MCP required) should be visible
    expect(screen.getByText('General question')).toBeDefined();

    // Confluence is off, so its bookmark should not be visible
    expect(screen.queryByText('Search Confluence')).toBeNull();

    // Cross-MCP requires both jira + confluence — jira not present, confluence off
    expect(screen.queryByText('Cross-MCP query')).toBeNull();
  });

  it('calls onSelect when a bookmark is clicked', async () => {
    const caps = [makeCap('puat')];
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<BookmarkPrompts bookmarks={bookmarks} capabilities={caps} onSelect={onSelect} />);

    await user.click(screen.getByLabelText('Bookmark prompts'));
    await waitFor(() => {
      expect(screen.getByText('AFF vs FAS split')).toBeDefined();
    });
    await user.click(screen.getByText('AFF vs FAS split'));
    expect(onSelect).toHaveBeenCalledWith('What is the AFF vs FAS split?');
  });

  it('groups bookmarks by group label', async () => {
    const caps = [makeCap('puat')];
    const user = userEvent.setup();
    render(<BookmarkPrompts bookmarks={bookmarks} capabilities={caps} onSelect={vi.fn()} />);
    await user.click(screen.getByLabelText('Bookmark prompts'));

    await waitFor(() => {
      expect(screen.getByText('PUAT')).toBeDefined();
    });
    expect(screen.getByText('General')).toBeDefined();
  });
});
