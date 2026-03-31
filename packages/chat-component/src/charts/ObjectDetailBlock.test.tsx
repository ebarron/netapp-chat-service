import { render, screen } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { ObjectDetailBlock, applyQualifier } from './ObjectDetailBlock';

const minimalDetail = {
  type: 'object-detail',
  kind: 'alert',
  name: 'InstanceDown — node-east-01',
  status: 'critical',
  subtitle: 'Firing since 2025-06-14 09:32 UTC (4h 28m)',
  sections: [],
};

describe('applyQualifier', () => {
  it('appends card qualifier when no per-item qualifier is set', () => {
    expect(applyQualifier('Show vol1', undefined, 'on SVM svm1 on cluster cls1'))
      .toBe('Show vol1 on SVM svm1 on cluster cls1');
  });

  it('uses per-item qualifier instead of card qualifier', () => {
    expect(applyQualifier('Tell me about SVM svm1', 'on cluster cls1', 'on SVM svm1 on cluster cls1'))
      .toBe('Tell me about SVM svm1 on cluster cls1');
  });

  it('suppresses qualifier when per-item qualifier is empty string', () => {
    expect(applyQualifier('Show cluster cls1', '', 'on SVM svm1 on cluster cls1'))
      .toBe('Show cluster cls1');
  });

  it('does not duplicate qualifier already in message', () => {
    expect(applyQualifier('Show vol1 on SVM svm1 on cluster cls1', undefined, 'on SVM svm1 on cluster cls1'))
      .toBe('Show vol1 on SVM svm1 on cluster cls1');
  });

  it('returns message unchanged when both qualifiers are undefined', () => {
    expect(applyQualifier('Show vol1', undefined, undefined)).toBe('Show vol1');
  });
});

