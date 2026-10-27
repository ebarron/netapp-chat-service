import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { render } from '../test-utils';
import {
  ChatRenderExtensionProvider,
  createPanelExtensionRegistry,
  type PanelExtensionDefinition,
} from './PanelExtensions';
import { DashboardBlock } from './charts/DashboardBlock';
import { normalizeDashboardPayload } from './charts/chartTypes';

interface Availability {
  unavailableCount: number;
  label: string;
}

function normalizeAvailability(value: unknown): Availability | null {
  if (
    typeof value !== 'object'
    || value === null
    || Array.isArray(value)
    || typeof (value as Record<string, unknown>).unavailableCount !== 'number'
    || typeof (value as Record<string, unknown>).label !== 'string'
  ) {
    return null;
  }
  return {
    unavailableCount: (value as Record<string, unknown>).unavailableCount as number,
    label: (value as Record<string, unknown>).label as string,
  };
}

const availabilityExtension: PanelExtensionDefinition<Availability> = {
  kind: 'example.availability',
  schemaVersion: 1,
  normalize: normalizeAvailability,
  render: ({ data, context }) => (
    <section aria-label="Custom availability">
      {data.label}: {data.unavailableCount}
      <span>{context.readOnly ? 'read only' : 'read write'}</span>
      {context.sendMessage && (
        <button onClick={() => context.sendMessage?.('show eligible classes')}>
          Ask
        </button>
      )}
    </section>
  ),
};

function dashboardJson(
  extension?: Record<string, unknown>,
  extraPanel?: Record<string, unknown>,
): string {
  return JSON.stringify({
    title: 'Extensions',
    panels: [
      { type: 'stat', title: 'Existing stat', value: '7' },
      {
        type: 'callout',
        title: 'Built-in fallback',
        body: 'Validated fallback body',
        ...(extension ? { extension } : {}),
      },
      ...(extraPanel ? [extraPanel] : []),
    ],
  });
}

describe('panel extension registry', () => {
  it('renders existing panels and old-style callouts unchanged', () => {
    render(<DashboardBlock json={dashboardJson()} />);

    expect(screen.getByText('Existing stat')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
    expect(screen.getByRole('note', { name: 'Built-in fallback' })).toBeInTheDocument();
    expect(screen.getByText('Validated fallback body')).toBeInTheDocument();
  });

  it('renders the built-in fallback when no provider is present', () => {
    render(
      <DashboardBlock
        json={dashboardJson({
          kind: 'example.availability',
          schemaVersion: 1,
          data: { unavailableCount: 2, label: 'Unavailable' },
        })}
      />,
    );

    expect(screen.getByRole('note', { name: 'Built-in fallback' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Custom availability' })).not.toBeInTheDocument();
  });

  it.each([
    ['unknown kind', 'other.kind', 1, { unavailableCount: 2, label: 'Unavailable' }],
    ['unknown version', 'example.availability', 2, { unavailableCount: 2, label: 'Unavailable' }],
    ['bad data', 'example.availability', 1, { unavailableCount: 'two', label: 'Unavailable' }],
  ])('renders the fallback for %s', (_name, kind, schemaVersion, data) => {
    const registry = createPanelExtensionRegistry([availabilityExtension]);
    render(
      <ChatRenderExtensionProvider registry={registry}>
        <DashboardBlock
          json={dashboardJson({ kind, schemaVersion, data })}
        />
      </ChatRenderExtensionProvider>,
    );

    expect(screen.getByRole('note', { name: 'Built-in fallback' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Custom availability')).not.toBeInTheDocument();
  });

  it('renders only normalized data with the safe context', () => {
    const onAction = vi.fn();
    const registry = createPanelExtensionRegistry([availabilityExtension]);
    render(
      <ChatRenderExtensionProvider registry={registry}>
        <DashboardBlock
          readOnly
          onAction={onAction}
          json={dashboardJson({
            kind: 'example.availability',
            schemaVersion: 1,
            data: {
              unavailableCount: 2,
              label: 'Unavailable',
              ignoredWireField: 'must not reach renderer',
            },
          })}
        />
      </ChatRenderExtensionProvider>,
    );

    expect(screen.getByLabelText('Custom availability')).toHaveTextContent(
      'Unavailable: 2read only',
    );
    expect(screen.queryByText('must not reach renderer')).not.toBeInTheDocument();
    expect(screen.queryByRole('note', { name: 'Built-in fallback' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Ask' }));
    expect(onAction).toHaveBeenCalledWith('show eligible classes');
  });

  it('isolates renderer exceptions and deterministically restores the fallback', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const registry = createPanelExtensionRegistry([{
      ...availabilityExtension,
      render: () => {
        throw new Error('host renderer failed');
      },
    }]);
    render(
      <ChatRenderExtensionProvider registry={registry}>
        <DashboardBlock
          json={dashboardJson({
            kind: 'example.availability',
            schemaVersion: 1,
            data: { unavailableCount: 2, label: 'Unavailable' },
          })}
        />
      </ChatRenderExtensionProvider>,
    );

    expect(screen.getByRole('note', { name: 'Built-in fallback' })).toBeInTheDocument();
    consoleError.mockRestore();
  });

  it('rejects duplicate kind and schema-version registrations clearly', () => {
    expect(() => createPanelExtensionRegistry([
      availabilityExtension,
      { ...availabilityExtension },
    ])).toThrow(
      'Duplicate panel extension registration: "example.availability" v1',
    );
  });

  it('keeps malformed metadata optional under strict canvas validation', () => {
    const result = normalizeDashboardPayload({
      title: 'Fallback first',
      panels: [{
        type: 'callout',
        title: 'Built-in fallback',
        body: 'Validated fallback body',
        extension: {
          kind: 'example.availability',
          schemaVersion: 'one',
          data: {},
        },
      }],
    });

    expect(result.value).not.toBeNull();
    expect(result.value!.panels[0]).not.toHaveProperty('extension');
    expect(result.issues).toContainEqual({
      code: 'invalid_panel_extension',
      path: '$.panels[0].extension',
      severity: 'warning',
    });
  });

  it('never forwards extension metadata to Markdown or another built-in panel', () => {
    const registry = createPanelExtensionRegistry([availabilityExtension]);
    const json = dashboardJson({
      kind: 'example.availability',
      schemaVersion: 1,
      data: {
        unavailableCount: 'invalid',
        label: '<img src=x onerror=alert(1)>',
      },
    }, {
      type: 'callout',
      title: 'Second callout',
      body: 'Ordinary content',
    });
    const { container } = render(
      <ChatRenderExtensionProvider registry={registry}>
        <DashboardBlock json={json} />
      </ChatRenderExtensionProvider>,
    );

    expect(screen.getByText('Validated fallback body')).toBeInTheDocument();
    expect(screen.getByText('Ordinary content')).toBeInTheDocument();
    expect(container.querySelector('img')).toBeNull();
    expect(container).not.toHaveTextContent('onerror');
  });
});
