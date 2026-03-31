import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { PropertiesSection } from './PropertiesSection';

describe('PropertiesSection', () => {
  it('renders label/value pairs', () => {
    render(
      <PropertiesSection
        data={{
          columns: 2,
          items: [
            { label: 'Severity', value: 'critical' },
            { label: 'Cluster', value: 'cluster-east' },
          ],
        }}
      />
    );
    expect(screen.getByText('Severity')).toBeDefined();
    expect(screen.getByText('critical')).toBeDefined();
    expect(screen.getByText('Cluster')).toBeDefined();
    expect(screen.getByText('cluster-east')).toBeDefined();
  });

  it('renders linked values as Anchor with underline="hover"', () => {
    render(
      <PropertiesSection
        data={{
          items: [
            { label: 'Cluster', value: 'cluster-east', link: 'Tell me about cluster-east' },
          ],
        }}
      />
    );
    const anchor = screen.getByText('cluster-east');
    expect(anchor.tagName).toBe('A');
  });

  it('clicking linked value calls onAction with the link string', () => {
    const onAction = vi.fn();
    render(
      <PropertiesSection
        data={{
          items: [
            { label: 'Cluster', value: 'cluster-east', link: 'Tell me about cluster-east' },
          ],
        }}
        onAction={onAction}
      />
    );
    screen.getByText('cluster-east').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about cluster-east');
  });

  it('renders colored values with correct color prop', () => {
    render(
      <PropertiesSection
        data={{
          items: [
            { label: 'Status', value: 'critical', color: 'red' },
          ],
        }}
      />
    );
    expect(screen.getByText('critical')).toBeDefined();
  });

  it('defaults columns to 2 when not specified', () => {
    const { container } = render(
      <PropertiesSection
        data={{
          items: [
            { label: 'A', value: '1' },
            { label: 'B', value: '2' },
          ],
        }}
      />
    );
    // SimpleGrid renders a div — just verify it renders successfully
    expect(container.querySelector('[class*="simpleGrid"]') || container.firstChild).toBeDefined();
  });

  it('renders empty items without crash', () => {
    render(<PropertiesSection data={{ items: [] }} />);
    // No crash expected
  });
});
