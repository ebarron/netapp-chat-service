// Package capability defines the per-MCP capability state model used by the
// chatbot to control tool access. Each capability maps 1:1 to an MCP server.
//
// Design ref: docs/chatbot-design-spec.md section 7.1, 7.2, 7.4
package capability

import (
	"encoding/json"
	"fmt"
)

// State represents the autonomy level for a capability.
type State string

const (
	// StateOff disables the capability entirely.
	StateOff State = "off"
	// StateAsk requires user approval for every tool invocation.
	StateAsk State = "ask"
	// StateAllow permits autonomous tool execution.
	StateAllow State = "allow"
)

// Valid returns true if s is a recognized state.
func (s State) Valid() bool {
	switch s {
	case StateOff, StateAsk, StateAllow:
		return true
	}
	return false
}

// String implements fmt.Stringer.
func (s State) String() string { return string(s) }

// Capability describes a single MCP capability and its current state.
type Capability struct {
	ID                 string `json:"id" yaml:"id"`
	Name               string `json:"name" yaml:"name"`
	Description        string `json:"description" yaml:"description"`
	State              State  `json:"state" yaml:"state"`
	Available          bool   `json:"available" yaml:"-"`
	ToolsCount         int    `json:"tools_count" yaml:"-"`
	ReadOnlyToolsCount int    `json:"read_only_tools_count" yaml:"-"`
	ServerName         string `json:"-" yaml:"server_name"`
}

// DefaultCapabilities returns an empty capability set.
// Products provide their capabilities via config.yaml mcp_servers definitions.
func DefaultCapabilities() []Capability {
	return nil
}

// CapabilityMap is a convenience alias for capability states keyed by ID.
type CapabilityMap map[string]State

// MarshalJSON implements custom JSON marshaling.
func (cm CapabilityMap) MarshalJSON() ([]byte, error) {
	m := make(map[string]string, len(cm))
	for k, v := range cm {
		m[k] = string(v)
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements custom JSON unmarshaling with validation.
func (cm *CapabilityMap) UnmarshalJSON(data []byte) error {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*cm = make(CapabilityMap, len(m))
	for k, v := range m {
		s := State(v)
		if !s.Valid() {
			return fmt.Errorf("invalid capability state %q for %q", v, k)
		}
		(*cm)[k] = s
	}
	return nil
}

// Merge applies saved states onto a list of capabilities, preserving defaults
// for any capabilities not present in the map.
func Merge(caps []Capability, saved CapabilityMap) []Capability {
	result := make([]Capability, len(caps))
	copy(result, caps)
	for i := range result {
		if s, ok := saved[result[i].ID]; ok && s.Valid() {
			result[i].State = s
		}
	}
	return result
}

// ToMap converts a capability slice into a CapabilityMap of just the states.
func ToMap(caps []Capability) CapabilityMap {
	m := make(CapabilityMap, len(caps))
	for _, c := range caps {
		m[c.ID] = c.State
	}
	return m
}
