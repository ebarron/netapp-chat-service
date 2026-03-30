package render

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebarron/netapp-chat-service/internal/llm"
)

// MetricsFetcher queries VictoriaMetrics for time-series data.
type MetricsFetcher interface {
	QueryRange(query string, start, end time.Time, step string) ([]map[string]any, error)
}

// VolumeInput is the input for the render_volume_detail tool.
// The LLM gathers all data and passes it here; Go builds the UI.
type VolumeInput struct {
	// Identity (required)
	Volume  string `json:"volume"`
	SVM     string `json:"svm"`
	Cluster string `json:"cluster"`

	// Status badge (optional, defaults to "ok")
	Status string `json:"status"`

	// Output target: "canvas" to produce a canvas-object-detail fence.
	OutputTarget string `json:"output_target"`

	// Properties (all optional — tool includes what's available)
	State          string `json:"state"`
	SizeTotal      string `json:"size_total"`
	UsedPercent    string `json:"used_percent"`
	Aggregate      string `json:"aggregate"`
	SnapshotPolicy string `json:"snapshot_policy"`
	QoSPolicy      string `json:"qos_policy"`
	Style          string `json:"style"`
	Protocol       string `json:"protocol"`

	// Monitoring status from get_volume_monitoring_status
	MonitoringEnabled bool   `json:"monitoring_enabled"`
	MonitoringRules   int    `json:"monitoring_rules"`
	MonitoringSummary string `json:"monitoring_summary"` // e.g. "3 capacity, 3 data-protection"

	// Chart data — LLM passes raw metrics results
	PerformanceData []map[string]any `json:"performance_data"` // [{time, iops_read, iops_write, latency}]
	CapacityData    []map[string]any `json:"capacity_data"`    // [{time, used_percent}]

	// Alerts — LLM passes filtered alert list
	Alerts []AlertItem `json:"alerts"`

	// Free-text analysis from the LLM
	Analysis string `json:"analysis"`
}

// VolumeToolDef returns the LLM tool definition for render_volume_detail.
func VolumeToolDef() llm.ToolDef {
	schema, _ := json.Marshal(volumeSchema)
	return llm.ToolDef{
		Name:        "render_volume_detail",
		Description: "Render a complete volume detail view. Call this after gathering all volume data (properties, metrics, monitoring status, alerts) and writing your analysis. Returns a formatted object-detail block ready for display.",
		Schema:      schema,
	}
}

// NewVolumeHandler returns a handler for the render_volume_detail tool.
// If fetcher is non-nil, chart data is fetched server-side from VictoriaMetrics
// rather than relying on the LLM to pass it.
func NewVolumeHandler(fetcher MetricsFetcher) func(ctx context.Context, input json.RawMessage) (string, error) {
	return func(_ context.Context, input json.RawMessage) (string, error) {
		var req VolumeInput
		if err := json.Unmarshal(input, &req); err != nil {
			return "", fmt.Errorf("render_volume_detail: invalid input: %w", err)
		}
		if req.Volume == "" || req.SVM == "" || req.Cluster == "" {
			return "", fmt.Errorf("render_volume_detail: volume, svm, and cluster are required")
		}

		// Fetch chart data server-side if a fetcher is available and
		// the LLM didn't already provide the data.
		if fetcher != nil {
			fetchChartData(fetcher, &req)
		}

		od := buildVolumeDetail(req)
		if req.OutputTarget == "canvas" {
			return od.MarshalCanvasBlock()
		}
		return od.MarshalBlock()
	}
}

