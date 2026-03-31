/**
 * Realistic JSON fixtures for the three lighthouse interests.
 * These serve as golden examples for visual testing and regression baselines.
 * Ref: spec §4.8 lighthouse interests (morning-coffee, resource-status, volume-provision)
 */

export const morningCoffee = {
  title: 'Good Morning — Fleet Overview',
  toggle: { label: 'Show Detailed', message: 'show me a per cluster view of my fleet' },
  panels: [
    {
      type: 'alert-summary' as const,
      title: 'Alert Counts',
      width: 'half' as const,
      data: { critical: 1, warning: 4, info: 2, ok: 47 },
    },
    {
      type: 'stat' as const,
      title: 'Clusters Online',
      width: 'half' as const,
      value: '3 / 3',
      subtitle: 'All clusters reporting',
      trend: 'flat' as const,
    },
    {
      type: 'area' as const,
      title: 'Aggregate Usage — Last 7 Days',
      width: 'full' as const,
      xKey: 'day',
      yLabel: 'Avg Used %',
      series: [
        { key: 'cluster_east', label: 'cluster-east', color: 'blue' },
        { key: 'cluster_west', label: 'cluster-west', color: 'teal' },
        { key: 'cluster_dr', label: 'cluster-dr', color: 'gray' },
      ],
      data: [
        { day: 'Mon', cluster_east: 62, cluster_west: 55, cluster_dr: 30 },
        { day: 'Tue', cluster_east: 63, cluster_west: 56, cluster_dr: 30 },
        { day: 'Wed', cluster_east: 64, cluster_west: 57, cluster_dr: 31 },
        { day: 'Thu', cluster_east: 65, cluster_west: 57, cluster_dr: 31 },
        { day: 'Fri', cluster_east: 66, cluster_west: 58, cluster_dr: 32 },
        { day: 'Sat', cluster_east: 66, cluster_west: 58, cluster_dr: 32 },
        { day: 'Sun', cluster_east: 67, cluster_west: 59, cluster_dr: 32 },
      ],
    },
    {
      type: 'resource-table' as const,
      title: 'Volumes Needing Attention',
      width: 'full' as const,
      columns: ['Name', 'Cluster', 'Capacity', 'IOPS', 'Status'],
      rows: [
        { name: 'vol_logs', Name: 'vol_logs', Cluster: 'cluster-east', Capacity: '95.0%', IOPS: '320', Status: 'Critical', capacity_trend: [90, 91, 92, 93, 94, 95, 95], iops_trend: [280, 290, 300, 310, 315, 318, 320] },
        { name: 'vol_data02', Name: 'vol_data02', Cluster: 'cluster-east', Capacity: '87.0%', IOPS: '1100', Status: 'Warning', capacity_trend: [83, 84, 85, 86, 86, 87, 87], iops_trend: [950, 980, 1000, 1030, 1060, 1080, 1100] },
        { name: 'vol_archive', Name: 'vol_archive', Cluster: 'cluster-west', Capacity: '84.0%', IOPS: '45', Status: 'Warning', capacity_trend: [80, 81, 82, 83, 83, 84, 84], iops_trend: [40, 42, 43, 44, 44, 45, 45] },
      ],
    },
    {
      type: 'alert-list' as const,
      title: 'Recent Alerts',
      width: 'full' as const,
      items: [
        { severity: 'critical' as const, message: 'vol_logs on cluster-east is at 95% capacity', time: '12 min ago' },
        { severity: 'warning' as const, message: 'aggr1 on cluster-east at 81% — approaching threshold', time: '45 min ago' },
        { severity: 'warning' as const, message: 'vol_data02 at 87% capacity', time: '2 hrs ago' },
        { severity: 'info' as const, message: 'ONTAP 9.15.1 firmware update available for cluster-west', time: '6 hrs ago' },
      ],
    },
    {
      type: 'callout' as const,
      icon: '💡',
      title: 'Recommendation',
      width: 'full' as const,
      body: 'vol_logs on cluster-east is at 95% and growing ~1.5%/day. At this rate it will fill in ~3 days. Consider expanding or migrating older data.',
    },
    {
      type: 'action-button' as const,
      width: 'full' as const,
      buttons: [
        { label: 'Show all alerts', action: 'message' as const, message: 'Show all active alerts across all clusters', variant: 'outline' as const },
        { label: 'Check vol_logs', action: 'message' as const, message: 'Tell me more about vol_logs on cluster-east' },
        { label: 'Check capacities', action: 'message' as const, message: 'Show all volumes over 80% capacity' },
      ],
    },
  ],
};

