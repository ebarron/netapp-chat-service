package render

import "encoding/json"

// ObjectDetail mirrors the TypeScript ObjectDetailData interface.
// It is the top-level structure returned by all render tools.
type ObjectDetail struct {
	Type      string    `json:"type"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Status    string    `json:"status,omitempty"`
	Subtitle  string    `json:"subtitle,omitempty"`
	Qualifier string    `json:"qualifier,omitempty"`
	Sections  []Section `json:"sections"`
}

// MarshalBlock returns the object-detail fenced code block
// that the frontend parses from assistant messages.
func (od *ObjectDetail) MarshalBlock() (string, error) {
	return od.marshalAs("object-detail")
}

// MarshalCanvasBlock returns a canvas-object-detail fenced code block
// that the canvas fence interceptor converts to a canvas_open SSE event.
func (od *ObjectDetail) MarshalCanvasBlock() (string, error) {
	return od.marshalAs("canvas-object-detail")
}

func (od *ObjectDetail) marshalAs(fenceType string) (string, error) {
	data, err := json.Marshal(od)
	if err != nil {
		return "", err
	}
	return "```" + fenceType + "\n" + string(data) + "\n```", nil
}

// Section is a single UI section within an ObjectDetail card.
type Section struct {
	Title  string `json:"title"`
	Layout string `json:"layout"` // properties, chart, alert-list, timeline, actions, text, table
	Data   any    `json:"data"`
}

// PropertiesData is the data payload for layout "properties".
type PropertiesData struct {
	Columns int            `json:"columns,omitempty"`
	Items   []PropertyItem `json:"items"`
}

// PropertyItem is a single key-value property row.
type PropertyItem struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	Color     string `json:"color,omitempty"`
	Link      string `json:"link,omitempty"`
	Qualifier string `json:"qualifier,omitempty"`
}

// ActionsData is the data payload for layout "actions".
type ActionsData struct {
	Type    string         `json:"type"` // "action-button"
	Buttons []ActionButton `json:"buttons"`
}

// ActionButton mirrors the TypeScript ActionButtonItem interface.
type ActionButton struct {
	Label             string `json:"label"`
	Action            string `json:"action"` // "execute" or "message"
	Tool              string `json:"tool,omitempty"`
	Message           string `json:"message,omitempty"`
	Icon              string `json:"icon,omitempty"`
	Variant           string `json:"variant,omitempty"`
	Qualifier         string `json:"qualifier,omitempty"`
	RequiresReadWrite bool   `json:"requiresReadWrite,omitempty"`
}

// TextData is the data payload for layout "text".
type TextData struct {
	Body string `json:"body"`
}

// AlertListData is the data payload for layout "alert-list".
type AlertListData struct {
	Type  string      `json:"type"` // "alert-list"
	Items []AlertItem `json:"items"`
}

// AlertItem is a single alert entry.
type AlertItem struct {
	Severity string `json:"severity"` // critical, warning, info
	Message  string `json:"message"`
	Time     string `json:"time"`
}

// AreaChartData is the data payload for layout "chart" with type "area".
type AreaChartData struct {
	Type        string           `json:"type"` // "area"
	Title       string           `json:"title"`
	XKey        string           `json:"xKey"`
	YLabel      string           `json:"yLabel,omitempty"`
	Series      []SeriesDef      `json:"series"`
	Data        []map[string]any `json:"data"`
	Annotations []Annotation     `json:"annotations,omitempty"`
}

// SeriesDef defines a single data series in a chart.
type SeriesDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// Annotation is a horizontal reference line on a chart.
type Annotation struct {
	Y     float64 `json:"y"`
	Label string  `json:"label"`
	Color string  `json:"color,omitempty"`
	Style string  `json:"style,omitempty"` // solid, dashed
}

// StatData is the data payload for a single stat panel.
type StatData struct {
	Type  string `json:"type"` // "stat"
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
	Color string `json:"color,omitempty"`
}
