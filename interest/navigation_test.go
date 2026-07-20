package interest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// navigationInterest is a GENERIC parameterized navigation interest (C6),
// exercised here in isolation from any consumer. It carries narrow, imperative
// navigation triggers and its body enumerates a destination catalog; the LLM
// selects a destination from the user's phrasing and calls
// open_nav_view(destination). Destinations are intentionally generic (not
// NABox routes) — the engine hardcodes none of them.
const navigationInterest = `---
id: navigation
name: Navigate to a screen
source: builtin
triggers:
  - open
  - go to
  - bring up
  - navigate to
  - show me the
---

The user wants to navigate to a screen. Pick the matching destination from the
catalog below based on the user's phrasing and call ` + "`open_nav_view(destination)`" + ` with
that destination string. Do not render anything yourself.

| Screen | Destination |
|--------|-------------|
| Overview | overview |
| Settings | settings |
| Reports | reports |
| Profile | profile |
`

func writeNavInterest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "navigation.md"), []byte(navigationInterest), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestNavigationInterest_ParsesUngated verifies the parameterized navigation
// interest parses, is ungated (no requires), and its body references the
// open_nav_view seam.
func TestNavigationInterest_ParsesUngated(t *testing.T) {
	got, err := Parse([]byte(navigationInterest))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Meta.ID != "navigation" {
		t.Errorf("ID = %q, want navigation", got.Meta.ID)
	}
	if len(got.Meta.Requires) != 0 {
		t.Errorf("navigation interest should be ungated, got requires=%v", got.Meta.Requires)
	}
	if !strings.Contains(got.Body, "open_nav_view") {
		t.Error("body should instruct the model to call open_nav_view")
	}
	for _, dest := range []string{"overview", "settings", "reports", "profile"} {
		if !strings.Contains(got.Body, dest) {
			t.Errorf("body missing destination %q", dest)
		}
	}
}

// TestNavigationInterest_AppearsOnceInIndex verifies the ungated navigation
// interest appears exactly once in BuildIndex — even with no capabilities
// enabled (it is capability-independent).
func TestNavigationInterest_AppearsOnceInIndex(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{writeNavInterest(t)}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, enabled := range []map[string]bool{nil, {}, {"metrics": true}} {
		idx := c.BuildIndex(enabled)
		if n := strings.Count(idx, "| navigation |"); n != 1 {
			t.Errorf("BuildIndex(%v): navigation appears %d times, want 1:\n%s", enabled, n, idx)
		}
	}
}

// TestNavigationInterest_MatchAndArgResolution verifies navigation triggers
// match the interest, and that the destination catalog in the body lets the
// (caller/LLM) resolve a phrasing to the right destination argument. The
// engine matches by trigger; the destination selection lives in the body.
func TestNavigationInterest_MatchAndArgResolution(t *testing.T) {
	c := NewCatalog(nil)
	if err := c.Load([]string{writeNavInterest(t)}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Ungated ⇒ matches even with no capabilities enabled.
	for _, phrase := range []string{"open settings", "go to the reports", "bring up my profile"} {
		got := c.Match(phrase, map[string]bool{})
		if got == nil || got.Meta.ID != "navigation" {
			t.Errorf("Match(%q) = %v, want navigation", phrase, got)
		}
	}

	// The body's destination catalog resolves a screen label → destination arg.
	body := c.Get("navigation").Body
	for _, row := range []string{
		"| Overview | overview |",
		"| Settings | settings |",
		"| Reports | reports |",
		"| Profile | profile |",
	} {
		if !strings.Contains(body, row) {
			t.Errorf("catalog missing row %q", row)
		}
	}
}