export const resourceStatus = {
  title: 'Resource Status — vol_prod_db01',
  panels: [
    {
      type: 'stat' as const,
      title: 'Volume State',
      width: 'third' as const,
      value: 'Online',
    },
    {
      type: 'gauge' as const,
      title: 'Capacity Used',
      width: 'third' as const,
      value: 78,
      max: 100,
      unit: '%',
      thresholds: { warning: 80, critical: 95 },
    },
    {
      type: 'stat' as const,
      title: 'Avg Latency',
      width: 'third' as const,
      value: '1.8 ms',
      trend: 'down' as const,
      trendValue: '-12%',
    },
    {
      type: 'area' as const,
      title: 'IOPS — Last 24 Hours',
      width: 'half' as const,
      xKey: 'hour',
      series: [
        { key: 'read', label: 'Read', color: 'blue' },
        { key: 'write', label: 'Write', color: 'violet' },
      ],
      data: [
        { hour: '00:00', read: 420, write: 180 },
        { hour: '04:00', read: 150, write: 60 },
        { hour: '08:00', read: 800, write: 340 },
        { hour: '12:00', read: 1100, write: 520 },
        { hour: '16:00', read: 950, write: 410 },
        { hour: '20:00', read: 600, write: 250 },
      ],
    },
    {
      type: 'area' as const,
      title: 'Throughput — Last 24 Hours',
      width: 'half' as const,
      xKey: 'hour',
      yLabel: 'MB/s',
      series: [
        { key: 'read_mbps', label: 'Read MB/s', color: 'teal' },
        { key: 'write_mbps', label: 'Write MB/s', color: 'orange' },
      ],
      data: [
        { hour: '00:00', read_mbps: 85, write_mbps: 35 },
        { hour: '04:00', read_mbps: 30, write_mbps: 12 },
        { hour: '08:00', read_mbps: 160, write_mbps: 70 },
        { hour: '12:00', read_mbps: 220, write_mbps: 105 },
        { hour: '16:00', read_mbps: 190, write_mbps: 80 },
        { hour: '20:00', read_mbps: 120, write_mbps: 50 },
      ],
    },
    {
      type: 'status-grid' as const,
      title: 'Related Components',
      width: 'full' as const,
      items: [
        { name: 'aggr1_east', status: 'ok' as const },
        { name: 'svm_prod', status: 'ok' as const },
        { name: 'lif_data1', status: 'ok' as const },
        { name: 'lif_data2', status: 'warning' as const, detail: 'high utilization' },
        { name: 'snapshot_policy', status: 'ok' as const },
      ],
    },
    {
      type: 'callout' as const,
      title: 'Summary',
      width: 'full' as const,
      body: 'vol_prod_db01 is online and healthy at 78% capacity. IOPS are within normal range. lif_data2 shows elevated utilization — worth monitoring.',
    },
    {
      type: 'action-button' as const,
      width: 'full' as const,
      buttons: [
        { label: 'Show snapshots', action: 'message' as const, message: 'Show snapshots for vol_prod_db01' },
        { label: 'Check aggregate', action: 'message' as const, message: 'Tell me about aggr1_east' },
      ],
    },
  ],
};

