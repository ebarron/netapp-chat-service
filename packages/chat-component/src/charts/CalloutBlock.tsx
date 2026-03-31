import { Paper, Text } from '@mantine/core';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { CalloutData } from './chartTypes';

interface CalloutBlockProps {
  data: CalloutData;
}

export function CalloutBlock({ data }: CalloutBlockProps) {
  return (
    <Paper
      p="sm"
      radius="sm"
      withBorder
      role="note"
      aria-label={data.title}
      style={{
        borderLeft: '4px solid var(--mantine-color-blue-6)',
      }}
    >
      <Text fw={500} fz="sm" mb={4}>
        {data.icon ? `${data.icon} ` : ''}
        {data.title}
      </Text>
      <div style={{ fontSize: 'var(--mantine-font-size-sm)' }}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {data.body}
        </ReactMarkdown>
      </div>
    </Paper>
  );
}
