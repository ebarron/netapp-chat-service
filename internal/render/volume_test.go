package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalBlock(t *testing.T) {
	od := &ObjectDetail{
		Type:     "object-detail",
		Kind:     "volume",
		Name:     "vol1",
		Status:   "ok",
		Sections: []Section{},
	}
	block, err := od.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	if !strings.Contains(block, "object-detail") {
		t.Error("expected object-detail in block")
	}
	// Extract JSON between fences
	lines := strings.Split(block, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	jsonStr := strings.Join(lines[1:len(lines)-1], "\n")
	var parsed ObjectDetail
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("JSON in block is invalid: %v", err)
	}
	if parsed.Kind != "volume" || parsed.Name != "vol1" {
		t.Errorf("round-trip: kind=%q name=%q", parsed.Kind, parsed.Name)
	}
}

func TestBuildVolumeDetail_Always6Sections(t *testing.T) {
	req := VolumeInput{Volume: "vol_test", SVM: "svm1", Cluster: "cls1"}
	od := buildVolumeDetail(req)
	if len(od.Sections) != 6 {
		t.Fatalf("expected 6 sections, got %d", len(od.Sections))
	}
	titles := []string{"Properties", "Performance (last 24h)", "Capacity Trend (30 days)", "Active Alerts", "Health Analysis", "Actions"}
	for i, title := range titles {
		if od.Sections[i].Title != title {
			t.Errorf("section[%d] title = %q, want %q", i, od.Sections[i].Title, title)
		}
	}
}

func TestBuildVolumeDetail_DefaultsStatus(t *testing.T) {
	od := buildVolumeDetail(VolumeInput{Volume: "v1", SVM: "svm1", Cluster: "c1"})
	if od.Status != "ok" {
		t.Errorf("default status = %q, want ok", od.Status)
	}
}

func TestBuildVolumeDetail_CustomStatus(t *testing.T) {
	od := buildVolumeDetail(VolumeInput{Volume: "v1", SVM: "svm1", Cluster: "c1", Status: "critical"})
	if od.Status != "critical" {
		t.Errorf("status = %q, want critical", od.Status)
	}
}

func TestBuildVolumeActions_MonitoringToggle(t *testing.T) {
	// Enabled => Stop Monitoring
	section := buildVolumeActions(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1", MonitoringEnabled: true})
	data := section.Data.(ActionsData)
	if data.Buttons[0].Label != "Stop Monitoring" {
		t.Errorf("enabled: first button = %q", data.Buttons[0].Label)
	}
	if !data.Buttons[0].RequiresReadWrite {
		t.Error("monitoring button should have requiresReadWrite")
	}

	// Disabled => Monitor this Volume
	section = buildVolumeActions(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1", MonitoringEnabled: false})
	data = section.Data.(ActionsData)
	if data.Buttons[0].Label != "Monitor this Volume" {
		t.Errorf("disabled: first button = %q", data.Buttons[0].Label)
	}
	if !data.Buttons[0].RequiresReadWrite {
		t.Error("monitoring button should have requiresReadWrite")
	}
}

func TestBuildVolumeActions_AlwaysHas3Buttons(t *testing.T) {
	section := buildVolumeActions(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1"})
	data := section.Data.(ActionsData)
	if len(data.Buttons) != 3 {
		t.Errorf("expected 3 buttons, got %d", len(data.Buttons))
	}
}

func TestBuildVolumeProperties_MonitoringRow(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		rules   int
		summary string
		wantVal string
	}{
		{"not monitored", false, 0, "", "Not monitored"},
		{"enabled with count", true, 6, "", "Active (6 rules)"},
		{"enabled with summary", true, 6, "3 capacity, 3 data-protection", "Active (3 capacity, 3 data-protection)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := VolumeInput{
				Volume:            "v1",
				SVM:               "svm1",
				Cluster:           "cls1",
				MonitoringEnabled: tt.enabled,
				MonitoringRules:   tt.rules,
				MonitoringSummary: tt.summary,
			}
			section := buildVolumeProperties(req)
			data := section.Data.(PropertiesData)
			var monItem *PropertyItem
			for i := range data.Items {
				if data.Items[i].Label == "Monitoring" {
					monItem = &data.Items[i]
					break
				}
			}
			if monItem == nil {
				t.Fatal("no Monitoring property found")
			}
			if monItem.Value != tt.wantVal {
				t.Errorf("Monitoring value = %q, want %q", monItem.Value, tt.wantVal)
			}
		})
	}
}

