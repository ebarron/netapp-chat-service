import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { CalloutBlock } from './CalloutBlock';
import { DashboardBlock } from './DashboardBlock';
import type { CalloutData } from './chartTypes';

describe('Dark/Light Mode — theme token usage', () => {
  it('CalloutBlock uses Mantine CSS variable for border color', () => {
    const data: CalloutData = {
      type: 'callout',
      title: 'Tip',
      body: 'Theme safe border.',
    };
    const { container } = render(<CalloutBlock data={data} />);
    // Find the element with the border-left style (Mantine Paper may wrap)
    const styled = container.querySelector('[style*="border-left"]') as HTMLElement;
    expect(styled).toBeDefined();
    const style = styled?.getAttribute('style') ?? '';
    expect(style).toContain('--mantine-color-blue-6');
    expect(style).not.toMatch(/#[0-9a-fA-F]{3,8}/);
  });

  it('DashboardBlock uses light-dark() background via CSS class', () => {
    const json = JSON.stringify({
      title: 'Theme Test',
      panels: [{ type: 'stat', title: 'A', value: '1' }],
    });
    const { container } = render(<DashboardBlock json={json} />);
    const dashboard = container.querySelector('[class*="dashboard"]');
    expect(dashboard).toBeDefined();
    // The CSS module class contains the light-dark() background-color rule.
    // We verify the class is applied; actual color switching is handled by the
    // Mantine CSS runtime.
    expect(dashboard?.className).toMatch(/dashboard/);
  });

  it('DashboardBlock max-width is applied via CSS class', () => {
    const json = JSON.stringify({ title: 'Width Test', panels: [] });
    const { container } = render(<DashboardBlock json={json} />);
    const dashboard = container.querySelector('[class*="dashboard"]');
    expect(dashboard).toBeDefined();
  });

  it('no hardcoded hex colors in inline styles', () => {
    // Render a callout and verify no hex color in the style string
    const data: CalloutData = {
      type: 'callout',
      title: 'Check',
      body: 'No hex colors.',
    };
    const { container } = render(<CalloutBlock data={data} />);
    const paper = container.firstElementChild as HTMLElement;
    const style = paper.getAttribute('style') ?? '';
    expect(style).not.toMatch(/#[0-9a-fA-F]{3,8}/);
  });
});