describe('ObjectDetailBlock', () => {
  it('renders identity header with name, status badge, and subtitle', () => {
    render(<ObjectDetailBlock json={JSON.stringify(minimalDetail)} />);
    expect(screen.getByText('InstanceDown — node-east-01')).toBeDefined();
    expect(screen.getByText('critical')).toBeDefined();
    expect(screen.getByText('Firing since 2025-06-14 09:32 UTC (4h 28m)')).toBeDefined();
  });

  it('renders article role with name as label', () => {
    render(<ObjectDetailBlock json={JSON.stringify(minimalDetail)} />);
    expect(screen.getByRole('article', { name: 'InstanceDown — node-east-01' })).toBeDefined();
  });

  it('renders without status badge when status is absent', () => {
    const data = { ...minimalDetail, status: undefined };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('InstanceDown — node-east-01')).toBeDefined();
    expect(screen.queryByText('critical')).toBeNull();
  });

  it('renders empty sections array without crash', () => {
    render(<ObjectDetailBlock json={JSON.stringify(minimalDetail)} />);
    // Just name/status rendered, no sections
    expect(screen.getByText('InstanceDown — node-east-01')).toBeDefined();
  });

  it('falls back to code block on invalid JSON', () => {
    render(<ObjectDetailBlock json="not valid json" />);
    expect(screen.getByText('not valid json')).toBeDefined();
  });

  it('renders sections in order', () => {
    const data = {
      ...minimalDetail,
      sections: [
        { title: 'First Section', layout: 'text', data: { body: 'Hello' } },
        { title: 'Second Section', layout: 'text', data: { body: 'World' } },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    const first = screen.getByText('First Section');
    const second = screen.getByText('Second Section');
    // Verify DOM order
    expect(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('renders properties section with label/value pairs', () => {
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Details',
          layout: 'properties',
          data: {
            columns: 2,
            items: [
              { label: 'Severity', value: 'high', color: 'red' },
              { label: 'Cluster', value: 'cluster-east', link: 'Tell me about cluster-east' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('Severity')).toBeDefined();
    expect(screen.getByText('high')).toBeDefined();
    expect(screen.getByText('Cluster')).toBeDefined();
    expect(screen.getByText('cluster-east')).toBeDefined();
  });

  it('renders text section with markdown', () => {
    const data = {
      ...minimalDetail,
      sections: [
        { title: 'Notes', layout: 'text', data: { body: '**Bold** text' } },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('Bold')).toBeDefined();
  });

  it('renders actions section with buttons', () => {
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Actions',
          layout: 'actions',
          data: {
            buttons: [
              { label: 'Investigate', action: 'message', message: 'Investigate node' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('Investigate')).toBeDefined();
  });

  it('renders timeline section with events', () => {
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Timeline',
          layout: 'timeline',
          data: {
            events: [
              { time: '09:32', label: 'Alert fired', severity: 'critical' },
              { time: '09:35', label: 'Notification sent' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('Alert fired')).toBeDefined();
    expect(screen.getByText('Notification sent')).toBeDefined();
  });

  it('renders alert-list section', () => {
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Active Alerts',
          layout: 'alert-list',
          data: {
            items: [
              { severity: 'critical', message: 'InstanceDown', time: '4h ago' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('InstanceDown')).toBeDefined();
  });

  it('renders table section', () => {
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Notifications',
          layout: 'table',
          data: {
            title: 'Notifications',
            columns: ['Time', 'Channel'],
            rows: [{ name: 'r1', Time: '09:33', Channel: 'Slack' }],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} />);
    expect(screen.getByText('Slack')).toBeDefined();
  });

  it('passes onAction to child components', () => {
    const onAction = vi.fn();
    const data = {
      ...minimalDetail,
      sections: [
        {
          title: 'Actions',
          layout: 'actions',
          data: {
            buttons: [
              { label: 'Do it', action: 'message', message: 'do something' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    screen.getByText('Do it').click();
    expect(onAction).toHaveBeenCalledWith('do something');
  });

  it('enriches action messages with qualifier from object-detail', () => {
    const onAction = vi.fn();
    const data = {
      type: 'object-detail',
      kind: 'volume',
      name: 'vol1',
      qualifier: 'on SVM svm1 on cluster cls1',
      sections: [
        {
          title: 'Properties',
          layout: 'properties',
          data: {
            items: [
              { label: 'Cluster', value: 'cls1' },
              { label: 'SVM', value: 'svm1' },
              { label: 'State', value: 'online' },
            ],
          },
        },
        {
          title: 'Actions',
          layout: 'actions',
          data: {
            buttons: [
              { label: 'Show snapshots', action: 'message', message: 'Show snapshots for volume vol1' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    screen.getByText('Show snapshots').click();
    expect(onAction).toHaveBeenCalledWith(
      'Show snapshots for volume vol1 on SVM svm1 on cluster cls1'
    );
  });

  it('does not duplicate qualifier already present in action message', () => {
    const onAction = vi.fn();
    const data = {
      type: 'object-detail',
      kind: 'volume',
      name: 'vol1',
      qualifier: 'on SVM svm1 on cluster cls1',
      sections: [
        {
          title: 'Actions',
          layout: 'actions',
          data: {
            buttons: [
              { label: 'Resize', action: 'message', message: 'Resize volume vol1 on SVM svm1 on cluster cls1' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    screen.getByText('Resize').click();
    expect(onAction).toHaveBeenCalledWith(
      'Resize volume vol1 on SVM svm1 on cluster cls1'
    );
  });

  it('enriches property link clicks with qualifier', () => {
    const onAction = vi.fn();
    const data = {
      type: 'object-detail',
      kind: 'volume',
      name: 'vol1',
      qualifier: 'on SVM svm1 on cluster cls1',
      sections: [
        {
          title: 'Properties',
          layout: 'properties',
          data: {
            items: [
              { label: 'Cluster', value: 'cls1' },
              { label: 'SVM', value: 'svm1' },
              { label: 'Aggregate', value: 'aggr1', link: 'Tell me about aggregate aggr1' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    screen.getByText('aggr1').click();
    expect(onAction).toHaveBeenCalledWith(
      'Tell me about aggregate aggr1 on SVM svm1 on cluster cls1'
    );
  });

  it('uses per-item qualifier override on property links', () => {
    const onAction = vi.fn();
    const data = {
      type: 'object-detail',
      kind: 'volume',
      name: 'vol1',
      qualifier: 'on SVM svm1 on cluster cls1',
      sections: [
        {
          title: 'Properties',
          layout: 'properties',
          data: {
            items: [
              { label: 'Cluster', value: 'cls1', link: 'Show cluster cls1', qualifier: '' },
              { label: 'SVM', value: 'svm1', link: 'Tell me about SVM svm1', qualifier: 'on cluster cls1' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    // Cluster link with qualifier="" should NOT append any qualifier
    screen.getByText('cls1').click();
    expect(onAction).toHaveBeenCalledWith('Show cluster cls1');
    // SVM link with qualifier="on cluster cls1" should use that instead of the card qualifier
    screen.getByText('svm1').click();
    expect(onAction).toHaveBeenCalledWith('Tell me about SVM svm1 on cluster cls1');
  });

  it('uses per-item qualifier override on action buttons', () => {
    const onAction = vi.fn();
    const data = {
      type: 'object-detail',
      kind: 'volume',
      name: 'vol1',
      qualifier: 'on SVM svm1 on cluster cls1',
      sections: [
        {
          title: 'Actions',
          layout: 'actions',
          data: {
            buttons: [
              { label: 'Show snapshots', action: 'message', message: 'Show snapshots for vol1' },
              { label: 'Check cluster', action: 'message', message: 'Show cluster cls1', qualifier: '' },
            ],
          },
        },
      ],
    };
    render(<ObjectDetailBlock json={JSON.stringify(data)} onAction={onAction} />);
    // Button without per-item qualifier inherits card qualifier
    screen.getByText('Show snapshots').click();
    expect(onAction).toHaveBeenCalledWith('Show snapshots for vol1 on SVM svm1 on cluster cls1');
    // Button with qualifier="" suppresses the card qualifier
    screen.getByText('Check cluster').click();
    expect(onAction).toHaveBeenCalledWith('Show cluster cls1');
  });
});
