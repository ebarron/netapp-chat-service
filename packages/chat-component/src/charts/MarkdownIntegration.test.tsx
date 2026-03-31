/**
 * Integration tests for the ReactMarkdown → chart/dashboard rendering pipeline.
 * Tests the code-fence handler registration by rendering markdown content
 * containing ```chart and ```dashboard blocks.
 */
import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ChartBlock } from './ChartBlock';
import { DashboardBlock } from './DashboardBlock';

/**
 * A standalone test wrapper that replicates the ReactMarkdown components config
 * from ChatPanel's MessageBubble, so we can test the integration without needing
 * the full ChatPanel + useChatPanel setup.
 */
function MarkdownWithCharts({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        code: ({ className, children, ...props }: React.HTMLAttributes<HTMLElement>) => {
          const text = String(children).replace(/\n$/, '');
          if (className === 'language-dashboard') {
            return <DashboardBlock json={text} />;
          }
          if (className === 'language-chart') {
            return <ChartBlock json={text} />;
          }
          return <code className={className} {...props}>{children}</code>;
        },
        pre: ({ children }: React.HTMLAttributes<HTMLPreElement>) => <>{children}</>,
      }}
    >
      {content}
    </ReactMarkdown>
  );
}

describe('Markdown chart integration', () => {
  it('renders a ```chart code fence as a chart block', () => {
    const md = [
      'Here is your data:',
      '',
      '```chart',
      JSON.stringify({
        type: 'stat',
        title: 'Total Capacity',
        value: '10.5 TB',
      }),
      '```',
      '',
      'That is the summary.',
    ].join('\n');

    render(<MarkdownWithCharts content={md} />);
    expect(screen.getByText('Total Capacity')).toBeDefined();
    expect(screen.getByText('10.5 TB')).toBeDefined();
    expect(screen.getByText('Here is your data:')).toBeDefined();
    expect(screen.getByText('That is the summary.')).toBeDefined();
  });

  it('renders a ```dashboard code fence as a dashboard block', () => {
    const dashboard = {
      title: 'Morning Report',
      panels: [
        { type: 'stat', title: 'Clusters', value: '3', width: 'half' },
        { type: 'stat', title: 'Volumes', value: '42', width: 'half' },
      ],
    };
    const md = [
      'Good morning! Here is your summary:',
      '',
      '```dashboard',
      JSON.stringify(dashboard),
      '```',
    ].join('\n');

    render(<MarkdownWithCharts content={md} />);
    expect(screen.getByText('Morning Report')).toBeDefined();
    expect(screen.getByText('Clusters')).toBeDefined();
    expect(screen.getByText('Volumes')).toBeDefined();
    expect(screen.getByText('Good morning! Here is your summary:')).toBeDefined();
  });

  it('falls back to plain code for unknown languages', () => {
    const md = [
      '```json',
      '{"key": "value"}',
      '```',
    ].join('\n');

    render(<MarkdownWithCharts content={md} />);
    expect(screen.getByText('{"key": "value"}')).toBeDefined();
  });

  it('falls back to code block for invalid chart JSON', () => {
    const md = [
      '```chart',
      'not valid json',
      '```',
    ].join('\n');

    render(<MarkdownWithCharts content={md} />);
    expect(screen.getByText('not valid json')).toBeDefined();
  });

  it('renders mixed markdown with multiple chart blocks', () => {
    const md = [
      '## Status',
      '',
      '```chart',
      JSON.stringify({ type: 'stat', title: 'First Stat', value: '100' }),
      '```',
      '',
      'Some text between charts.',
      '',
      '```chart',
      JSON.stringify({ type: 'gauge', title: 'Disk Use', value: 70, max: 100, unit: '%' }),
      '```',
    ].join('\n');

    render(<MarkdownWithCharts content={md} />);
    expect(screen.getByText('First Stat')).toBeDefined();
    expect(screen.getByText('100')).toBeDefined();
    expect(screen.getByText('Some text between charts.')).toBeDefined();
    expect(screen.getByText('Disk Use')).toBeDefined();
  });
});
