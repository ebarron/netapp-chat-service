package interest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testBuiltinDir returns the path to the testdata/interests directory
// containing sample interest files for testing.
func testBuiltinDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("testdata", "interests")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata/interests not found: %v", err)
	}
	return dir
}

func TestCatalog_LoadEmbedded(t *testing.T) {
	c := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := c.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	all := c.All()
	if len(all) != 3 {
		t.Fatalf("loaded %d interests, want 3", len(all))
	}
	ids := make(map[string]bool)
	for _, i := range all {
		ids[i.Meta.ID] = true
	}
	for _, want := range []string{"health-check", "resource-status", "object-detail"} {
		if !ids[want] {
			t.Errorf("missing interest %q", want)
		}
	}
}

func TestCatalog_FilterByCapabilities(t *testing.T) {
	c := NewCatalog(nil)
	// Only enable metrics — resource-status requires [metrics, storage] so it should be excluded.
	enabled := map[string]bool{"metrics": true}
	if err := c.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := c.Get("resource-status"); got != nil {
		t.Error("resource-status should be filtered out when storage is missing")
	}
	if got := c.Get("health-check"); got == nil {
		t.Error("health-check should be present (only requires metrics)")
	}
}

func TestCatalog_NilEnabled_NoFiltering(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, nil); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(c.All()) != 3 {
		t.Errorf("with nil enabled, all 3 interests should load, got %d", len(c.All()))
	}
}

func TestCatalog_UserInterests(t *testing.T) {
	dir := t.TempDir()
	content := "---\nid: backup-status\nname: Backup Health\nsource: user\ntriggers:\n  - backup status\nrequires:\n  - metrics\n---\n\nShow backup info.\n"
	if err := os.WriteFile(filepath.Join(dir, "backup-status.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := c.Load([]string{testBuiltinDir(t), dir}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := c.Get("backup-status"); got == nil {
		t.Error("user interest backup-status not loaded")
	} else if got.Body != "Show backup info." {
		t.Errorf("Body = %q", got.Body)
	}
	if len(c.All()) != 4 {
		t.Errorf("total interests = %d, want 4", len(c.All()))
	}
}

func TestCatalog_UserCannotShadowBuiltin(t *testing.T) {
	dir := t.TempDir()
	content := "---\nid: health-check\nname: My Override\nsource: user\ntriggers:\n  - test\nrequires:\n  - metrics\n---\n\nOverridden.\n"
	if err := os.WriteFile(filepath.Join(dir, "health-check.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := c.Load([]string{testBuiltinDir(t), dir}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := c.Get("health-check")
	if got == nil {
		t.Fatal("health-check should exist")
	}
	if got.Meta.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", got.Meta.Source)
	}
}

func TestCatalog_BuildIndex(t *testing.T) {
	c := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := c.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	index := c.BuildIndex()
	if !strings.Contains(index, "health-check") {
		t.Error("index missing health-check")
	}
	if !strings.Contains(index, "resource-status") {
		t.Error("index missing resource-status")
	}
	if !strings.Contains(index, "| ID |") {
		t.Error("index missing header row")
	}
	triggerChecks := map[string]string{
		"health-check":    "health check",
		"resource-status": "show resources",
	}
	for id, trigger := range triggerChecks {
		if !strings.Contains(index, trigger) {
			t.Errorf("index for %q missing trigger phrase %q", id, trigger)
		}
	}
}

func TestCatalog_BuildIndex_Empty(t *testing.T) {
	c := NewCatalog(nil)
	enabled := map[string]bool{}
	if err := c.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if index := c.BuildIndex(); index != "" {
		t.Errorf("expected empty index, got %q", index)
	}
}

func TestCatalog_Get_NonExistent(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, map[string]bool{"metrics": true}); err != nil {
		t.Fatal(err)
	}
	if got := c.Get("nonexistent"); got != nil {
		t.Error("expected nil for nonexistent interest")
	}
}

func TestCatalog_LoadFS_MalformedSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no frontmatter here"), 0644)
	os.WriteFile(filepath.Join(dir, "good.md"), []byte("---\nid: good\nname: Good\nsource: user\nrequires:\n  - metrics\n---\nbody\n"), 0644)
	c := NewCatalog(nil)
	if err := c.Load([]string{dir}, nil); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := c.Get("good"); got == nil {
		t.Error("good interest should have been loaded despite malformed sibling")
	}
}

