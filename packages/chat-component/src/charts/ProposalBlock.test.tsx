import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { ProposalBlock } from './ProposalBlock';
import type { ProposalData } from './chartTypes';

describe('ProposalBlock', () => {
  it('renders title and command', () => {
    const data: ProposalData = {
      type: 'proposal',
      title: 'Create Volume',
      command: 'volume create -vserver svm1 -volume new_vol -aggregate aggr1 -size 2TB',
    };
    render(<ProposalBlock data={data} />);
    expect(screen.getByText('Create Volume')).toBeDefined();
    expect(
      screen.getByText('volume create -vserver svm1 -volume new_vol -aggregate aggr1 -size 2TB')
    ).toBeDefined();
  });

  it('renders a multiline command', () => {
    const data: ProposalData = {
      type: 'proposal',
      title: 'Multi-Step',
      command: 'step1\nstep2\nstep3',
    };
    render(<ProposalBlock data={data} />);
    expect(screen.getByText('Multi-Step')).toBeDefined();
  });
});
