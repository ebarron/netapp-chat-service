import { render, screen } from '../test-utils';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { CodeBlock } from './CodeBlock';

describe('CodeBlock', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders code inside a <pre> (Mantine Code block) and exposes a copy button', () => {
    render(<CodeBlock code={'line one\nline two'} language="bash" />);
    const pre = document.querySelector('pre');
    expect(pre).not.toBeNull();
    expect(pre!.textContent).toBe('line one\nline two');
    expect(screen.getByRole('button', { name: 'Copy code' })).toBeDefined();
  });

  it('copies the raw code text to the clipboard', async () => {
    render(<CodeBlock code="echo hello" language="bash" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Copy code' }));
    expect(await navigator.clipboard.readText()).toBe('echo hello');
  });
});