func TestCatalog_BuiltinIDs(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, nil); err != nil {
		t.Fatal(err)
	}
	ids := c.BuiltinIDs()
	for _, want := range []string{"health-check", "resource-status", "object-detail"} {
		if !ids[want] {
			t.Errorf("BuiltinIDs missing %q", want)
		}
	}
}

func TestSaveAndDeleteUserInterest(t *testing.T) {
	dir := t.TempDir()
	i := &Interest{
		Meta: InterestMeta{
			ID:       "test-save",
			Name:     "Test Save",
			Source:   "user",
			Triggers: []string{"save test"},
			Requires: []string{"metrics"},
		},
		Body: "Body content.",
	}
	if err := SaveUserInterest(dir, i); err != nil {
		t.Fatalf("SaveUserInterest() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "test-save.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse saved file error = %v", err)
	}
	if parsed.Meta.ID != "test-save" {
		t.Errorf("ID = %q, want test-save", parsed.Meta.ID)
	}
	if parsed.Body != "Body content." {
		t.Errorf("Body = %q", parsed.Body)
	}
	if err := DeleteUserInterest(dir, "test-save"); err != nil {
		t.Fatalf("DeleteUserInterest() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test-save.md")); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestCatalog_Save_Valid(t *testing.T) {
	dir := t.TempDir()
	c := NewCatalog(nil)
	i := &Interest{
		Meta: InterestMeta{ID: "new-one", Name: "New", Source: "user", Requires: []string{"metrics"}},
		Body: "Hello.",
	}
	if err := c.Save(dir, i); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := c.Get("new-one"); got == nil {
		t.Error("interest not in catalog after Save")
	}
	if _, err := os.Stat(filepath.Join(dir, "new-one.md")); err != nil {
		t.Errorf("file not on disk: %v", err)
	}
}

func TestCatalog_Save_RejectsNonUser(t *testing.T) {
	c := NewCatalog(nil)
	i := &Interest{
		Meta: InterestMeta{ID: "x", Name: "X", Source: "builtin", Requires: []string{"metrics"}},
		Body: "B.",
	}
	if err := c.Save(t.TempDir(), i); err == nil {
		t.Fatal("expected error for non-user source")
	}
}

func TestCatalog_Save_RejectsBuiltinShadow(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, nil); err != nil {
		t.Fatal(err)
	}
	i := &Interest{
		Meta: InterestMeta{ID: "health-check", Name: "Override", Source: "user", Requires: []string{"metrics"}},
		Body: "B.",
	}
	if err := c.Save(t.TempDir(), i); err == nil {
		t.Fatal("expected error for shadowing built-in")
	}
}

func TestCatalog_Save_CapExceeded(t *testing.T) {
	c := NewCatalog(nil)
	dir := t.TempDir()
	for i := 0; i < MaxUserInterests; i++ {
		id := fmt.Sprintf("user-%d", i)
		c.interests[id] = &Interest{
			Meta: InterestMeta{ID: id, Name: id, Source: "user", Requires: []string{"metrics"}},
			Body: "b",
		}
	}
	i := &Interest{
		Meta: InterestMeta{ID: "overflow", Name: "Overflow", Source: "user", Requires: []string{"metrics"}},
		Body: "B.",
	}
	if err := c.Save(dir, i); err == nil {
		t.Fatal("expected error for cap exceeded")
	}
}

func TestCatalog_Save_UpdateExemptFromCap(t *testing.T) {
	c := NewCatalog(nil)
	dir := t.TempDir()
	for i := 0; i < MaxUserInterests; i++ {
		id := fmt.Sprintf("user-%d", i)
		c.interests[id] = &Interest{
			Meta: InterestMeta{ID: id, Name: id, Source: "user", Requires: []string{"metrics"}},
			Body: "b",
		}
	}
	updated := &Interest{
		Meta: InterestMeta{ID: "user-0", Name: "Updated", Source: "user", Requires: []string{"metrics"}},
		Body: "Updated body.",
	}
	if err := c.Save(dir, updated); err != nil {
		t.Fatalf("Save() for update should not be capped: %v", err)
	}
	if got := c.Get("user-0"); got.Meta.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Meta.Name, "Updated")
	}
}

