import { Paper, Text, Code } from '@mantine/core';
import type { ProposalData } from './chartTypes';

interface ProposalBlockProps {
  data: ProposalData;
}

export function ProposalBlock({ data }: ProposalBlockProps) {
  return (
    <Paper p="sm" radius="sm" withBorder role="note" aria-label={`Proposal: ${data.title}`}>
      <Text fw={500} fz="sm" mb="xs">
        {data.title}
      </Text>
      <Code block>{data.command}</Code>
    </Paper>
  );
}
