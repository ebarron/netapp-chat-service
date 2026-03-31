/**
 * TypeScript interfaces for all chart and dashboard panel types.
 * Spec ref: §5.2 (chart schemas), §4.7 (interest-specific types), §4.4 (dashboard block)
 */

// --- Basic Chart Types (§5.2) ---

export interface SeriesDefinition {
  key: string;
  label: string;
  color?: string;
}

export interface AreaChartData {
  type: 'area';
  title: string;
  width?: PanelWidth;
  xKey: string;
  yLabel?: string;
  series: SeriesDefinition[];
  data: Record<string, unknown>[];
  annotations?: ChartAnnotation[];
}

export interface BarChartData {
  type: 'bar';
  title: string;
  width?: PanelWidth;
  xKey: string;
  series: SeriesDefinition[];
  data: Record<string, unknown>[];
}

export interface GaugeData {
  type: 'gauge';
  title: string;
  width?: PanelWidth;
  value: number;
  max: number;
  unit?: string;
  thresholds?: { warning: number; critical: number };
}

export interface SparklineData {
  type: 'sparkline';
  title?: string;
  width?: PanelWidth;
  data: number[];
  color?: string;
}

export interface StatusGridData {
  type: 'status-grid';
  title: string;
  width?: PanelWidth;
  items: Array<{
    name: string;
    status: 'ok' | 'warning' | 'critical' | 'unknown';
    detail?: string;
  }>;
}

export interface StatData {
  type: 'stat';
  title: string;
  width?: PanelWidth;
  value: string;
  subtitle?: string;
  trend?: 'up' | 'down' | 'flat';
  trendValue?: string;
}

// --- Interest-Specific Types (§4.7) ---

export interface AlertSummaryData {
  type: 'alert-summary';
  title?: string;
  width?: PanelWidth;
  data: {
    critical?: number;
    warning?: number;
    info?: number;
    ok?: number;
  };
}

export interface ResourceTableData {
  type: 'resource-table';
  title: string;
  width?: PanelWidth;
  columns: string[];
  rows: Array<{
    name: string;
    [key: string]: unknown;
  }>;
}

export interface AlertListData {
  type: 'alert-list';
  title?: string;
  width?: PanelWidth;
  items: Array<{
    severity: 'critical' | 'warning' | 'info';
    message: string;
    time: string;
  }>;
}

export interface CalloutData {
  type: 'callout';
  width?: PanelWidth;
  icon?: string;
  title: string;
  body: string;
}

export interface ProposalData {
  type: 'proposal';
  title: string;
  width?: PanelWidth;
  command: string;
  format?: string;
}

export interface ActionButtonItem {
  label: string;
  action: 'execute' | 'message';
  tool?: string;
  params?: Record<string, unknown>;
  message?: string;
  icon?: string;
  variant?: 'primary' | 'outline';
  /** Per-button qualifier override. When set, this replaces the card-level qualifier
   *  for this button's action message. Use "" to suppress qualifier entirely. */
  qualifier?: string;
  /** When true, the button is disabled in read-only mode (e.g. monitoring toggle). */
  requiresReadWrite?: boolean;
}

export interface ActionButtonData {
  type: 'action-button';
  width?: PanelWidth;
  buttons: ActionButtonItem[];
}

export interface ActionFormField {
  key: string;
  label: string;
  type: 'text' | 'select' | 'checkbox';
  placeholder?: string;
  required?: boolean;
  defaultValue?: string;
  options?: string[];
}

export interface ActionFormData {
  type: 'action-form';
  width?: PanelWidth;
  fields: ActionFormField[];
  submit: {
    label: string;
    tool: string;
    params?: Record<string, unknown>;
  };
  secondary?: {
    label: string;
    action: 'message';
    message: string;
  };
  recheck?: {
    label: string;
    fields: string[];
    message: string;
  };
}

// --- Object Detail Types (§3.2–3.3 of chatbot-object-detail-design.md) ---

export interface PropertyItem {
  label: string;
  value: string;
  color?: string;
  link?: string; // injects follow-up chat prompt on click
  /** Per-link qualifier override. When set, this replaces the card-level qualifier
   *  for this link's action message. Use "" to suppress qualifier entirely (e.g. clusters). */
  qualifier?: string;
}

export interface PropertiesData {
  columns?: number;
  items: PropertyItem[];
}

export interface TimelineEvent {
  time: string;
  label: string;
  severity?: string;
  icon?: string;
}

export interface TimelineData {
  events: TimelineEvent[];
}

export interface ChartAnnotation {
  y: number;
  label: string;
  color?: string;
  style?: 'solid' | 'dashed';
}

export interface ObjectDetailSection {
  title: string;
  layout: 'properties' | 'chart' | 'alert-list' | 'timeline' | 'actions' | 'text' | 'table';
  data: unknown; // validated per-layout at render time
}

export interface ObjectDetailData {
  type: 'object-detail';
  kind: string;
  name: string;
  status?: string;
  subtitle?: string;
  /** Identity qualifier appended to action messages for unique identification.
   *  E.g. "on SVM vdbench on cluster cls1" for volumes, "alert-id abc" for alerts. */
  qualifier?: string;
  sections: ObjectDetailSection[];
}