// fetchChartData queries VictoriaMetrics for performance and capacity
// time-series data, populating the request fields if they are empty.
func fetchChartData(f MetricsFetcher, req *VolumeInput) {
	now := time.Now()
	vol := req.Volume
	svm := req.SVM

	// Performance data (24h) — read/write IOPS and latency
	if len(req.PerformanceData) == 0 {
		start := now.Add(-24 * time.Hour)
		step := "5m"

		readOps, _ := f.QueryRange(fmt.Sprintf(`volume_read_ops{volume="%s", svm="%s"}`, vol, svm), start, now, step)
		writeOps, _ := f.QueryRange(fmt.Sprintf(`volume_write_ops{volume="%s", svm="%s"}`, vol, svm), start, now, step)
		latency, _ := f.QueryRange(fmt.Sprintf(`volume_avg_latency{volume="%s", svm="%s"}`, vol, svm), start, now, step)

		// Merge into performance_data array keyed by time.
		if len(readOps) > 0 || len(writeOps) > 0 || len(latency) > 0 {
			req.PerformanceData = mergeTimeSeries(readOps, writeOps, latency)
		}
	}

	// Capacity data (30d)
	if len(req.CapacityData) == 0 {
		start := now.Add(-30 * 24 * time.Hour)
		capData, err := f.QueryRange(fmt.Sprintf(`volume_size_used_percent{volume="%s", svm="%s"}`, vol, svm), start, now, "1d")
		if err != nil {
			slog.Warn("render: capacity query failed", "error", err)
		} else {
			// Rename "value" to "used_percent" for the chart.
			for _, p := range capData {
				if v, ok := p["value"]; ok {
					p["used_percent"] = v
					delete(p, "value")
				}
			}
			req.CapacityData = capData
		}
	}
}

// mergeTimeSeries combines read IOPS, write IOPS, and latency series into
// a single array with {time, iops_read, iops_write, latency} entries.
func mergeTimeSeries(readOps, writeOps, latency []map[string]any) []map[string]any {
	// Build index by time from the longest series.
	type point struct {
		Read  float64
		Write float64
		Lat   float64
	}
	index := map[string]*point{}
	var order []string

	addSeries := func(data []map[string]any, setter func(*point, float64)) {
		for _, d := range data {
			t, _ := d["time"].(string)
			if t == "" {
				continue
			}
			p, exists := index[t]
			if !exists {
				p = &point{}
				index[t] = p
				order = append(order, t)
			}
			if v, ok := d["value"].(float64); ok {
				setter(p, v)
			}
		}
	}

	addSeries(readOps, func(p *point, v float64) { p.Read = v })
	addSeries(writeOps, func(p *point, v float64) { p.Write = v })
	// Convert latency from microseconds to milliseconds.
	addSeries(latency, func(p *point, v float64) { p.Lat = v / 1000.0 })

	result := make([]map[string]any, 0, len(order))
	for _, t := range order {
		p := index[t]
		result = append(result, map[string]any{
			"time":       t,
			"iops_read":  p.Read,
			"iops_write": p.Write,
			"latency":    p.Lat,
		})
	}
	return result
}

func buildVolumeDetail(req VolumeInput) *ObjectDetail {
	status := req.Status
	if status == "" {
		status = "ok"
	}

	qualifier := fmt.Sprintf("on SVM %s on cluster %s", req.SVM, req.Cluster)

	od := &ObjectDetail{
		Type:      "object-detail",
		Kind:      "volume",
		Name:      req.Volume,
		Status:    status,
		Subtitle:  fmt.Sprintf("Volume on SVM %s, cluster %s", req.SVM, req.Cluster),
		Qualifier: qualifier,
		Sections:  make([]Section, 0, 7),
	}

	// 1. Properties — always first, always present
	od.Sections = append(od.Sections, buildVolumeProperties(req))

	// 2. Performance chart (24h)
	od.Sections = append(od.Sections, buildPerformanceChart(req))

	// 3. Capacity chart (30d)
	od.Sections = append(od.Sections, buildCapacityChart(req))

	// 4. Alerts — always present (empty state handled)
	od.Sections = append(od.Sections, buildAlertList(req))

	// 5. Analysis text — always present
	od.Sections = append(od.Sections, buildAnalysis(req))

	// 6. Actions — ALWAYS present, monitoring button GUARANTEED
	od.Sections = append(od.Sections, buildVolumeActions(req))

	return od
}

