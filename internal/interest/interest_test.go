package interest

import (
	"reflect"
	"testing"
)

const validFile = `---
id: morning-coffee
name: Fleet Health Overview
source: builtin
triggers:
  - how's everything
  - any issues
  - summary
requires:
  - harvest
---

When the user wants a health check, produce a dashboard.
`

func TestParse_Valid(t *testing.T) {
	got, err := Parse([]byte(validFile))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	wantMeta := InterestMeta{
		ID:       "morning-coffee",
		Name:     "Fleet Health Overview",
		Source:   "builtin",
		Triggers: []string{"how's everything", "any issues", "summary"},
		Requires: []string{"harvest"},
	}
	if !reflect.DeepEqual(got.Meta, wantMeta) {
		t.Errorf("Meta = %+v, want %+v", got.Meta, wantMeta)
	}
	if got.Body != "When the user wants a health check, produce a dashboard." {
		t.Errorf("Body = %q", got.Body)
	}
}

func TestParse_MissingID(t *testing.T) {
	input := "---\nname: Test\nsource: user\ntriggers:\n  - hello\nrequires:\n  - harvest\n---\nbody\n"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestParse_MissingName(t *testing.T) {
	input := "---\nid: test\nsource: user\ntriggers:\n  - hello\nrequires:\n  - harvest\n---\nbody\n"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParse_MissingRequires(t *testing.T) {
	input := "---\nid: test\nname: Test\nsource: user\ntriggers:\n  - hello\n---\nbody\n"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("expected error for missing requires")
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	if _, err := Parse([]byte("Just a regular markdown file.")); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParse_MalformedYAML(t *testing.T) {
	input := "---\nid: [invalid yaml\n---\nbody\n"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParse_NoClosingDelimiter(t *testing.T) {
	input := "---\nid: test\nname: Test\n"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("expected error for missing closing delimiter")
	}
}

func TestParse_MultipleRequires(t *testing.T) {
	input := "---\nid: volume-provision\nname: Smart Volume Placement\nsource: builtin\ntriggers:\n  - provision a volume\nrequires:\n  - harvest\n  - ontap\n---\n\nInstructions for provisioning.\n"
	got, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !reflect.DeepEqual(got.Meta.Requires, []string{"harvest", "ontap"}) {
		t.Errorf("Requires = %v, want [harvest ontap]", got.Meta.Requires)
	}
}

func TestParse_EmptyBody(t *testing.T) {
	input := "---\nid: empty\nname: Empty Body\nsource: user\ntriggers:\n  - hello\nrequires:\n  - harvest\n---\n"
	got, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Body != "" {
		t.Errorf("Body = %q, want empty", got.Body)
	}
}

func TestParse_LeadingWhitespace(t *testing.T) {
	input := "\n\n---\nid: whitespace\nname: Whitespace Test\nsource: user\ntriggers:\n  - test\nrequires:\n  - harvest\n---\nbody here\n"
	got, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Meta.ID != "whitespace" {
		t.Errorf("ID = %q, want %q", got.Meta.ID, "whitespace")
	}
}

func TestParse_OutputTarget(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTarget string
	}{
		{
			name:       "canvas",
			input:      "---\nid: t1\nname: T1\nsource: user\ntriggers:\n  - test\nrequires:\n  - harvest\noutput_target: canvas\n---\nbody\n",
			wantTarget: "canvas",
		},
		{
			name:       "chat",
			input:      "---\nid: t2\nname: T2\nsource: user\ntriggers:\n  - test\nrequires:\n  - harvest\noutput_target: chat\n---\nbody\n",
			wantTarget: "chat",
		},
		{
			name:       "omitted defaults to chat",
			input:      "---\nid: t3\nname: T3\nsource: user\ntriggers:\n  - test\nrequires:\n  - harvest\n---\nbody\n",
			wantTarget: "chat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got.Meta.EffectiveOutputTarget() != tt.wantTarget {
				t.Errorf("EffectiveOutputTarget() = %q, want %q", got.Meta.EffectiveOutputTarget(), tt.wantTarget)
			}
		})
	}
}

func TestBuiltinInterests_CanvasTarget(t *testing.T) {
	// Verify built-in interests with output_target: canvas parse correctly.
	cat := NewCatalog(nil)
	enabled := map[string]bool{"metrics": true, "storage": true}
	if err := cat.Load([]string{testBuiltinDir(t)}, enabled); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	interest := cat.Get("object-detail")
	if interest == nil {
		t.Fatal("object-detail interest not found in catalog")
	}
	if interest.Meta.EffectiveOutputTarget() != "canvas" {
		t.Errorf("object-detail: EffectiveOutputTarget() = %q, want %q",
			interest.Meta.EffectiveOutputTarget(), "canvas")
	}
}