export const volumeProvision = {
  title: 'Volume Provisioning — 2 TB NFS High-Performance',
  panels: [
    {
      type: 'callout' as const,
      icon: '📋',
      title: 'Provisioning Requirements',
      width: 'full' as const,
      body: 'Protocol: NFS • Size: 2 TB • Tier: High-Performance (SSD) • QoS: Adaptive',
    },
    {
      type: 'resource-table' as const,
      title: 'Candidate Aggregates',
      width: 'full' as const,
      columns: ['Name', 'Cluster', 'Free', 'Disk Type', 'Score'],
      rows: [
        { name: 'aggr_ssd_east01', Name: 'aggr_ssd_east01', Cluster: 'cluster-east', Free: '4.2 TB', 'Disk Type': 'SSD', Score: '★★★' },
        { name: 'aggr_ssd_east02', Name: 'aggr_ssd_east02', Cluster: 'cluster-east', Free: '3.1 TB', 'Disk Type': 'SSD', Score: '★★☆' },
        { name: 'aggr_ssd_west01', Name: 'aggr_ssd_west01', Cluster: 'cluster-west', Free: '5.8 TB', 'Disk Type': 'SSD', Score: '★★☆' },
      ],
    },
    {
      type: 'bar' as const,
      title: 'Available Space by Aggregate',
      width: 'full' as const,
      xKey: 'name',
      series: [{ key: 'free_tb', label: 'Free TB', color: 'teal' }],
      data: [
        { name: 'aggr_ssd_east01', free_tb: 4.2 },
        { name: 'aggr_ssd_east02', free_tb: 3.1 },
        { name: 'aggr_ssd_west01', free_tb: 5.8 },
      ],
    },
    {
      type: 'proposal' as const,
      title: 'Recommended CLI Command',
      width: 'full' as const,
      command: 'volume create -vserver svm_prod -volume vol_app_new -aggregate aggr_ssd_east01 -size 2TB -policy nfs-default -space-guarantee none -percent-snapshot-space 5 -qos-adaptive-policy-group perf-tier',
    },
    {
      type: 'callout' as const,
      icon: '⚠️',
      title: 'Pre-Flight Checks',
      width: 'full' as const,
      body: 'aggr_ssd_east01 has 4.2 TB free. After provisioning this 2 TB volume, 2.2 TB will remain (52% utilized). This is within recommended thresholds.',
    },
    {
      type: 'action-form' as const,
      width: 'full' as const,
      fields: [
        { key: 'volume_name', label: 'Volume Name', type: 'text' as const, placeholder: 'e.g. my_vol', required: true, defaultValue: 'vol_app_new' },
        { key: 'qos_policy', label: 'Performance Policy', type: 'select' as const, placeholder: 'None', options: ['perf-tier', 'value-tier', 'extreme'] },
        { key: 'export_policy', label: 'Export Policy', type: 'select' as const, placeholder: 'None', options: ['default', 'nfs-open', 'nfs-restricted'] },
        { key: 'snapshot_policy', label: 'Snapshot Policy', type: 'select' as const, placeholder: 'None', options: ['default', 'none', 'daily-7'] },
        { key: 'enable_monitoring', label: 'Enable Monitoring', type: 'checkbox' as const, defaultValue: 'false' },
      ],
      submit: {
        label: 'Provision on cluster-east',
        tool: 'create_volume',
        params: { svm: 'svm_prod', aggregate: 'aggr_ssd_east01', size: '2TB' },
      },
      secondary: { label: 'Show other options', action: 'message' as const, message: 'Show me provisioning options on other clusters.' },
      recheck: {
        label: 'Re-check Placement',
        fields: ['qos_policy'],
        message: 'Re-check provisioning for a {size} volume named {volume_name} with QoS policy {qos_policy} on my fastest storage',
      },
    },
  ],
};

/**
 * Console-style morning coffee (v2) — A/B comparison variant.
 * Uses per-cluster capacity rows, headroom performance, and IOPS in the volumes table.
 */