func TestCatalog_Delete_Valid(t *testing.T) {
	dir := t.TempDir()
	c := NewCatalog(nil)
	i := &Interest{
		Meta: InterestMeta{ID: "del-me", Name: "Del", Source: "user", Requires: []string{"metrics"}},
		Body: "B.",
	}
	if err := c.Save(dir, i); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(dir, "del-me"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if c.Get("del-me") != nil {
		t.Error("interest still in catalog after Delete")
	}
	if _, err := os.Stat(filepath.Join(dir, "del-me.md")); !os.IsNotExist(err) {
		t.Error("file still on disk")
	}
}

func TestCatalog_Delete_RejectsBuiltin(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(t.TempDir(), "health-check"); err == nil {
		t.Fatal("expected error for deleting built-in")
	}
}

func TestCatalog_Delete_NotFound(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Delete(t.TempDir(), "nope"); err == nil {
		t.Fatal("expected error for nonexistent interest")
	}
}

func TestCatalog_UserCount(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{testBuiltinDir(t)}, nil); err != nil {
		t.Fatal(err)
	}
	if got := c.UserCount(); got != 0 {
		t.Errorf("UserCount = %d, want 0", got)
	}
	c.interests["custom"] = &Interest{
		Meta: InterestMeta{ID: "custom", Source: "user", Requires: []string{"metrics"}},
		Body: "B.",
	}
	if got := c.UserCount(); got != 1 {
		t.Errorf("UserCount = %d, want 1", got)
	}
}

func TestCatalog_BuildIndex_TargetColumn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "chat-interest.md"), []byte("---\nid: chat-one\nname: Chat Interest\nsource: builtin\ntriggers:\n  - hello\nrequires:\n  - metrics\noutput_target: chat\n---\nbody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "canvas-interest.md"), []byte("---\nid: canvas-one\nname: Canvas Interest\nsource: builtin\ntriggers:\n  - show detail\nrequires:\n  - metrics\noutput_target: canvas\n---\nbody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "default-interest.md"), []byte("---\nid: default-one\nname: Default Interest\nsource: builtin\ntriggers:\n  - test\nrequires:\n  - metrics\n---\nbody\n"), 0644)
	c := NewCatalog(nil)
	if err := c.Load([]string{dir}, nil); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	index := c.BuildIndex()
	if !strings.Contains(index, "| Target |") {
		t.Error("index header missing Target column")
	}
	if !strings.Contains(index, "| canvas-one | Canvas Interest | show detail | canvas |") {
		t.Errorf("canvas interest row incorrect in index:\n%s", index)
	}
	if !strings.Contains(index, "| chat-one | Chat Interest | hello | chat |") {
		t.Errorf("chat interest row incorrect in index:\n%s", index)
	}
	if !strings.Contains(index, "| default-one | Default Interest | test | chat |") {
		t.Errorf("default interest row incorrect in index:\n%s", index)
	}
}

func TestCatalog_Match(t *testing.T) {
	c := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := c.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := []struct {
		name    string
		message string
		wantID  string
	}{
		{"exact trigger", "health check", "health-check"},
		{"case insensitive", "Health Check", "health-check"},
		{"trigger in sentence", "can you do a health check please?", "health-check"},
		{"resource trigger", "show resources", "resource-status"},
		{"no match", "what is the meaning of life?", ""},
		{"object detail trigger", "show me details about vol1", "object-detail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Match(tt.message)
			if tt.wantID == "" {
				if got != nil {
					t.Errorf("Match(%q) = %q, want nil", tt.message, got.Meta.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("Match(%q) = nil, want %q", tt.message, tt.wantID)
			}
			if got.Meta.ID != tt.wantID {
				t.Errorf("Match(%q) = %q, want %q", tt.message, got.Meta.ID, tt.wantID)
			}
		})
	}
}

func TestCatalog_Match_Empty(t *testing.T) {
	c := NewCatalog(nil)
	if got := c.Match("hello"); got != nil {
		t.Errorf("Match on empty catalog = %v, want nil", got.Meta.ID)
	}
}