// --- Layout (§4.5) ---

export type PanelWidth = 'full' | 'half' | 'third';

// --- Panel Union ---

export type PanelData =
  | AreaChartData
  | BarChartData
  | GaugeData
  | SparklineData
  | StatusGridData
  | StatData
  | AlertSummaryData
  | ResourceTableData
  | AlertListData
  | CalloutData
  | ProposalData
  | ActionButtonData
  | ActionFormData;

// --- Dashboard Block (§4.4) ---

export interface DashboardToggle {
  label: string;
  message: string;
}

export interface DashboardData {
  title: string;
  panels: PanelData[];
  toggle?: DashboardToggle;
}

// --- Standalone Chart Block (§5.1) ---
// A standalone chart block uses any of the basic chart types directly.
export type ChartData =
  | AreaChartData
  | BarChartData
  | GaugeData
  | SparklineData
  | StatusGridData
  | StatData;

// --- Panel types recognized by the system ---
const KNOWN_PANEL_TYPES = new Set([
  'area',
  'bar',
  'gauge',
  'sparkline',
  'status-grid',
  'stat',
  'alert-summary',
  'resource-table',
  'alert-list',
  'callout',
  'proposal',
  'action-button',
  'action-form',
]);

// --- Data point limit (§5.2 safety net) ---

const MAX_DATA_POINTS = 200;

/**
 * Downsample an array to at most MAX_DATA_POINTS by picking every Nth element.
 * Always includes the first and last element for continuity.
 */
function downsampleArray<T>(arr: T[]): T[] {
  if (arr.length <= MAX_DATA_POINTS) return arr;
  const step = (arr.length - 1) / (MAX_DATA_POINTS - 1);
  const result: T[] = [];
  for (let i = 0; i < MAX_DATA_POINTS - 1; i++) {
    result.push(arr[Math.round(i * step)]);
  }
  result.push(arr[arr.length - 1]);
  return result;
}

/**
 * Apply data-point limits to a panel if it contains a large data array.
 * Mutates the panel in place and returns it for chaining.
 */
export function downsamplePanel(panel: PanelData): PanelData {
  switch (panel.type) {
    case 'area':
    case 'bar':
      if (Array.isArray(panel.data) && panel.data.length > MAX_DATA_POINTS) {
        panel.data = downsampleArray(panel.data);
      }
      break;
    case 'sparkline':
      if (Array.isArray(panel.data) && panel.data.length > MAX_DATA_POINTS) {
        panel.data = downsampleArray(panel.data);
      }
      break;
    default:
      break;
  }
  return panel;
}

/** Strip JS-style comments and trailing commas — common LLM JSON errors. */
function sanitizeJson(text: string): string {
  const noComments = text.replace(/^(\s*)\/\/.*$/gm, '$1');
  return noComments.replace(/,\s*([\]}])/g, '$1');
}

/**
 * Parse a dashboard JSON string into a typed DashboardData.
 * Returns null on failure (malformed JSON, missing fields, etc.).
 * Unknown panel types are silently skipped (§OQ #5).
 */
export function parseDashboard(json: string): DashboardData | null {
  try {
    const parsed = JSON.parse(sanitizeJson(json));
    if (typeof parsed !== 'object' || parsed === null) return null;
    if (typeof parsed.title !== 'string') return null;
    if (!Array.isArray(parsed.panels)) return null;

    const panels: PanelData[] = [];
    for (const panel of parsed.panels) {
      if (typeof panel !== 'object' || panel === null) continue;
      // Accept panels with known type, or infer the type from shape.
      if (!panel.type || !KNOWN_PANEL_TYPES.has(panel.type)) {
        const inferred = inferChartType(panel as Record<string, unknown>);
        if (inferred) {
          panel.type = inferred;
        } else {
          continue;
        }
      }
      panels.push(downsamplePanel(panel as PanelData));
    }

    const result: DashboardData = { title: parsed.title, panels };
    if (
      parsed.toggle &&
      typeof parsed.toggle === 'object' &&
      typeof parsed.toggle.label === 'string' &&
      typeof parsed.toggle.message === 'string'
    ) {
      result.toggle = { label: parsed.toggle.label, message: parsed.toggle.message };
    }
    return result;
  } catch {
    return null;
  }
}

/**
 * Infer a chart type from the shape of a parsed JSON object that is missing
 * the `type` discriminator. Returns the inferred type string, or null if the
 * shape doesn't match any known chart type.
 *
 * Rules are ordered from most-specific to least-specific to avoid false
 * positives (e.g. a gauge also has `value`, but requires `max`).
 */
