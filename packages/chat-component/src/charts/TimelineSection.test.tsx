import { render, screen, userEvent } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { TimelineSection } from './TimelineSection';

describe('TimelineSection', () => {
  it('renders events with time and label', () => {
    render(
      <TimelineSection
        data={{
          events: [
            { time: '09:32', label: 'Alert fired', severity: 'critical' },
            { time: '09:35', label: 'Notification sent' },
          ],
        }}
      />
    );
    expect(screen.getByText('09:32')).toBeDefined();
    expect(screen.getByText('Alert fired')).toBeDefined();
    expect(screen.getByText('09:35')).toBeDefined();
    expect(screen.getByText('Notification sent')).toBeDefined();
  });

  it('shows all events when ≤10', () => {
    const events = Array.from({ length: 10 }, (_, i) => ({
      time: `${i}:00`,
      label: `Event ${i}`,
    }));
    render(<TimelineSection data={{ events }} />);
    for (let i = 0; i < 10; i++) {
      expect(screen.getByText(`Event ${i}`)).toBeDefined();
    }
    expect(screen.queryByText(/Show \d+ more/)).toBeNull();
  });

  it('collapses after 10 events with "Show N more" button', () => {
    const events = Array.from({ length: 15 }, (_, i) => ({
      time: `${i}:00`,
      label: `Event ${i}`,
    }));
    render(<TimelineSection data={{ events }} />);

    // First 10 visible
    for (let i = 0; i < 10; i++) {
      expect(screen.getByText(`Event ${i}`)).toBeDefined();
    }
    // Events 10-14 not visible
    expect(screen.queryByText('Event 10')).toBeNull();
    expect(screen.queryByText('Event 14')).toBeNull();

    // "Show 5 more" button present
    expect(screen.getByText('Show 5 more')).toBeDefined();
  });

  it('clicking "Show more" reveals remaining events', async () => {
    const user = userEvent.setup();
    const events = Array.from({ length: 15 }, (_, i) => ({
      time: `${i}:00`,
      label: `Event ${i}`,
    }));
    render(<TimelineSection data={{ events }} />);

    await user.click(screen.getByText('Show 5 more'));

    // All 15 events now visible
    for (let i = 0; i < 15; i++) {
      expect(screen.getByText(`Event ${i}`)).toBeDefined();
    }
    // Button gone
    expect(screen.queryByText('Show 5 more')).toBeNull();
  });

  it('renders empty events without crash', () => {
    render(<TimelineSection data={{ events: [] }} />);
  });
});
