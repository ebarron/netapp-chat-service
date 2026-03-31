import { render, screen, userEvent, fireEvent } from '../../test-utils';
import { describe, it, expect, vi } from 'vitest';
import { ActionFormBlock } from './ActionFormBlock';
import type { ActionFormData } from './chartTypes';

/** Helper to pick a Mantine Select option by clicking. The dropdown renders in
 *  a portal with display:none (jsdom can't process CSS transitions), so we must
 *  query with { hidden: true } and use fireEvent instead of userEvent. */
async function pickSelectOption(user: ReturnType<typeof userEvent.setup>, name: string, option: string) {
  const input = screen.getByRole('textbox', { name });
  await user.click(input);
  const opt = screen.getByRole('option', { name: option, hidden: true });
  fireEvent.click(opt);
}

const baseData: ActionFormData = {
  type: 'action-form',
  fields: [
    { key: 'volume_name', label: 'Volume Name', type: 'text', placeholder: 'e.g. my_vol', required: true },
    { key: 'qos_policy', label: 'Performance Policy', type: 'select', placeholder: 'None', options: ['gold', 'silver', 'bronze'] },
    { key: 'export_policy', label: 'Export Policy', type: 'select', placeholder: 'None', options: ['default', 'nfs-open'] },
  ],
  submit: {
    label: 'Provision on cluster-east',
    tool: 'create_volume',
    params: { svm: 'svm_prod', aggregate: 'aggr1', size: '2TB' },
  },
  secondary: { label: 'Show other options', action: 'message', message: 'Show me provisioning options on other clusters.' },
  recheck: {
    label: 'Re-check Placement',
    fields: ['qos_policy'],
    message: 'Re-check provisioning for a {size} volume named {volume_name} with QoS policy {qos_policy}',
  },
};