export function inferChartType(obj: Record<string, unknown>): string | null {
  // object-detail: kind + name + sections array (check before charts)
  if (typeof obj.kind === 'string' && typeof obj.name === 'string' && Array.isArray(obj.sections)) {
    return 'object-detail';
  }
  // area/bar: xKey + series + data array of objects
  if (typeof obj.xKey === 'string' && Array.isArray(obj.series) && Array.isArray(obj.data)) {
    // bar if any series has no yLabel hint, but both are valid — default to bar
    return typeof obj.yLabel === 'string' ? 'area' : 'bar';
  }
  // gauge: numeric value + max
  if (typeof obj.value === 'number' && typeof obj.max === 'number') return 'gauge';
  // sparkline: data is array of numbers (no xKey)
  if (Array.isArray(obj.data) && !obj.xKey && obj.data.length > 0 && typeof obj.data[0] === 'number') return 'sparkline';
  // resource-table: columns + rows
  if (Array.isArray(obj.columns) && Array.isArray(obj.rows)) return 'resource-table';
  // proposal: command field
  if (typeof obj.command === 'string' && typeof obj.title === 'string') return 'proposal';
  // action-form: fields array + submit object
  if (Array.isArray(obj.fields) && typeof obj.submit === 'object' && obj.submit !== null) return 'action-form';
  // action-button: buttons array
  if (Array.isArray(obj.buttons) && obj.buttons.length > 0) return 'action-button';
  // callout: title + body (string fields)
  if (typeof obj.title === 'string' && typeof obj.body === 'string') return 'callout';
  // alert-summary: data object with severity counts
  if (typeof obj.data === 'object' && obj.data !== null && !Array.isArray(obj.data)) {
    const d = obj.data as Record<string, unknown>;
    if (typeof d.critical === 'number' || typeof d.warning === 'number') return 'alert-summary';
  }
  // items-based types: status-grid vs alert-list
  if (Array.isArray(obj.items) && obj.items.length > 0) {
    const first = obj.items[0] as Record<string, unknown>;
    // Alert-list: severity + any message-like field (LLMs often use Prometheus field names)
    if (typeof first?.severity === 'string') {
      const msg = first.message ?? first.alertname ?? first.description ?? first.summary ?? first.name;
      if (typeof msg === 'string') return 'alert-list';
    }
    if (typeof first?.name === 'string' && typeof first?.status === 'string') return 'status-grid';
  }
  // stat: string value + title (most generic — check last)
  if (typeof obj.value === 'string' && typeof obj.title === 'string') return 'stat';
  return null;
}

/**
 * Normalize alert-list items to canonical field names.
 * LLMs sometimes return Prometheus-style fields (alertname, description,
 * summary, startsAt) instead of the schema-specified message/time.
 */
function normalizeAlertItems(obj: Record<string, unknown>): void {
  if (!Array.isArray(obj.items)) return;
  obj.items = (obj.items as Record<string, unknown>[]).map((item) => ({
    ...item,
    message: item.message ?? item.alertname ?? item.description ?? item.summary ?? item.name ?? 'Unknown alert',
    time: item.time ?? item.startsAt ?? item.timestamp ?? '',
  }));
}

/**
 * Parse a standalone chart JSON string into a typed ChartData.
 * If the JSON object lacks a `type` field, attempts to infer it from shape.
 * Returns null on failure.
 */
export function parseChart(json: string): PanelData | null {
  try {
    const parsed = JSON.parse(sanitizeJson(json));
    if (typeof parsed !== 'object' || parsed === null) return null;
    if (!KNOWN_PANEL_TYPES.has(parsed.type)) {
      const inferred = inferChartType(parsed);
      if (!inferred || inferred === 'object-detail') return null;
      parsed.type = inferred;
    }
    if (parsed.type === 'alert-list') normalizeAlertItems(parsed);
    return downsamplePanel(parsed as PanelData);
  } catch {
    return null;
  }
}

const VALID_SECTION_LAYOUTS = new Set([
  'properties', 'chart', 'alert-list', 'timeline', 'actions', 'text', 'table',
]);

/**
 * Parse an object-detail JSON string into a typed ObjectDetailData.
 * Returns null on failure (malformed JSON, missing required fields).
 * Unknown section layouts are kept (forward-compatible).
 */
export function parseObjectDetail(json: string): ObjectDetailData | null {
  try {
    const parsed = JSON.parse(sanitizeJson(json));
    if (typeof parsed !== 'object' || parsed === null) return null;
    if (typeof parsed.name !== 'string') return null;
    if (!Array.isArray(parsed.sections)) return null;

    const sections: ObjectDetailSection[] = [];
    for (const sec of parsed.sections) {
      if (typeof sec !== 'object' || sec === null) continue;
      if (typeof sec.title !== 'string') continue;
      if (typeof sec.layout !== 'string') continue;
      // Normalize alert-list items inside object-detail sections.
      if (sec.layout === 'alert-list' && typeof sec.data === 'object' && sec.data !== null) {
        normalizeAlertItems(sec.data as Record<string, unknown>);
      }
      sections.push(sec as ObjectDetailSection);
    }

    return {
      type: 'object-detail',
      kind: typeof parsed.kind === 'string' ? parsed.kind : 'unknown',
      name: parsed.name,
      status: typeof parsed.status === 'string' ? parsed.status : undefined,
      subtitle: typeof parsed.subtitle === 'string' ? parsed.subtitle : undefined,
      qualifier: typeof parsed.qualifier === 'string' ? parsed.qualifier : undefined,
      sections,
    };
  } catch {
    return null;
  }
}