export const morningCoffeeV2 = {
  title: 'Good Morning — Fleet Overview',
  toggle: { label: 'Show Summary', message: 'show me a summary of my fleet' },
  panels: [
    {
      type: 'alert-summary' as const,
      title: 'Alert Counts',
      width: 'full' as const,
      data: { critical: 1, warning: 4, info: 2 },
    },
    {
      type: 'resource-table' as const,
      title: 'Storage Capacity',
      width: 'half' as const,
      columns: ['Cluster', 'Capacity'],
      rows: [
        { name: 'cluster-east', Cluster: 'cluster-east', Capacity: '67.2% (4.7 / 7.0 TiB)', capacity_trend: [62, 63, 64, 65, 66, 66, 67] },
        { name: 'cluster-west', Cluster: 'cluster-west', Capacity: '59.1% (3.5 / 6.0 TiB)', capacity_trend: [55, 56, 57, 57, 58, 58, 59] },
      ],
    },
    {
      type: 'resource-table' as const,
      title: 'Storage Performance',
      width: 'half' as const,
      columns: ['Cluster', 'Used'],
      rows: [
        { name: 'cluster-east', Cluster: 'cluster-east', Used: '62.2%', used_trend: [58, 61, 59, 63, 60, 64, 62] },
        { name: 'cluster-west', Cluster: 'cluster-west', Used: '39.9%', used_trend: [42, 38, 41, 37, 40, 39, 40] },
      ],
    },
    {
      type: 'resource-table' as const,
      title: 'Top Volumes',
      width: 'full' as const,
      columns: ['Volume', 'Capacity', 'IOPS'],
      rows: [
        { name: 'vol_logs', Volume: 'vol_logs', Capacity: '95.2%', IOPS: '1240', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [90, 91, 92, 93, 94, 95, 95], iops_trend: [1100, 1150, 1180, 1200, 1220, 1230, 1240] },
        { name: 'vol_data02', Volume: 'vol_data02', Capacity: '87.3%', IOPS: '3420', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [83, 84, 85, 86, 86, 87, 87], iops_trend: [3200, 3250, 3300, 3350, 3380, 3400, 3420] },
        { name: 'vol_archive', Volume: 'vol_archive', Capacity: '84.1%', IOPS: '180', cluster: 'cluster-west', svm: 'svm_backup', capacity_trend: [80, 81, 82, 83, 83, 84, 84], iops_trend: [150, 160, 170, 175, 178, 180, 180] },
        { name: 'vol_app_data', Volume: 'vol_app_data', Capacity: '79.8%', IOPS: '5670', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [75, 76, 77, 78, 79, 79, 80], iops_trend: [5200, 5300, 5400, 5500, 5550, 5600, 5670] },
        { name: 'vol_staging', Volume: 'vol_staging', Capacity: '76.4%', IOPS: '920', cluster: 'cluster-west', svm: 'svm_dev', capacity_trend: [73, 74, 74, 75, 75, 76, 76], iops_trend: [850, 870, 880, 890, 900, 910, 920] },
      ],
    },
    {
      type: 'callout' as const,
      icon: '💡',
      title: 'Recommendation',
      width: 'full' as const,
      body: 'vol_logs on cluster-east is at 95% and growing ~1.5%/day. cluster-east CPU headroom is at 42% (optimal: 68%) — comfortable but worth monitoring during peak hours.',
    },
  ],
};

// --- Object-Detail Fixtures (§8.4 of chatbot-object-detail-design.md) ---

/**
 * Alert object-detail matching the Confluence Alert Details page layout.
 * Exercises all 7 section layouts: properties, chart, alert-list, timeline, actions, text, table.
 */
