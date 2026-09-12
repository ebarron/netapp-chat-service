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
  /** LLMs may use strings or descriptor objects such as { key, label }. */
  columns: unknown[];
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
  /** Optional on the wire when `tool` or `message` makes the action unambiguous. */
  action?: 'execute' | 'message';
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
    /** Whether this submit performs a read-write action. Defaults to `true`
     *  (form submits run a tool, e.g. provisioning), so the button is disabled
     *  in read-only mode. Set to `false` for read-only-safe submits — e.g. a
     *  picker that only re-renders a dashboard — so the button stays enabled
     *  in read-only mode. */
    requiresReadWrite?: boolean;
  };
  secondary?: {
    label: string;
    action: 'message';
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

type UnknownRecord = Record<string, unknown>;
type ValidationMode = 'strict' | 'defensive';

export interface PayloadValidationIssue {
  code: string;
  path: string;
  severity: 'error' | 'warning';
}

export interface NormalizedPayload<T> {
  value: T | null;
  issues: PayloadValidationIssue[];
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function addIssue(
  issues: PayloadValidationIssue[],
  mode: ValidationMode,
  code: string,
  path: string,
): void {
  issues.push({ code, path, severity: mode === 'strict' ? 'error' : 'warning' });
}

function requiredArray(
  value: UnknownRecord,
  key: string,
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): unknown[] {
  if (Array.isArray(value[key])) return value[key];
  addIssue(issues, mode, 'missing_or_invalid_collection', `${path}.${key}`);
  return [];
}

function optionalArray(
  value: UnknownRecord,
  key: string,
  path: string,
  issues: PayloadValidationIssue[],
): unknown[] {
  if (value[key] === undefined || value[key] === null) return [];
  if (Array.isArray(value[key])) return value[key];
  issues.push({
    code: 'invalid_optional_collection',
    path: `${path}.${key}`,
    severity: 'warning',
  });
  return [];
}

function requiredString(
  value: UnknownRecord,
  key: string,
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): string | null {
  if (typeof value[key] === 'string') return value[key];
  addIssue(issues, mode, 'missing_or_invalid_string', `${path}.${key}`);
  return null;
}

function normalizeWidth(width: unknown): PanelWidth | undefined {
  return width === 'full' || width === 'half' || width === 'third' ? width : undefined;
}

function normalizeRecordArray(
  values: unknown[],
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): UnknownRecord[] {
  const records: UnknownRecord[] = [];
  values.forEach((value, index) => {
    if (isRecord(value)) {
      records.push(value);
    } else {
      addIssue(issues, mode, 'invalid_collection_item', `${path}[${index}]`);
    }
  });
  return records;
}

function normalizeSeries(
  values: unknown[],
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): SeriesDefinition[] {
  const result: SeriesDefinition[] = [];
  normalizeRecordArray(values, path, mode, issues).forEach((series, index) => {
    if (typeof series.key !== 'string' || typeof series.label !== 'string') {
      addIssue(issues, mode, 'invalid_series', `${path}[${index}]`);
      return;
    }
    result.push({
      key: series.key,
      label: series.label,
      color: typeof series.color === 'string' ? series.color : undefined,
    });
  });
  return result;
}

function normalizeButtons(
  values: unknown[],
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): ActionButtonItem[] {
  const result: ActionButtonItem[] = [];
  normalizeRecordArray(values, path, mode, issues).forEach((button, index) => {
    if (typeof button.label !== 'string') {
      addIssue(issues, mode, 'invalid_action_button', `${path}[${index}].label`);
      return;
    }
    const explicitAction = button.action === 'execute' || button.action === 'message'
      ? button.action
      : undefined;
    const inferredAction = explicitAction
      ?? (typeof button.tool === 'string' ? 'execute' : undefined)
      ?? (typeof button.message === 'string' ? 'message' : undefined);
    const hasTarget = inferredAction === 'execute'
      ? typeof button.tool === 'string' && button.tool.length > 0
      : inferredAction === 'message'
        ? typeof button.message === 'string' && button.message.length > 0
        : false;
    if (!hasTarget) {
      addIssue(issues, mode, 'invalid_action_target', `${path}[${index}]`);
      return;
    }
    result.push({
      label: button.label,
      action: inferredAction,
      tool: typeof button.tool === 'string' ? button.tool : undefined,
      params: isRecord(button.params) ? button.params : undefined,
      message: typeof button.message === 'string' ? button.message : undefined,
      icon: typeof button.icon === 'string' ? button.icon : undefined,
      variant: button.variant === 'primary' || button.variant === 'outline'
        ? button.variant
        : undefined,
      qualifier: typeof button.qualifier === 'string' ? button.qualifier : undefined,
      requiresReadWrite: typeof button.requiresReadWrite === 'boolean'
        ? button.requiresReadWrite
        : undefined,
    });
  });
  return result;
}

function normalizePanelValue(
  value: unknown,
  path: string,
  mode: ValidationMode,
  issues: PayloadValidationIssue[],
): PanelData | null {
  if (!isRecord(value)) {
    addIssue(issues, mode, 'invalid_panel', path);
    return null;
  }

  let type = typeof value.type === 'string' && KNOWN_PANEL_TYPES.has(value.type)
    ? value.type
    : inferChartType(value);
  if (!type || type === 'object-detail') {
    // Unknown panel types remain forward-compatible: callers skip them.
    issues.push({ code: 'unknown_panel_type', path: `${path}.type`, severity: 'warning' });
    return null;
  }

  const width = normalizeWidth(value.width);
  switch (type) {
    case 'area':
    case 'bar': {
      const title = requiredString(value, 'title', path, mode, issues);
      const xKey = requiredString(value, 'xKey', path, mode, issues);
      const series = normalizeSeries(
        requiredArray(value, 'series', path, mode, issues),
        `${path}.series`,
        mode,
        issues,
      );
      const data = normalizeRecordArray(
        requiredArray(value, 'data', path, mode, issues),
        `${path}.data`,
        mode,
        issues,
      );
      if (title === null || xKey === null) return null;
      const base = { ...value, type, title, width, xKey, series, data };
      if (type === 'area') {
        const annotations = normalizeRecordArray(
          optionalArray(value, 'annotations', path, issues),
          `${path}.annotations`,
          'defensive',
          issues,
        ).flatMap((annotation) => (
          typeof annotation.y === 'number' && typeof annotation.label === 'string'
            ? [{
                y: annotation.y,
                label: annotation.label,
                color: typeof annotation.color === 'string' ? annotation.color : undefined,
                style: annotation.style === 'solid' || annotation.style === 'dashed'
                  ? annotation.style
                  : undefined,
              }]
            : []
        ));
        return downsamplePanel({
          ...base,
          yLabel: typeof value.yLabel === 'string' ? value.yLabel : undefined,
          annotations,
        } as AreaChartData);
      }
      return downsamplePanel(base as BarChartData);
    }
    case 'gauge': {
      const title = requiredString(value, 'title', path, mode, issues);
      if (title === null || typeof value.value !== 'number' || typeof value.max !== 'number') {
        if (typeof value.value !== 'number') addIssue(issues, mode, 'invalid_number', `${path}.value`);
        if (typeof value.max !== 'number') addIssue(issues, mode, 'invalid_number', `${path}.max`);
        return null;
      }
      const thresholds = isRecord(value.thresholds)
        && typeof value.thresholds.warning === 'number'
        && typeof value.thresholds.critical === 'number'
        ? { warning: value.thresholds.warning, critical: value.thresholds.critical }
        : undefined;
      return {
        ...value,
        type,
        title,
        width,
        value: value.value,
        max: value.max,
        unit: typeof value.unit === 'string' ? value.unit : undefined,
        thresholds,
      } as GaugeData;
    }
    case 'sparkline': {
      const data = requiredArray(value, 'data', path, mode, issues).filter(
        (point): point is number => typeof point === 'number',
      );
      return downsamplePanel({
        ...value,
        type,
        width,
        title: typeof value.title === 'string' ? value.title : undefined,
        color: typeof value.color === 'string' ? value.color : undefined,
        data,
      } as SparklineData);
    }
    case 'status-grid': {
      const title = requiredString(value, 'title', path, mode, issues);
      const items = normalizeRecordArray(
        requiredArray(value, 'items', path, mode, issues),
        `${path}.items`,
        mode,
        issues,
      ).flatMap((item, index) => {
        if (typeof item.name !== 'string') {
          addIssue(issues, mode, 'invalid_status_item', `${path}.items[${index}]`);
          return [];
        }
        const status = item.status === 'ok'
          || item.status === 'warning'
          || item.status === 'critical'
          || item.status === 'unknown'
          ? item.status
          : 'unknown';
        return [{
          name: item.name,
          status,
          detail: typeof item.detail === 'string' ? item.detail : undefined,
        }];
      });
      if (title === null) return null;
      return { ...value, type, title, width, items } as StatusGridData;
    }
    case 'stat': {
      const title = requiredString(value, 'title', path, mode, issues);
      const statValue = typeof value.value === 'string'
        ? value.value
        : mode === 'defensive' && value.value !== undefined && value.value !== null
          ? String(value.value)
          : requiredString(value, 'value', path, mode, issues);
      if (title === null || statValue === null) return null;
      return {
        ...value,
        type,
        title,
        width,
        value: statValue,
        subtitle: typeof value.subtitle === 'string' ? value.subtitle : undefined,
        trend: value.trend === 'up' || value.trend === 'down' || value.trend === 'flat'
          ? value.trend
          : undefined,
        trendValue: typeof value.trendValue === 'string' ? value.trendValue : undefined,
      } as StatData;
    }
    case 'alert-summary': {
      if (!isRecord(value.data)) {
        addIssue(issues, mode, 'missing_or_invalid_object', `${path}.data`);
        if (mode === 'strict') return null;
      }
      const counts: AlertSummaryData['data'] = {};
      for (const severity of ['critical', 'warning', 'info', 'ok'] as const) {
        if (isRecord(value.data) && typeof value.data[severity] === 'number') {
          counts[severity] = value.data[severity];
        }
      }
      return {
        ...value,
        type,
        width,
        title: typeof value.title === 'string' ? value.title : undefined,
        data: counts,
      } as AlertSummaryData;
    }
    case 'resource-table': {
      const title = requiredString(value, 'title', path, mode, issues);
      const columns = requiredArray(value, 'columns', path, mode, issues);
      const rows = normalizeRecordArray(
        requiredArray(value, 'rows', path, mode, issues),
        `${path}.rows`,
        mode,
        issues,
      ) as ResourceTableData['rows'];
      if (title === null) return null;
      return { ...value, type, title, width, columns, rows } as ResourceTableData;
    }
    case 'alert-list': {
      const items = normalizeRecordArray(
        requiredArray(value, 'items', path, mode, issues),
        `${path}.items`,
        mode,
        issues,
      ).flatMap((item, index) => {
        const severity = item.severity === 'critical'
          || item.severity === 'warning'
          || item.severity === 'info'
          ? item.severity
          : null;
        const message = item.message ?? item.alertname ?? item.description ?? item.summary ?? item.name;
        if (!severity || typeof message !== 'string') {
          addIssue(issues, mode, 'invalid_alert_item', `${path}.items[${index}]`);
          return [];
        }
        const time = item.time ?? item.startsAt ?? item.timestamp ?? '';
        return [{ severity, message, time: typeof time === 'string' ? time : '' }];
      });
      return {
        ...value,
        type,
        width,
        title: typeof value.title === 'string' ? value.title : undefined,
        items,
      } as AlertListData;
    }
    case 'callout': {
      const title = requiredString(value, 'title', path, mode, issues);
      const body = requiredString(value, 'body', path, mode, issues);
      if (title === null || body === null) return null;
      return {
        ...value,
        type,
        title,
        width,
        body,
        icon: typeof value.icon === 'string' ? value.icon : undefined,
      } as CalloutData;
    }
    case 'proposal': {
      const title = requiredString(value, 'title', path, mode, issues);
      const command = requiredString(value, 'command', path, mode, issues);
      if (title === null || command === null) return null;
      return {
        ...value,
        type,
        title,
        width,
        command,
        format: typeof value.format === 'string' ? value.format : undefined,
      } as ProposalData;
    }
    case 'action-button': {
      // `buttons` remains required by the producer contract. Defensive parsing
      // supplies [] so malformed external data can never crash the renderer;
      // strict canvas ingestion rejects the same payload before state changes.
      const buttons = normalizeButtons(
        requiredArray(value, 'buttons', path, mode, issues),
        `${path}.buttons`,
        mode,
        issues,
      );
      return { ...value, type, width, buttons } as ActionButtonData;
    }
    case 'action-form': {
      const fields = normalizeRecordArray(
        requiredArray(value, 'fields', path, mode, issues),
        `${path}.fields`,
        mode,
        issues,
      ).flatMap((field, index) => {
        if (
          typeof field.key !== 'string'
          || typeof field.label !== 'string'
          || (field.type !== 'text' && field.type !== 'select' && field.type !== 'checkbox')
        ) {
          addIssue(issues, mode, 'invalid_form_field', `${path}.fields[${index}]`);
          return [];
        }
        return [{
          key: field.key,
          label: field.label,
          type: field.type,
          placeholder: typeof field.placeholder === 'string' ? field.placeholder : undefined,
          required: typeof field.required === 'boolean' ? field.required : undefined,
          defaultValue: typeof field.defaultValue === 'string' ? field.defaultValue : undefined,
          options: optionalArray(field, 'options', `${path}.fields[${index}]`, issues)
            .filter((option): option is string => typeof option === 'string'),
        }];
      });
      if (
        !isRecord(value.submit)
        || typeof value.submit.label !== 'string'
        || typeof value.submit.tool !== 'string'
      ) {
        addIssue(issues, mode, 'missing_or_invalid_submit', `${path}.submit`);
        return null;
      }
      const secondary = isRecord(value.secondary)
        && typeof value.secondary.label === 'string'
        && value.secondary.action === 'message'
        && typeof value.secondary.message === 'string'
        ? {
            label: value.secondary.label,
            action: 'message' as const,
            message: value.secondary.message,
          }
        : undefined;
      return {
        ...value,
        type,
        width,
        fields,
        submit: {
          label: value.submit.label,
          tool: value.submit.tool,
          params: isRecord(value.submit.params) ? value.submit.params : undefined,
          requiresReadWrite: typeof value.submit.requiresReadWrite === 'boolean'
            ? value.submit.requiresReadWrite
            : undefined,
        },
        secondary,
      } as ActionFormData;
    }
    default:
      return null;
  }
}

function hasErrors(issues: PayloadValidationIssue[], fromIndex = 0): boolean {
  return issues.slice(fromIndex).some((issue) => issue.severity === 'error');
}

export function normalizeDashboardPayload(
  input: unknown,
  mode: ValidationMode = 'strict',
): NormalizedPayload<DashboardData> {
  const issues: PayloadValidationIssue[] = [];
  if (!isRecord(input)) {
    addIssue(issues, mode, 'invalid_dashboard', '$');
    return { value: null, issues };
  }
  const title = requiredString(input, 'title', '$', mode, issues);
  if (!Array.isArray(input.panels)) {
    addIssue(issues, mode, 'missing_or_invalid_collection', '$.panels');
    return { value: null, issues };
  }
  const rawPanels = requiredArray(input, 'panels', '$', mode, issues);
  const panels: PanelData[] = [];
  rawPanels.forEach((panel, index) => {
    const normalized = normalizePanelValue(panel, `$.panels[${index}]`, mode, issues);
    if (normalized) panels.push(normalized);
  });
  if (title === null || (mode === 'strict' && hasErrors(issues))) {
    return { value: null, issues };
  }
  const result: DashboardData = { title, panels };
  if (
    isRecord(input.toggle)
    && typeof input.toggle.label === 'string'
    && typeof input.toggle.message === 'string'
  ) {
    result.toggle = { label: input.toggle.label, message: input.toggle.message };
  }
  return { value: result, issues };
}

/**
 * Parse a dashboard JSON string into a typed DashboardData.
 * Returns null on failure (malformed JSON, missing fields, etc.).
 * Unknown panel types are silently skipped (§OQ #5).
 */
export function parseDashboard(json: string): DashboardData | null {
  try {
    const parsed = JSON.parse(sanitizeJson(json));
    return normalizeDashboardPayload(parsed, 'defensive').value;
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
    const issues: PayloadValidationIssue[] = [];
    return normalizePanelValue(parsed, '$', 'defensive', issues);
  } catch {
    return null;
  }
}

const VALID_SECTION_LAYOUTS = new Set([
  'properties', 'chart', 'alert-list', 'timeline', 'actions', 'text', 'table',
]);

export function normalizeObjectDetailPayload(
  input: unknown,
  mode: ValidationMode = 'strict',
): NormalizedPayload<ObjectDetailData> {
  const issues: PayloadValidationIssue[] = [];
  if (!isRecord(input)) {
    addIssue(issues, mode, 'invalid_object_detail', '$');
    return { value: null, issues };
  }
  const name = requiredString(input, 'name', '$', mode, issues);
  if (!Array.isArray(input.sections)) {
    addIssue(issues, mode, 'missing_or_invalid_collection', '$.sections');
    return { value: null, issues };
  }
  const rawSections = requiredArray(input, 'sections', '$', mode, issues);
  const sections: ObjectDetailSection[] = [];

  rawSections.forEach((rawSection, index) => {
    const path = `$.sections[${index}]`;
    if (!isRecord(rawSection)) {
      addIssue(issues, mode, 'invalid_section', path);
      return;
    }
    const title = requiredString(rawSection, 'title', path, mode, issues);
    const layout = typeof rawSection.layout === 'string' ? rawSection.layout : null;
    if (!layout) {
      addIssue(issues, mode, 'missing_or_invalid_layout', `${path}.layout`);
      return;
    }
    if (!VALID_SECTION_LAYOUTS.has(layout)) {
      issues.push({ code: 'unknown_section_layout', path: `${path}.layout`, severity: 'warning' });
      if (title !== null) sections.push(rawSection as unknown as ObjectDetailSection);
      return;
    }
    if (title === null || !isRecord(rawSection.data)) {
      if (!isRecord(rawSection.data)) {
        addIssue(issues, mode, 'missing_or_invalid_object', `${path}.data`);
      }
      return;
    }

    const data = rawSection.data;
    let normalizedData: unknown = data;
    switch (layout) {
      case 'properties': {
        const items = normalizeRecordArray(
          requiredArray(data, 'items', `${path}.data`, mode, issues),
          `${path}.data.items`,
          mode,
          issues,
        ).flatMap((item, itemIndex) => {
          if (typeof item.label !== 'string' || typeof item.value !== 'string') {
            addIssue(issues, mode, 'invalid_property_item', `${path}.data.items[${itemIndex}]`);
            return [];
          }
          return [{
            label: item.label,
            value: item.value,
            color: typeof item.color === 'string' ? item.color : undefined,
            link: typeof item.link === 'string' ? item.link : undefined,
            qualifier: typeof item.qualifier === 'string' ? item.qualifier : undefined,
          }];
        });
        normalizedData = {
          ...data,
          columns: typeof data.columns === 'number' ? data.columns : undefined,
          items,
        };
        break;
      }
      case 'chart': {
        normalizedData = normalizePanelValue(
          // The section heading supplies the title when an embedded chart
          // intentionally omits its own, avoiding duplicate visible headings.
          { ...data, title: data.title ?? '' },
          `${path}.data`,
          mode,
          issues,
        );
        break;
      }
      case 'alert-list': {
        normalizedData = normalizePanelValue(
          { ...data, type: 'alert-list' },
          `${path}.data`,
          mode,
          issues,
        );
        break;
      }
      case 'timeline': {
        const events = normalizeRecordArray(
          requiredArray(data, 'events', `${path}.data`, mode, issues),
          `${path}.data.events`,
          mode,
          issues,
        ).flatMap((event, eventIndex) => {
          if (typeof event.time !== 'string' || typeof event.label !== 'string') {
            addIssue(issues, mode, 'invalid_timeline_event', `${path}.data.events[${eventIndex}]`);
            return [];
          }
          return [{
            time: event.time,
            label: event.label,
            severity: typeof event.severity === 'string' ? event.severity : undefined,
            icon: typeof event.icon === 'string' ? event.icon : undefined,
          }];
        });
        normalizedData = { ...data, events };
        break;
      }
      case 'actions': {
        normalizedData = normalizePanelValue(
          { ...data, type: 'action-button' },
          `${path}.data`,
          mode,
          issues,
        );
        break;
      }
      case 'text':
        normalizedData = { ...data, body: typeof data.body === 'string' ? data.body : '' };
        break;
      case 'table': {
        normalizedData = normalizePanelValue(
          { ...data, type: 'resource-table', title: data.title ?? '' },
          `${path}.data`,
          mode,
          issues,
        );
        break;
      }
    }
    if (normalizedData !== null) {
      sections.push({
        title,
        layout: layout as ObjectDetailSection['layout'],
        data: normalizedData,
      });
    }
  });

  if (name === null || (mode === 'strict' && hasErrors(issues))) {
    return { value: null, issues };
  }
  return {
    value: {
      type: 'object-detail',
      kind: typeof input.kind === 'string' ? input.kind : 'unknown',
      name,
      status: typeof input.status === 'string' ? input.status : undefined,
      subtitle: typeof input.subtitle === 'string' ? input.subtitle : undefined,
      qualifier: typeof input.qualifier === 'string' ? input.qualifier : undefined,
      sections,
    },
    issues,
  };
}

/**
 * Strictly validates model-generated canvas content before it enters React
 * state. Close directives are intentionally allowed without render fields.
 */
export function normalizeCanvasContent(
  input: unknown,
): NormalizedPayload<Record<string, unknown>> {
  if (!isRecord(input)) {
    return {
      value: null,
      issues: [{ code: 'invalid_canvas_content', path: '$', severity: 'error' }],
    };
  }
  if (input.close === true) {
    const identity = typeof input.title === 'string' || typeof input.name === 'string';
    return identity
      ? { value: { ...input, close: true }, issues: [] }
      : {
          value: null,
          issues: [{ code: 'missing_close_identity', path: '$', severity: 'error' }],
        };
  }
  if (input.type === 'object-detail' || 'sections' in input) {
    const normalized = normalizeObjectDetailPayload(input, 'strict');
    return {
      value: normalized.value
        ? normalized.value as unknown as Record<string, unknown>
        : null,
      issues: normalized.issues,
    };
  }
  if (input.type === 'dashboard' || 'panels' in input) {
    const normalized = normalizeDashboardPayload(input, 'strict');
    return {
      value: normalized.value
        ? {
            ...(input.type === 'dashboard' ? { type: 'dashboard' } : {}),
            ...normalized.value,
          }
        : null,
      issues: normalized.issues,
    };
  }
  return {
    value: null,
    issues: [{ code: 'unknown_canvas_content', path: '$.type', severity: 'error' }],
  };
}

/**
 * Parse an object-detail JSON string into a typed ObjectDetailData.
 * Returns null on failure (malformed JSON, missing required fields).
 * Unknown section layouts are kept (forward-compatible).
 */
export function parseObjectDetail(json: string): ObjectDetailData | null {
  try {
    const parsed = JSON.parse(sanitizeJson(json));
    return normalizeObjectDetailPayload(parsed, 'defensive').value;
  } catch {
    return null;
  }
}
