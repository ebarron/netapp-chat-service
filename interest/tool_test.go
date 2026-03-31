package interest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolDef(t *testing.T) {
	def := ToolDef()
	if def.Name != "get_interest" {
		t.Errorf("ToolDef Name = %q, want %q", def.Name, "get_interest")
	}
	if def.Description == "" {
		t.Error("ToolDef Description is empty")
	}
	if len(def.Schema) == 0 {
		t.Error("ToolDef Schema is empty")
	}

	var schema map[string]any
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatalf("ToolDef Schema is not valid JSON: %v", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	if _, ok := props["id"]; !ok {
		t.Error("schema missing 'id' property")
	}
}

func TestNewHandler_ValidID(t *testing.T) {
	cat := catalogWithTestInterest(t)
	handler := NewHandler(cat)

	input, _ := json.Marshal(map[string]string{"id": "test-interest"})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Test body content") {
		t.Errorf("result = %q, want to contain %q", result, "Test body content")
	}
}

func TestNewHandler_UnknownID(t *testing.T) {
	cat := catalogWithTestInterest(t)
	handler := NewHandler(cat)

	input, _ := json.Marshal(map[string]string{"id": "nonexistent"})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestNewHandler_EmptyID(t *testing.T) {
	cat := catalogWithTestInterest(t)
	handler := NewHandler(cat)

	input, _ := json.Marshal(map[string]string{"id": ""})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestNewHandler_InvalidJSON(t *testing.T) {
	cat := catalogWithTestInterest(t)
	handler := NewHandler(cat)

	_, err := handler(context.Background(), json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "invalid input")
	}
}

// catalogWithTestInterest creates a Catalog with one interest for testing.
func catalogWithTestInterest(t *testing.T) *Catalog {
	t.Helper()
	cat := NewCatalog(nil)
	cat.interests["test-interest"] = &Interest{
		Meta: InterestMeta{
			ID:       "test-interest",
			Name:     "Test Interest",
			Source:   "builtin",
			Requires: []string{"harvest"},
		},
		Body: "Test body content\n\nWith multiple paragraphs.",
	}
	return cat
}

// --- SaveToolDef tests ---

func TestSaveToolDef(t *testing.T) {
	def := SaveToolDef()
	if def.Name != "save_interest" {
		t.Errorf("Name = %q, want %q", def.Name, "save_interest")
	}
	if def.Description == "" {
		t.Error("Description is empty")
	}

	var schema map[string]any
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatalf("Schema is not valid JSON: %v", err)
	}
	props := schema["properties"].(map[string]any)
	for _, field := range []string{"id", "name", "triggers", "requires", "body"} {
		if _, ok := props[field]; !ok {
			t.Errorf("schema missing %q property", field)
		}
	}
	required := schema["required"].([]any)
	if len(required) != 4 {
		t.Errorf("required fields = %d, want 4", len(required))
	}
}

func TestDeleteToolDef(t *testing.T) {
	def := DeleteToolDef()
	if def.Name != "delete_interest" {
		t.Errorf("Name = %q, want %q", def.Name, "delete_interest")
	}
	if def.Description == "" {
		t.Error("Description is empty")
	}

	var schema map[string]any
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatalf("Schema is not valid JSON: %v", err)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["id"]; !ok {
		t.Error("schema missing 'id' property")
	}
}

// --- save handler tests ---

func TestNewSaveHandler_Valid(t *testing.T) {
	dir := t.TempDir()
	cat := NewCatalog(nil)
	validCaps := map[string]bool{"harvest": true, "ontap": true}
	handler := NewSaveHandler(cat, dir, validCaps)

	input, _ := json.Marshal(map[string]any{
		"id":       "backup-status",
		"name":     "Backup Health",
		"triggers": []string{"backup status"},
		"requires": []string{"harvest"},
		"body":     "Show backup info.",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "saved successfully") {
		t.Errorf("result = %q, want to contain %q", result, "saved successfully")
	}

	// Verify file written to disk.
	if _, err := os.Stat(filepath.Join(dir, "backup-status.md")); err != nil {
		t.Errorf("file not found on disk: %v", err)
	}

	// Verify in-memory catalog updated.
	if got := cat.Get("backup-status"); got == nil {
		t.Error("interest not in catalog after save")
	}
}

func TestNewSaveHandler_Update(t *testing.T) {
	dir := t.TempDir()
	cat := NewCatalog(nil)
	cat.interests["my-interest"] = &Interest{
		Meta: InterestMeta{
			ID: "my-interest", Name: "Old Name", Source: "user",
			Requires: []string{"harvest"},
		},
		Body: "Old body.",
	}
	validCaps := map[string]bool{"harvest": true}
	handler := NewSaveHandler(cat, dir, validCaps)

	input, _ := json.Marshal(map[string]any{
		"id":       "my-interest",
		"name":     "New Name",
		"requires": []string{"harvest"},
		"body":     "New body.",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "saved") {
		t.Errorf("result = %q", result)
	}
	got := cat.Get("my-interest")
	if got == nil || got.Meta.Name != "New Name" {
		t.Errorf("interest not updated: %+v", got)
	}
}

func TestNewSaveHandler_RejectsBuiltinShadow(t *testing.T) {
	cat := catalogWithTestInterest(t) // has builtin "test-interest"
	dir := t.TempDir()
	validCaps := map[string]bool{"harvest": true}
	handler := NewSaveHandler(cat, dir, validCaps)

	input, _ := json.Marshal(map[string]any{
		"id":       "test-interest",
		"name":     "Override",
		"requires": []string{"harvest"},
		"body":     "Override body.",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for shadowing built-in")
	}
	if !strings.Contains(err.Error(), "built-in") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "built-in")
	}
}

func TestNewSaveHandler_RejectsInvalidID(t *testing.T) {
	cat := NewCatalog(nil)
	dir := t.TempDir()
	validCaps := map[string]bool{"harvest": true}
	handler := NewSaveHandler(cat, dir, validCaps)

	tests := []struct {
		name string
		id   string
	}{
		{"starts with digit", "1bad"},
		{"uppercase", "Bad-Name"},
		{"spaces", "has spaces"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{
				"id":       tt.id,
				"name":     "Test",
				"requires": []string{"harvest"},
				"body":     "Body.",
			})
			_, err := handler(context.Background(), input)
			if err == nil {
				t.Errorf("expected error for id %q", tt.id)
			}
		})
	}
}

