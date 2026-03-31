import { render, screen } from '../../test-utils';
import { describe, it, expect } from 'vitest';
import { AutoJsonBlock } from './AutoJsonBlock';

describe('AutoJsonBlock', () => {
  describe('array of objects → table', () => {
    it('renders a table with auto-detected columns', () => {
      const json = JSON.stringify([
        { alertname: 'InstanceDown', severity: 'critical', instance: 'node-east-01:9090' },
        { alertname: 'HighCPU', severity: 'warning', instance: 'node-west-02:9090' },
      ]);
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Alertname')).toBeDefined();
      expect(screen.getByText('InstanceDown')).toBeDefined();
      expect(screen.getByText('HighCPU')).toBeDefined();
      expect(screen.getByText('node-east-01:9090')).toBeDefined();
    });

    it('unions columns across rows with different keys', () => {
      const json = JSON.stringify([
        { name: 'vol1', size: '100GB' },
        { name: 'vol2', protocol: 'NFS' },
      ]);
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Name')).toBeDefined();
      expect(screen.getByText('Size')).toBeDefined();
      expect(screen.getByText('Protocol')).toBeDefined();
    });
  });

  describe('object with items array → title + table', () => {
    it('renders items as a table with optional title', () => {
      const json = JSON.stringify({
        title: 'Active Alerts',
        items: [
          { alertname: 'InstanceDown', severity: 'critical', startsAt: '2026-01-07T10:30:00Z' },
          { alertname: 'DiskFailing', severity: 'warning', startsAt: '2026-01-07T09:00:00Z' },
        ],
      });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Active Alerts')).toBeDefined();
      expect(screen.getByText('InstanceDown')).toBeDefined();
      expect(screen.getByText('DiskFailing')).toBeDefined();
    });

    it('renders without title when not present', () => {
      const json = JSON.stringify({
        items: [{ name: 'vol1', state: 'online' }],
      });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('vol1')).toBeDefined();
      expect(screen.getByText('online')).toBeDefined();
    });
  });

  describe('flat object → key-value properties', () => {
    it('renders key-value pairs with formatted labels', () => {
      const json = JSON.stringify({
        cluster_name: 'prod-east',
        node_count: 4,
        is_healthy: true,
      });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Cluster Name')).toBeDefined();
      expect(screen.getByText('prod-east')).toBeDefined();
      expect(screen.getByText('Node Count')).toBeDefined();
      expect(screen.getByText('4')).toBeDefined();
      expect(screen.getByText('Is Healthy')).toBeDefined();
      expect(screen.getByText('Yes')).toBeDefined();
    });

    it('formats camelCase keys as Title Case', () => {
      const json = JSON.stringify({ volumeName: 'vol_prod_01', totalSize: '500GB' });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Volume Name')).toBeDefined();
      expect(screen.getByText('Total Size')).toBeDefined();
    });

    it('renders null values as dash', () => {
      const json = JSON.stringify({ name: 'test', value: null });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('—')).toBeDefined();
    });

    it('renders boolean false as No', () => {
      const json = JSON.stringify({ name: 'test', enabled: false });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('No')).toBeDefined();
    });
  });

  describe('edge cases', () => {
    it('renders invalid JSON as code block', () => {
      render(<AutoJsonBlock json="not valid json {{{" />);
      expect(screen.getByText('not valid json {{{')).toBeDefined();
    });

    it('renders a primitive JSON value as text', () => {
      render(<AutoJsonBlock json='"hello world"' />);
      expect(screen.getByText('hello world')).toBeDefined();
    });

    it('renders an array of primitives as comma-separated text', () => {
      render(<AutoJsonBlock json='[1, 2, 3, 4, 5]' />);
      expect(screen.getByText('1, 2, 3, 4, 5')).toBeDefined();
    });

    it('renders nested objects as stringified values', () => {
      const json = JSON.stringify({ name: 'test', config: { timeout: 30, retries: 3 } });
      render(<AutoJsonBlock json={json} />);
      expect(screen.getByText('Config')).toBeDefined();
      // nested object is stringified
      expect(screen.getByText('{"timeout":30,"retries":3}')).toBeDefined();
    });

    it('caps table rows at 200', () => {
      const rows = Array.from({ length: 250 }, (_, i) => ({ id: i, name: `item-${i}` }));
      render(<AutoJsonBlock json={JSON.stringify(rows)} />);
      // Should render 200 rows (plus thead row), not 250
      const tableRows = screen.getAllByRole('row');
      // +1 for header row
      expect(tableRows.length).toBe(201);
    });
  });
});