func TestBuildPerformanceChart_NoData(t *testing.T) {
	section := buildPerformanceChart(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1"})
	if section.Layout != "text" {
		t.Errorf("empty perf data should fall back to text, got %q", section.Layout)
	}
}

func TestBuildPerformanceChart_WithData(t *testing.T) {
	req := VolumeInput{
		Volume: "v1", SVM: "s1", Cluster: "c1",
		PerformanceData: []map[string]any{{"time": "12:00", "iops_read": 100}},
	}
	section := buildPerformanceChart(req)
	if section.Layout != "chart" {
		t.Errorf("with data should be chart, got %q", section.Layout)
	}
}

func TestBuildCapacityChart_NoData(t *testing.T) {
	section := buildCapacityChart(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1"})
	if section.Layout != "text" {
		t.Errorf("empty capacity data should fall back to text, got %q", section.Layout)
	}
}

func TestBuildCapacityChart_WithData(t *testing.T) {
	req := VolumeInput{
		Volume: "v1", SVM: "s1", Cluster: "c1",
		CapacityData: []map[string]any{{"time": "2024-01-01", "used_percent": 72}},
	}
	section := buildCapacityChart(req)
	if section.Layout != "chart" {
		t.Errorf("with data should be chart, got %q", section.Layout)
	}
	data := section.Data.(AreaChartData)
	if len(data.Annotations) != 2 {
		t.Errorf("expected 2 annotations (warning+critical), got %d", len(data.Annotations))
	}
}

func TestNewVolumeHandler_MissingRequired(t *testing.T) {
	handler := NewVolumeHandler(nil)
	tests := []struct {
		name  string
		input string
	}{
		{"empty", `{}`},
		{"missing svm", `{"volume":"v1","cluster":"c1"}`},
		{"missing volume", `{"svm":"s1","cluster":"c1"}`},
		{"missing cluster", `{"volume":"v1","svm":"s1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(nil, json.RawMessage(tt.input))
			if err == nil {
				t.Error("expected error for missing required fields")
			}
		})
	}
}

func TestNewVolumeHandler_ValidInput(t *testing.T) {
	handler := NewVolumeHandler(nil)
	input := `{"volume":"vol1","svm":"svm1","cluster":"cls1","status":"warning","analysis":"Looks good"}`
	result, err := handler(nil, json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "object-detail") {
		t.Error("result should be a fenced object-detail block")
	}
	lines := strings.Split(result, "\n")
	jsonStr := strings.Join(lines[1:len(lines)-1], "\n")
	var od ObjectDetail
	if err := json.Unmarshal([]byte(jsonStr), &od); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if od.Kind != "volume" {
		t.Errorf("kind = %q", od.Kind)
	}
	if len(od.Sections) != 6 {
		t.Errorf("sections = %d, want 6", len(od.Sections))
	}
}

func TestBuildAlertList_NilAlerts(t *testing.T) {
	section := buildAlertList(VolumeInput{Volume: "v1", SVM: "s1", Cluster: "c1"})
	data := section.Data.(AlertListData)
	if data.Items == nil {
		t.Error("nil alerts should be converted to empty slice")
	}
}

func TestBuildAlertList_WithAlerts(t *testing.T) {
	req := VolumeInput{
		Volume: "v1", SVM: "s1", Cluster: "c1",
		Alerts: []AlertItem{{Severity: "warning", Message: "high usage", Time: "2024-01-01T12:00:00Z"}},
	}
	section := buildAlertList(req)
	data := section.Data.(AlertListData)
	if len(data.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(data.Items))
	}
}

func TestValueOr(t *testing.T) {
	if v := valueOr("hello", "default"); v != "hello" {
		t.Errorf("got %q", v)
	}
	if v := valueOr("", "default"); v != "default" {
		t.Errorf("got %q", v)
	}
}

func TestMarshalCanvasBlock(t *testing.T) {
	od := &ObjectDetail{
		Type:     "object-detail",
		Kind:     "volume",
		Name:     "vol1",
		Status:   "ok",
		Sections: []Section{},
	}
	block, err := od.MarshalCanvasBlock()
	if err != nil {
		t.Fatalf("MarshalCanvasBlock: %v", err)
	}
	if !strings.HasPrefix(block, "```canvas-object-detail\n") {
		t.Errorf("expected canvas-object-detail fence, got prefix: %q", block[:40])
	}
	if !strings.HasSuffix(block, "\n```") {
		t.Error("expected closing fence")
	}
}

func TestNewVolumeHandler_CanvasOutputTarget(t *testing.T) {
	handler := NewVolumeHandler(nil)
	input := `{"volume":"vol1","svm":"svm1","cluster":"cls1","output_target":"canvas"}`
	result, err := handler(nil, json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "```canvas-object-detail\n") {
		t.Errorf("canvas output_target should produce canvas fence, got: %q", result[:40])
	}
}

func TestNewVolumeHandler_DefaultOutputTarget(t *testing.T) {
	handler := NewVolumeHandler(nil)
	input := `{"volume":"vol1","svm":"svm1","cluster":"cls1"}`
	result, err := handler(nil, json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "```object-detail\n") {
		t.Errorf("default should produce object-detail fence, got: %q", result[:30])
	}
}