describe('ActionFormBlock', () => {
  it('renders text field, select dropdowns, and buttons', () => {
    render(<ActionFormBlock data={baseData} />);
    expect(screen.getByLabelText('Volume Name')).toBeDefined();
    expect(screen.getAllByText('Performance Policy').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Export Policy').length).toBeGreaterThan(0);
    expect(screen.getByText('Provision on cluster-east')).toBeDefined();
    expect(screen.getByText('Show other options')).toBeDefined();
  });

  it('disables submit when required field is empty', () => {
    render(<ActionFormBlock data={baseData} />);
    const btn = screen.getByText('Provision on cluster-east').closest('button')!;
    expect(btn.disabled).toBe(true);
  });

  it('enables submit when required field is filled', async () => {
    const user = userEvent.setup();
    render(<ActionFormBlock data={baseData} />);
    await user.type(screen.getByLabelText('Volume Name'), 'test_vol');
    expect(screen.getByText('Provision on cluster-east').closest('button')!.disabled).toBe(false);
  });

  it('sends composed message via onAction on submit', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(<ActionFormBlock data={baseData} onAction={onAction} />);
    await user.type(screen.getByLabelText('Volume Name'), 'my_vol');
    await user.click(screen.getByText('Provision on cluster-east'));
    expect(onAction).toHaveBeenCalledWith(
      'Run create_volume with svm=svm_prod, aggregate=aggr1, size=2TB, volume_name=my_vol'
    );
  });

  it('pre-fills defaultValue', () => {
    const data: ActionFormData = {
      ...baseData,
      fields: [
        { key: 'volume_name', label: 'Volume Name', type: 'text', required: true, defaultValue: 'pre_filled' },
      ],
    };
    render(<ActionFormBlock data={data} />);
    expect((screen.getByLabelText('Volume Name') as HTMLInputElement).value).toBe('pre_filled');
    expect(screen.getByText('Provision on cluster-east').closest('button')!.disabled).toBe(false);
  });

  it('calls onAction for secondary button', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(<ActionFormBlock data={baseData} onAction={onAction} />);
    await user.click(screen.getByText('Show other options'));
    expect(onAction).toHaveBeenCalledWith('Show me provisioning options on other clusters.');
  });

  it('renders without secondary button', () => {
    const { secondary: _, recheck: _r, ...rest } = baseData;
    const data: ActionFormData = rest as ActionFormData;
    render(<ActionFormBlock data={data} />);
    expect(screen.getByText('Provision on cluster-east')).toBeDefined();
    expect(screen.queryByText('Show other options')).toBeNull();
  });

  it('does not send message when submit clicked with empty required field', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(<ActionFormBlock data={baseData} onAction={onAction} />);
    await user.click(screen.getByText('Provision on cluster-east'));
    expect(onAction).not.toHaveBeenCalled();
  });

  it('does not show recheck button initially', () => {
    render(<ActionFormBlock data={baseData} />);
    expect(screen.queryByText('Re-check Placement')).toBeNull();
  });

  it('shows recheck button when triggered select field changes', async () => {
    const user = userEvent.setup();
    render(<ActionFormBlock data={baseData} />);
    await pickSelectOption(user, 'Performance Policy', 'gold');
    expect(screen.getByText('Re-check Placement')).toBeDefined();
  });

  it('recheck button sends interpolated message', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(<ActionFormBlock data={baseData} onAction={onAction} />);
    await user.type(screen.getByLabelText('Volume Name'), 'my_vol');
    await pickSelectOption(user, 'Performance Policy', 'gold');
    await user.click(screen.getByText('Re-check Placement'));
    expect(onAction).toHaveBeenCalledWith(
      'Re-check provisioning for a 2TB volume named my_vol with QoS policy gold'
    );
  });

  it('includes select values in submit params when selected', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(<ActionFormBlock data={baseData} onAction={onAction} />);
    await user.type(screen.getByLabelText('Volume Name'), 'my_vol');
    await pickSelectOption(user, 'Export Policy', 'nfs-open');
    await user.click(screen.getByText('Provision on cluster-east'));
    expect(onAction).toHaveBeenCalledWith(
      'Run create_volume with svm=svm_prod, aggregate=aggr1, size=2TB, volume_name=my_vol, export_policy=nfs-open'
    );
  });

  describe('checkbox field type', () => {
    const checkboxData: ActionFormData = {
      type: 'action-form',
      fields: [
        { key: 'volume_name', label: 'Volume Name', type: 'text', required: true, defaultValue: 'my_vol' },
        { key: 'enable_monitoring', label: 'Enable Monitoring', type: 'checkbox', defaultValue: 'false' },
      ],
      submit: {
        label: 'Create Volume',
        tool: 'create_volume',
        params: { svm: 'svm_prod' },
      },
    };

    it('renders a switch for checkbox fields', () => {
      render(<ActionFormBlock data={checkboxData} />);
      expect(screen.getByRole('switch', { name: 'Enable Monitoring' })).toBeDefined();
    });

    it('defaults to unchecked when defaultValue is false', () => {
      render(<ActionFormBlock data={checkboxData} />);
      const toggle = screen.getByRole('switch', { name: 'Enable Monitoring' }) as HTMLInputElement;
      expect(toggle.checked).toBe(false);
    });

    it('defaults to checked when defaultValue is true', () => {
      const data: ActionFormData = {
        ...checkboxData,
        fields: [
          { key: 'volume_name', label: 'Volume Name', type: 'text', required: true, defaultValue: 'my_vol' },
          { key: 'enable_monitoring', label: 'Enable Monitoring', type: 'checkbox', defaultValue: 'true' },
        ],
      };
      render(<ActionFormBlock data={data} />);
      const toggle = screen.getByRole('switch', { name: 'Enable Monitoring' }) as HTMLInputElement;
      expect(toggle.checked).toBe(true);
    });

    it('toggles value on click', async () => {
      const user = userEvent.setup();
      render(<ActionFormBlock data={checkboxData} />);
      const toggle = screen.getByRole('switch', { name: 'Enable Monitoring' }) as HTMLInputElement;
      expect(toggle.checked).toBe(false);
      await user.click(toggle);
      expect(toggle.checked).toBe(true);
    });

    it('includes checkbox value in submit when checked', async () => {
      const user = userEvent.setup();
      const onAction = vi.fn();
      render(<ActionFormBlock data={checkboxData} onAction={onAction} />);
      await user.click(screen.getByRole('switch', { name: 'Enable Monitoring' }));
      await user.click(screen.getByText('Create Volume'));
      expect(onAction).toHaveBeenCalledWith(
        'Run create_volume with svm=svm_prod, volume_name=my_vol, enable_monitoring=true'
      );
    });

    it('omits checkbox value from submit when unchecked (value is false)', async () => {
      const user = userEvent.setup();
      const onAction = vi.fn();
      render(<ActionFormBlock data={checkboxData} onAction={onAction} />);
      await user.click(screen.getByText('Create Volume'));
      expect(onAction).toHaveBeenCalledWith(
        'Run create_volume with svm=svm_prod, volume_name=my_vol'
      );
    });
  });

  it('disables submit button when readOnly is true', () => {
    const data: ActionFormData = {
      ...baseData,
      fields: [
        { key: 'volume_name', label: 'Volume Name', type: 'text', required: true, defaultValue: 'filled' },
      ],
    };
    render(<ActionFormBlock data={data} readOnly />);
    const btn = screen.getByText('Provision on cluster-east').closest('button')!;
    expect(btn.disabled).toBe(true);
  });

  it('enables submit button when readOnly is false and required fields filled', () => {
    const data: ActionFormData = {
      ...baseData,
      fields: [
        { key: 'volume_name', label: 'Volume Name', type: 'text', required: true, defaultValue: 'filled' },
      ],
    };
    render(<ActionFormBlock data={data} readOnly={false} />);
    const btn = screen.getByText('Provision on cluster-east').closest('button')!;
    expect(btn.disabled).toBe(false);
  });
});