export const alertDetail = {
  type: 'object-detail' as const,
  kind: 'alert',
  name: 'InstanceDown — node-east-01',
  status: 'critical',
  subtitle: 'Firing since 2025-06-14 09:32 UTC (4h 28m)',
  sections: [
    {
      title: 'Alert Details',
      layout: 'properties' as const,
      data: {
        columns: 2,
        items: [
          { label: 'Severity', value: 'critical', color: 'red' },
          { label: 'Alert Name', value: 'InstanceDown' },
          { label: 'Impact', value: 'Data access may be degraded' },
          { label: 'Firing Since', value: '2025-06-14 09:32 UTC' },
          { label: 'Cluster', value: 'cluster-east', link: 'Tell me about cluster-east' },
          { label: 'Node', value: 'node-east-01' },
        ],
      },
    },
    {
      title: 'Metric Trends',
      layout: 'chart' as const,
      data: {
        type: 'area' as const,
        xKey: 'time',
        yLabel: 'Response Time (ms)',
        series: [{ key: 'value', label: 'Avg Response', color: 'blue' }],
        data: [
          { time: 'Jun 14 06:00', value: 12 },
          { time: 'Jun 14 07:00', value: 15 },
          { time: 'Jun 14 08:00', value: 45 },
          { time: 'Jun 14 09:00', value: 120 },
          { time: 'Jun 14 09:30', value: 0 },
          { time: 'Jun 14 10:00', value: 0 },
        ],
        annotations: [
          { y: 100, label: 'Threshold', color: 'red', style: 'dashed' },
        ],
      },
    },
    {
      title: 'Active Alerts',
      layout: 'alert-list' as const,
      data: {
        items: [
          { severity: 'critical' as const, message: 'InstanceDown — node-east-01', time: '4h ago' },
          { severity: 'warning' as const, message: 'HighLatency — vol_prod_db01', time: '2h ago' },
        ],
      },
    },
    {
      title: 'Timeline',
      layout: 'timeline' as const,
      data: {
        events: [
          { time: '09:32', label: 'Alert fired', severity: 'critical' },
          { time: '09:35', label: 'Notification sent to #ops-alerts', icon: 'notification' },
          { time: '09:40', label: 'Auto-remediation attempted', icon: 'action' },
          { time: '10:15', label: 'Escalation: no acknowledgment after 45m', severity: 'warning' },
        ],
      },
    },
    {
      title: 'Actions',
      layout: 'actions' as const,
      data: {
        buttons: [
          { label: 'Investigate Node', action: 'message' as const, message: 'What\'s happening on node-east-01?' },
          { label: 'Silence Alert (4h)', action: 'execute' as const, tool: 'silence_alert', params: { alertname: 'InstanceDown', duration: '4h' }, variant: 'outline' as const },
          { label: 'Acknowledge', action: 'execute' as const, tool: 'acknowledge_alert', params: { alertname: 'InstanceDown' }, variant: 'outline' as const },
        ],
      },
    },
    {
      title: 'Recommended Actions',
      layout: 'text' as const,
      data: {
        body: '1. Check node connectivity: `system node show`\n2. Review recent config changes on cluster-east\n3. If node is unreachable, initiate failover: `storage failover takeover -ofnode node-east-01`',
      },
    },
    {
      title: 'Notifications Sent',
      layout: 'table' as const,
      data: {
        columns: ['Time', 'Channel', 'Recipients', 'Status'],
        rows: [
          { Time: '09:33', Channel: 'Slack', Recipients: '#ops-alerts', Status: 'Delivered' },
          { Time: '09:33', Channel: 'Email', Recipients: 'oncall@corp.com', Status: 'Delivered' },
        ],
      },
    },
  ],
};

/**
 * Volume object-detail with properties, capacity chart, performance chart, alert-list, actions.
 */
