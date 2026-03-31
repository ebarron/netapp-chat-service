package capability

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestStateValid(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{StateOff, true},
		{StateAsk, true},
		{StateAllow, true},
		{State("invalid"), false},
		{State(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.Valid(); got != tt.want {
				t.Errorf("State(%q).Valid() = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	if got := StateAsk.String(); got != "ask" {
		t.Errorf("StateAsk.String() = %q, want %q", got, "ask")
	}
}

func TestDefaultCapabilities(t *testing.T) {
	caps := DefaultCapabilities()
	if len(caps) != 0 {
		t.Fatalf("DefaultCapabilities() returned %d capabilities, want 0", len(caps))
	}
}

func TestCapabilityMapMarshalJSON(t *testing.T) {
	cm := CapabilityMap{
		"metrics": StateAsk,
		"storage": StateAllow,
		"dashboards": StateOff,
	}
	data, err := json.Marshal(cm)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"metrics": "ask", "storage": "allow", "dashboards": "off"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCapabilityMapUnmarshalJSON(t *testing.T) {
	data := []byte(`{"harvest":"ask","ontap":"allow"}`)
	var cm CapabilityMap
	if err := json.Unmarshal(data, &cm); err != nil {
		t.Fatal(err)
	}
	if cm["harvest"] != StateAsk {
		t.Errorf("harvest = %q, want %q", cm["harvest"], StateAsk)
	}
	if cm["ontap"] != StateAllow {
		t.Errorf("ontap = %q, want %q", cm["ontap"], StateAllow)
	}
}

func TestCapabilityMapUnmarshalJSONInvalidState(t *testing.T) {
	data := []byte(`{"harvest":"invalid"}`)
	var cm CapabilityMap
	err := json.Unmarshal(data, &cm)
	if err == nil {
		t.Error("expected error for invalid state, got nil")
	}
}

func TestMerge(t *testing.T) {
	caps := []Capability{
		{ID: "metrics", Name: "Metrics", State: StateAsk, ServerName: "metrics-mcp"},
		{ID: "storage", Name: "Storage", State: StateAsk, ServerName: "storage-mcp"},
		{ID: "dashboards", Name: "Dashboards", State: StateAsk, ServerName: "grafana-mcp"},
	}
	saved := CapabilityMap{
		"metrics":    StateAllow,
		"dashboards": StateOff,
	}
	merged := Merge(caps, saved)

	if merged[0].ID != "metrics" || merged[0].State != StateAllow {
		t.Errorf("metrics state = %q, want %q", merged[0].State, StateAllow)
	}
	if merged[1].ID != "storage" || merged[1].State != StateAsk {
		t.Errorf("storage state = %q, want %q (default)", merged[1].State, StateAsk)
	}
	if merged[2].ID != "dashboards" || merged[2].State != StateOff {
		t.Errorf("dashboards state = %q, want %q", merged[2].State, StateOff)
	}
}

func TestMergeNilSaved(t *testing.T) {
	caps := []Capability{
		{ID: "metrics", State: StateAsk},
		{ID: "storage", State: StateAsk},
	}
	merged := Merge(caps, nil)
	for i, c := range merged {
		if c.State != StateAsk {
			t.Errorf("capability[%d] state = %q, want %q", i, c.State, StateAsk)
		}
	}
}

func TestToMap(t *testing.T) {
	caps := []Capability{
		{ID: "harvest", State: StateAllow},
		{ID: "ontap", State: StateOff},
	}
	m := ToMap(caps)
	if m["harvest"] != StateAllow {
		t.Errorf("harvest = %q, want %q", m["harvest"], StateAllow)
	}
	if m["ontap"] != StateOff {
		t.Errorf("ontap = %q, want %q", m["ontap"], StateOff)
	}
}
