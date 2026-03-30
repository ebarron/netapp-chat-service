package interest

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// InterestMeta holds the YAML frontmatter from an interest file.
type InterestMeta struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Source       string   `yaml:"source"`        // "builtin" or "user"
	Triggers     []string `yaml:"triggers"`      // phrases that signal this interest
	Requires     []string `yaml:"requires"`      // capability IDs
	OutputTarget string   `yaml:"output_target"` // "canvas" or "chat" (default: "chat")
}

// EffectiveOutputTarget returns the output target, defaulting to "chat"
// when the field is empty or omitted.
func (m InterestMeta) EffectiveOutputTarget() string {
	if m.OutputTarget == "canvas" {
		return "canvas"
	}
	return "chat"
}

// Interest represents a fully parsed interest file.
type Interest struct {
	Meta InterestMeta
	Body string // markdown body below the frontmatter
}

var frontmatterDelimiter = []byte("---")

// Parse reads a markdown file (as bytes) and splits it into YAML
// frontmatter and body.
func Parse(data []byte) (*Interest, error) {
	meta, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var m InterestMeta
	if err := yaml.Unmarshal(meta, &m); err != nil {
		return nil, fmt.Errorf("interest: invalid YAML frontmatter: %w", err)
	}

	if m.ID == "" {
		return nil, fmt.Errorf("interest: missing required field 'id'")
	}
	if m.Name == "" {
		return nil, fmt.Errorf("interest: missing required field 'name'")
	}
	if len(m.Requires) == 0 {
		return nil, fmt.Errorf("interest: missing required field 'requires'")
	}

	return &Interest{
		Meta: m,
		Body: strings.TrimSpace(body),
	}, nil
}

func splitFrontmatter(data []byte) (meta []byte, body string, err error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if !bytes.HasPrefix(trimmed, frontmatterDelimiter) {
		return nil, "", fmt.Errorf("interest: file does not start with YAML frontmatter (---)")
	}

	rest := trimmed[len(frontmatterDelimiter):]
	if idx := bytes.IndexByte(rest, '\n'); idx >= 0 {
		rest = rest[idx+1:]
	} else {
		return nil, "", fmt.Errorf("interest: frontmatter has no closing delimiter")
	}

	idx := bytes.Index(rest, frontmatterDelimiter)
	if idx < 0 {
		return nil, "", fmt.Errorf("interest: frontmatter has no closing delimiter")
	}

	meta = rest[:idx]

	after := rest[idx+len(frontmatterDelimiter):]
	if nlIdx := bytes.IndexByte(after, '\n'); nlIdx >= 0 {
		body = string(after[nlIdx+1:])
	}

	return meta, body, nil
}