export const volumeDetail = {
  type: 'object-detail' as const,
  kind: 'volume',
  name: 'vol_prod_db01',
  status: 'warning',
  subtitle: 'SVM: svm_prod • Cluster: cluster-east • Aggregate: aggr1_east',
  qualifier: 'on SVM svm_prod on cluster cluster-east',
  sections: [
    {
      title: 'Volume Properties',
      layout: 'properties' as const,
      data: {
        columns: 2,
        items: [
          { label: 'State', value: 'Online' },
          { label: 'Size', value: '2 TB' },
          { label: 'Used', value: '78%', color: 'yellow' },
          { label: 'Available', value: '440 GB' },
          { label: 'Protocol', value: 'NFS' },
          { label: 'Snapshot Policy', value: 'default' },
          { label: 'SVM', value: 'svm_prod', link: 'Tell me about SVM svm_prod', qualifier: 'on cluster cluster-east' },
          { label: 'Aggregate', value: 'aggr1_east', link: 'Tell me about aggregate aggr1_east', qualifier: 'on cluster cluster-east' },
        ],
      },
    },
    {
      title: 'Capacity Trend — Last 30 Days',
      layout: 'chart' as const,
      data: {
        type: 'area' as const,
        xKey: 'day',
        yLabel: 'Used %',
        series: [{ key: 'used', label: 'Used %', color: 'violet' }],
        data: [
          { day: 'Jun 1', used: 68 },
          { day: 'Jun 8', used: 71 },
          { day: 'Jun 15', used: 74 },
          { day: 'Jun 22', used: 76 },
          { day: 'Jun 29', used: 78 },
        ],
        annotations: [
          { y: 80, label: 'Warning', color: 'yellow', style: 'dashed' },
          { y: 95, label: 'Critical', color: 'red', style: 'dashed' },
        ],
      },
    },
    {
      title: 'IOPS — Last 24 Hours',
      layout: 'chart' as const,
      data: {
        type: 'area' as const,
        xKey: 'hour',
        series: [
          { key: 'read', label: 'Read', color: 'blue' },
          { key: 'write', label: 'Write', color: 'violet' },
        ],
        data: [
          { hour: '00:00', read: 420, write: 180 },
          { hour: '04:00', read: 150, write: 60 },
          { hour: '08:00', read: 800, write: 340 },
          { hour: '12:00', read: 1100, write: 520 },
          { hour: '16:00', read: 950, write: 410 },
          { hour: '20:00', read: 600, write: 250 },
        ],
      },
    },
    {
      title: 'Active Alerts',
      layout: 'alert-list' as const,
      data: {
        items: [
          { severity: 'warning' as const, message: 'vol_prod_db01 at 78% capacity — approaching threshold', time: '2h ago' },
        ],
      },
    },
    {
      title: 'Actions',
      layout: 'actions' as const,
      data: {
        buttons: [
          { label: 'Show Snapshots', action: 'message' as const, message: 'Show snapshots for vol_prod_db01' },
          { label: 'Show Aggregates', action: 'message' as const, message: 'Show aggregates for volume vol_prod_db01' },
          { label: 'Resize Volume', action: 'message' as const, message: 'Resize vol_prod_db01 to 3 TB' },
        ],
      },
    },
  ],
};

/**
 * Cluster object-detail with properties, charts, top-volumes table, alert-list, actions.
 */
