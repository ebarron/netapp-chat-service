import { render, screen } from '../test-utils';
import { ModeToggle } from './ModeToggle';
import { vi, describe, it, expect } from 'vitest';

describe('ModeToggle', () => {
  const onChange = vi.fn();

  it('renders read-only and read-write options', () => {
    render(<ModeToggle mode="read-only" onChange={onChange} timeLeft={null} />);
    expect(screen.getByText('Read-Only')).toBeDefined();
    expect(screen.getByText('Read/Write')).toBeDefined();
  });

  it('does not show timer in read-only mode', () => {
    render(<ModeToggle mode="read-only" onChange={onChange} timeLeft={null} />);
    expect(screen.queryByText(/\dm/)).toBeNull();
  });

  it('shows timer in read-write mode', () => {
    render(<ModeToggle mode="read-write" onChange={onChange} timeLeft={300000} />);
    expect(screen.getByText('5m 0s')).toBeDefined();
  });

  it('formats seconds-only correctly', () => {
    render(<ModeToggle mode="read-write" onChange={onChange} timeLeft={45000} />);
    expect(screen.getByText('45s')).toBeDefined();
  });

  it('renders disabled state', () => {
    render(<ModeToggle mode="read-only" onChange={onChange} timeLeft={null} disabled />);
    // SegmentedControl should still render text.
    expect(screen.getByText('Read-Only')).toBeDefined();
  });
});