func TestNewSaveHandler_RejectsInvalidCapability(t *testing.T) {
	cat := NewCatalog(nil)
	dir := t.TempDir()
	validCaps := map[string]bool{"harvest": true, "ontap": true}
	handler := NewSaveHandler(cat, dir, validCaps)

	input, _ := json.Marshal(map[string]any{
		"id":       "test-cap",
		"name":     "Test",
		"requires": []string{"harvest", "bogus"},
		"body":     "Body.",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for invalid capability")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "bogus")
	}
}

func TestNewSaveHandler_RejectsMissingFields(t *testing.T) {
	cat := NewCatalog(nil)
	dir := t.TempDir()
	handler := NewSaveHandler(cat, dir, map[string]bool{"harvest": true})

	input, _ := json.Marshal(map[string]any{
		"id":   "test",
		"name": "Test",
		// missing requires and body
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestNewSaveHandler_RejectsCapExceeded(t *testing.T) {
	cat := NewCatalog(nil)
	dir := t.TempDir()
	validCaps := map[string]bool{"harvest": true}

	// Fill up to the cap.
	for i := 0; i < MaxUserInterests; i++ {
		id := fmt.Sprintf("user-%d", i)
		cat.interests[id] = &Interest{
			Meta: InterestMeta{ID: id, Name: id, Source: "user", Requires: []string{"harvest"}},
			Body: "body",
		}
	}

	handler := NewSaveHandler(cat, dir, validCaps)
	input, _ := json.Marshal(map[string]any{
		"id":       "one-too-many",
		"name":     "Overflow",
		"requires": []string{"harvest"},
		"body":     "Body.",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for cap exceeded")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "limit")
	}
}

func TestNewSaveHandler_InvalidJSON(t *testing.T) {
	cat := NewCatalog(nil)
	handler := NewSaveHandler(cat, t.TempDir(), map[string]bool{})

	_, err := handler(context.Background(), json.RawMessage(`{bad`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "invalid input")
	}
}

// --- delete handler tests ---

func TestNewDeleteHandler_Valid(t *testing.T) {
	dir := t.TempDir()
	cat := NewCatalog(nil)

	// Pre-save a user interest.
	i := &Interest{
		Meta: InterestMeta{ID: "to-delete", Name: "Delete Me", Source: "user", Requires: []string{"harvest"}},
		Body: "Body.",
	}
	if err := SaveUserInterest(dir, i); err != nil {
		t.Fatal(err)
	}
	cat.interests["to-delete"] = i

	handler := NewDeleteHandler(cat, dir)
	input, _ := json.Marshal(map[string]string{"id": "to-delete"})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "deleted successfully") {
		t.Errorf("result = %q", result)
	}

	// Verify removed from memory and disk.
	if cat.Get("to-delete") != nil {
		t.Error("interest still in catalog")
	}
	if _, err := os.Stat(filepath.Join(dir, "to-delete.md")); !os.IsNotExist(err) {
		t.Error("file still on disk")
	}
}

func TestNewDeleteHandler_RejectsBuiltin(t *testing.T) {
	cat := catalogWithTestInterest(t) // builtin "test-interest"
	handler := NewDeleteHandler(cat, t.TempDir())

	input, _ := json.Marshal(map[string]string{"id": "test-interest"})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for deleting built-in")
	}
	if !strings.Contains(err.Error(), "built-in") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "built-in")
	}
}

func TestNewDeleteHandler_NotFound(t *testing.T) {
	cat := NewCatalog(nil)
	handler := NewDeleteHandler(cat, t.TempDir())

	input, _ := json.Marshal(map[string]string{"id": "nonexistent"})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for nonexistent interest")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestNewDeleteHandler_EmptyID(t *testing.T) {
	cat := NewCatalog(nil)
	handler := NewDeleteHandler(cat, t.TempDir())

	input, _ := json.Marshal(map[string]string{"id": ""})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestNewDeleteHandler_InvalidJSON(t *testing.T) {
	cat := NewCatalog(nil)
	handler := NewDeleteHandler(cat, t.TempDir())

	_, err := handler(context.Background(), json.RawMessage(`{bad`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "invalid input")
	}
}

// --- ID validation tests ---

func TestNewSaveHandler_RejectsInvalidOutputTarget(t *testing.T) {
	cat := NewCatalog(nil)
	dir := t.TempDir()
	handler := NewSaveHandler(cat, dir, map[string]bool{"harvest": true})

	input, _ := json.Marshal(map[string]any{
		"id":            "test-target",
		"name":          "Test",
		"requires":      []string{"harvest"},
		"body":          "Body.",
		"output_target": "bogus",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for invalid output_target")
	}
	if !strings.Contains(err.Error(), "output_target") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "output_target")
	}
}

func TestValidIDPattern(t *testing.T) {
	valid := []string{"a", "my-interest", "backup-status-2", "a1b2c3"}
	for _, id := range valid {
		if !validIDPattern.MatchString(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}
	invalid := []string{"", "1abc", "ABC", "has space", "-start", "under_score"}
	for _, id := range invalid {
		if validIDPattern.MatchString(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}