export const clusterDetail = {
  type: 'object-detail' as const,
  kind: 'cluster',
  name: 'cluster-east',
  status: 'ok',
  subtitle: 'ONTAP 9.15.1 • 4 nodes • 12 aggregates',
  sections: [
    {
      title: 'Cluster Properties',
      layout: 'properties' as const,
      data: {
        columns: 2,
        items: [
          { label: 'ONTAP Version', value: '9.15.1' },
          { label: 'Nodes', value: '4' },
          { label: 'Aggregates', value: '12' },
          { label: 'Volumes', value: '247' },
          { label: 'Total Capacity', value: '48 TB' },
          { label: 'Used', value: '67%' },
          { label: 'Management LIF', value: '10.0.1.100' },
          { label: 'HA Pairs', value: '2' },
        ],
      },
    },
    {
      title: 'Aggregate Usage',
      layout: 'chart' as const,
      data: {
        type: 'bar' as const,
        xKey: 'name',
        series: [{ key: 'used', label: 'Used %', color: 'teal' }],
        data: [
          { name: 'aggr1_east', used: 72 },
          { name: 'aggr2_east', used: 65 },
          { name: 'aggr_ssd_east01', used: 58 },
          { name: 'aggr_ssd_east02', used: 81 },
        ],
      },
    },
    {
      title: 'Cluster IOPS — Last 7 Days',
      layout: 'chart' as const,
      data: {
        type: 'area' as const,
        xKey: 'day',
        yLabel: 'IOPS',
        series: [{ key: 'total', label: 'Total IOPS', color: 'blue' }],
        data: [
          { day: 'Mon', total: 12400 },
          { day: 'Tue', total: 13800 },
          { day: 'Wed', total: 14200 },
          { day: 'Thu', total: 13600 },
          { day: 'Fri', total: 15100 },
          { day: 'Sat', total: 8200 },
          { day: 'Sun', total: 7500 },
        ],
      },
    },
    {
      title: 'Top Volumes by Usage',
      layout: 'table' as const,
      data: {
        columns: ['Name', 'SVM', 'Used', 'Size', 'IOPS'],
        rows: [
          { Name: 'vol_logs', SVM: 'svm_prod', Used: '95%', Size: '500 GB', IOPS: '320' },
          { Name: 'vol_prod_db01', SVM: 'svm_prod', Used: '78%', Size: '2 TB', IOPS: '1100' },
          { Name: 'vol_data02', SVM: 'svm_prod', Used: '87%', Size: '1 TB', IOPS: '450' },
        ],
      },
    },
    {
      title: 'Active Alerts',
      layout: 'alert-list' as const,
      data: {
        items: [
          { severity: 'critical' as const, message: 'vol_logs at 95% capacity', time: '12 min ago' },
          { severity: 'warning' as const, message: 'aggr_ssd_east02 at 81% — approaching threshold', time: '1h ago' },
          { severity: 'warning' as const, message: 'vol_data02 at 87% capacity', time: '2h ago' },
        ],
      },
    },
    {
      title: 'Actions',
      layout: 'actions' as const,
      data: {
        buttons: [
          { label: 'Show All Volumes', action: 'message' as const, message: 'Show all volumes on cluster-east' },
          { label: 'Check Aggregates', action: 'message' as const, message: 'Show aggregate details for cluster-east' },
          { label: 'View Alerts', action: 'message' as const, message: 'Show all alerts for cluster-east' },
        ],
      },
    },
  ],
};

/**
 * Object-list fixture — a paginated list of volumes with sparklines.
 * Demonstrates the object-list interest pattern: resource-table + action-button pagination.
 */
export const volumeList = {
  title: 'Top 10 Volumes by Capacity',
  panels: [
    {
      type: 'resource-table' as const,
      title: 'Volumes',
      width: 'full' as const,
      columns: ['Volume', 'Capacity', 'IOPS'],
      rows: [
        { name: 'vol_logs', Volume: 'vol_logs', Capacity: '95.2%', IOPS: '340', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [90, 91, 92, 93, 94, 94, 95], iops_trend: [310, 320, 330, 325, 340, 335, 340] },
        { name: 'vol_data02', Volume: 'vol_data02', Capacity: '87.3%', IOPS: '1250', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [82, 83, 84, 85, 86, 86, 87], iops_trend: [1100, 1150, 1200, 1180, 1220, 1240, 1250] },
        { name: 'vol_archive', Volume: 'vol_archive', Capacity: '82.1%', IOPS: '45', cluster: 'cluster-west', svm: 'svm_backup', capacity_trend: [78, 79, 79, 80, 81, 81, 82], iops_trend: [40, 42, 44, 43, 45, 44, 45] },
        { name: 'vol_prod_db01', Volume: 'vol_prod_db01', Capacity: '78.4%', IOPS: '3200', cluster: 'cluster-east', svm: 'svm_prod', capacity_trend: [74, 75, 76, 76, 77, 78, 78], iops_trend: [2900, 3000, 3050, 3100, 3150, 3180, 3200] },
        { name: 'vol_home', Volume: 'vol_home', Capacity: '71.6%', IOPS: '120', cluster: 'cluster-west', svm: 'svm_nas', capacity_trend: [68, 69, 69, 70, 70, 71, 72], iops_trend: [100, 105, 110, 115, 118, 120, 120] },
      ],
    },
    {
      type: 'action-button' as const,
      width: 'full' as const,
      buttons: [
        { label: 'Show next 10', action: 'message' as const, message: 'Show me volumes ranked 11-20 by capacity' },
      ],
    },
  ],
};