func buildVolumeProperties(req VolumeInput) Section {
	items := []PropertyItem{
		{Label: "State", Value: valueOr(req.State, "—")},
		{Label: "Total Size", Value: valueOr(req.SizeTotal, "—")},
		{Label: "Used", Value: valueOr(req.UsedPercent, "—")},
	}

	if req.Aggregate != "" {
		items = append(items, PropertyItem{
			Label:     "Aggregate",
			Value:     req.Aggregate,
			Link:      fmt.Sprintf("Tell me about aggregate %s", req.Aggregate),
			Qualifier: fmt.Sprintf("on cluster %s", req.Cluster),
		})
	}

	items = append(items, PropertyItem{
		Label:     "SVM",
		Value:     req.SVM,
		Link:      fmt.Sprintf("Tell me about SVM %s", req.SVM),
		Qualifier: fmt.Sprintf("on cluster %s", req.Cluster),
	})

	items = append(items, PropertyItem{
		Label:     "Cluster",
		Value:     req.Cluster,
		Link:      fmt.Sprintf("Tell me about cluster %s", req.Cluster),
		Qualifier: "",
	})

	if req.Style != "" {
		items = append(items, PropertyItem{Label: "Style", Value: req.Style})
	}
	if req.Protocol != "" {
		items = append(items, PropertyItem{Label: "Protocol", Value: req.Protocol})
	}
	if req.SnapshotPolicy != "" {
		items = append(items, PropertyItem{Label: "Snapshot Policy", Value: req.SnapshotPolicy})
	}
	if req.QoSPolicy != "" {
		items = append(items, PropertyItem{Label: "QoS Policy", Value: req.QoSPolicy})
	}

	// Monitoring status — always shown
	monValue := "Not monitored"
	monColor := ""
	if req.MonitoringEnabled {
		monValue = fmt.Sprintf("Active (%d rules)", req.MonitoringRules)
		if req.MonitoringSummary != "" {
			monValue = fmt.Sprintf("Active (%s)", req.MonitoringSummary)
		}
		monColor = "green"
	}
	items = append(items, PropertyItem{Label: "Monitoring", Value: monValue, Color: monColor})

	return Section{
		Title:  "Properties",
		Layout: "properties",
		Data:   PropertiesData{Items: items},
	}
}

func buildPerformanceChart(req VolumeInput) Section {
	if len(req.PerformanceData) == 0 {
		return Section{
			Title:  "Performance (last 24h)",
			Layout: "text",
			Data:   TextData{Body: "No I/O activity detected in the last 24 hours."},
		}
	}

	return Section{
		Title:  "Performance (last 24h)",
		Layout: "chart",
		Data: AreaChartData{
			Type:  "area",
			Title: "IOPS & Latency",
			XKey:  "time",
			Series: []SeriesDef{
				{Key: "iops_read", Label: "Read IOPS", Color: "#228be6"},
				{Key: "iops_write", Label: "Write IOPS", Color: "#40c057"},
				{Key: "latency", Label: "Latency (ms)", Color: "#fab005"},
			},
			Data: req.PerformanceData,
		},
	}
}

func buildCapacityChart(req VolumeInput) Section {
	if len(req.CapacityData) == 0 {
		return Section{
			Title:  "Capacity Trend (30 days)",
			Layout: "text",
			Data:   TextData{Body: "No capacity trend data available."},
		}
	}

	return Section{
		Title:  "Capacity Trend (30 days)",
		Layout: "chart",
		Data: AreaChartData{
			Type:   "area",
			Title:  "Used Capacity %",
			XKey:   "time",
			YLabel: "%",
			Series: []SeriesDef{
				{Key: "used_percent", Label: "Used %", Color: "#228be6"},
			},
			Data: req.CapacityData,
			Annotations: []Annotation{
				{Y: 85, Label: "Warning (85%)", Color: "#fab005", Style: "dashed"},
				{Y: 95, Label: "Critical (95%)", Color: "#fa5252", Style: "dashed"},
			},
		},
	}
}

func buildAlertList(req VolumeInput) Section {
	items := req.Alerts
	if items == nil {
		items = []AlertItem{}
	}
	return Section{
		Title:  "Active Alerts",
		Layout: "alert-list",
		Data: AlertListData{
			Type:  "alert-list",
			Items: items,
		},
	}
}

func buildAnalysis(req VolumeInput) Section {
	body := req.Analysis
	if body == "" {
		body = "No analysis available."
	}
	return Section{
		Title:  "Health Analysis",
		Layout: "text",
		Data:   TextData{Body: body},
	}
}

func buildVolumeActions(req VolumeInput) Section {
	var buttons []ActionButton

	// Monitoring toggle — ALWAYS present, ALWAYS requiresReadWrite
	if req.MonitoringEnabled {
		buttons = append(buttons, ActionButton{
			Label:             "Stop Monitoring",
			Action:            "message",
			Message:           fmt.Sprintf("Disable monitoring for volume %s", req.Volume),
			RequiresReadWrite: true,
		})
	} else {
		buttons = append(buttons, ActionButton{
			Label:             "Monitor this Volume",
			Action:            "message",
			Message:           fmt.Sprintf("Enable monitoring for volume %s", req.Volume),
			RequiresReadWrite: true,
		})
	}

	// Standard follow-up actions
	buttons = append(buttons,
		ActionButton{
			Label:   "Show Snapshots",
			Action:  "message",
			Message: fmt.Sprintf("List snapshots on volume %s SVM %s cluster %s", req.Volume, req.SVM, req.Cluster),
		},
		ActionButton{
			Label:   "Resize Volume",
			Action:  "message",
			Message: fmt.Sprintf("Resize volume %s", req.Volume),
			Variant: "outline",
		},
	)

	return Section{
		Title:  "Actions",
		Layout: "actions",
		Data: ActionsData{
			Type:    "action-button",
			Buttons: buttons,
		},
	}
}

func valueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// volumeSchema defines the JSON schema for the render_volume_detail tool input.
var volumeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"volume":              map[string]any{"type": "string", "description": "Volume name."},
		"svm":                 map[string]any{"type": "string", "description": "SVM name."},
		"cluster":             map[string]any{"type": "string", "description": "Cluster name."},
		"status":              map[string]any{"type": "string", "description": "Status badge: ok, warning, critical, info."},
		"state":               map[string]any{"type": "string", "description": "Volume state (e.g. online, offline)."},
		"size_total":          map[string]any{"type": "string", "description": "Total size as human string (e.g. '51.4 GB')."},
		"used_percent":        map[string]any{"type": "string", "description": "Used capacity as string (e.g. '89%')."},
		"aggregate":           map[string]any{"type": "string", "description": "Aggregate name."},
		"snapshot_policy":     map[string]any{"type": "string", "description": "Snapshot policy name."},
		"qos_policy":          map[string]any{"type": "string", "description": "QoS policy name."},
		"style":               map[string]any{"type": "string", "description": "Volume style (FlexVol, FlexGroup)."},
		"protocol":            map[string]any{"type": "string", "description": "Access protocol (NFS, CIFS, iSCSI, etc.)."},
		"monitoring_enabled":  map[string]any{"type": "boolean", "description": "Whether monitoring alert rules are active."},
		"monitoring_rules":    map[string]any{"type": "integer", "description": "Number of active monitoring rules."},
		"monitoring_summary":  map[string]any{"type": "string", "description": "Monitoring categories (e.g. '3 capacity, 3 data-protection')."},
		"alerts":              map[string]any{"type": "array", "description": "Active alerts array [{severity, message, time}]. Pass empty array if none.", "items": map[string]any{"type": "object"}},
		"analysis":            map[string]any{"type": "string", "description": "Your free-text health analysis of this volume. Include observations, risks, and recommendations."},
		"output_target":       map[string]any{"type": "string", "description": "Set to 'canvas' when the interest Target is canvas. This produces a canvas fence so the detail opens in the canvas panel."},
	},
	"required": []string{"volume", "svm", "cluster"},
}
