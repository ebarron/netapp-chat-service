package interest

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// MaxUserInterests is the cap on user-defined interests.
const MaxUserInterests = 10

// Catalog holds loaded interests and builds the compact index for the
// system prompt. It is the main entry point for the interest subsystem.
// All exported methods are safe for concurrent use.
type Catalog struct {
	mu        sync.RWMutex
	interests map[string]*Interest // keyed by ID
	logger    *slog.Logger
}

// NewCatalog creates an empty Catalog.
func NewCatalog(logger *slog.Logger) *Catalog {
	if logger == nil {
		logger = slog.Default()
	}
	return &Catalog{
		interests: make(map[string]*Interest),
		logger:    logger,
	}
}

// Load populates the catalog from one or more directories on disk.
// The first directory should contain built-in interests (source: builtin);
// subsequent directories contain user-defined interests (source: user).
// Either list may be empty. The enabled map keys are capability IDs that are
// currently connected; interests whose requires are not all satisfied are
// excluded from the catalog (but not from disk).
func (c *Catalog) Load(dirs []string, enabled map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interests = make(map[string]*Interest)

	for i, dir := range dirs {
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		builtin := i == 0
		dirFS := os.DirFS(dir)
		if err := c.loadFS(dirFS, builtin); err != nil {
			return fmt.Errorf("interest: load dir %s: %w", dir, err)
		}
	}

	// Filter by enabled capabilities.
	c.filterByCapabilities(enabled)

	return nil
}

// Get returns the interest with the given ID, or nil if not found.
func (c *Catalog) Get(id string) *Interest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.interests[id]
}

// All returns all loaded interests sorted by ID.
func (c *Catalog) All() []*Interest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allLocked()
}

// allLocked returns all interests sorted by ID. Caller must hold at least a read lock.
func (c *Catalog) allLocked() []*Interest {
	result := make([]*Interest, 0, len(c.interests))
	for _, i := range c.interests {
		result = append(result, i)
	}
	sort.Slice(result, func(a, b int) bool {
		return result[a].Meta.ID < result[b].Meta.ID
	})
	return result
}

// BuiltinIDs returns the set of built-in interest IDs.
func (c *Catalog) BuiltinIDs() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make(map[string]bool)
	for _, i := range c.interests {
		if i.Meta.Source == "builtin" {
			ids[i.Meta.ID] = true
		}
	}
	return ids
}

// BuildIndex produces the compact markdown table for the system prompt.
func (c *Catalog) BuildIndex() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	all := c.allLocked()
	if len(all) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("| ID | Name | Triggers | Target |\n")
	b.WriteString("|----|------|----------|--------|\n")
	for _, i := range all {
		triggers := strings.Join(i.Meta.Triggers, ", ")
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", i.Meta.ID, i.Meta.Name, triggers, i.Meta.EffectiveOutputTarget())
	}
	return b.String()
}

// loadFS walks an fs.FS and parses all .md files as interests.
func (c *Catalog) loadFS(fsys fs.FS, builtin bool) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			c.logger.Warn("interest: cannot read file", "path", path, "error", err)
			return nil
		}

		interest, err := Parse(data)
		if err != nil {
			c.logger.Warn("interest: parse error, skipping", "path", path, "error", err)
			return nil
		}

		// Enforce source tag for embedded interests.
		if builtin {
			interest.Meta.Source = "builtin"
		} else {
			interest.Meta.Source = "user"
		}

		// Reject user interest if it shadows a built-in ID.
		if !builtin {
			if existing, ok := c.interests[interest.Meta.ID]; ok && existing.Meta.Source == "builtin" {
				c.logger.Warn("interest: user interest shadows built-in, skipping",
					"id", interest.Meta.ID, "path", path)
				return nil
			}
		}

		c.interests[interest.Meta.ID] = interest
		return nil
	})
}

// filterByCapabilities removes interests whose requires are not all
// present in the enabled set. If enabled is nil, no filtering is done.
func (c *Catalog) filterByCapabilities(enabled map[string]bool) {
	if enabled == nil {
		return
	}
	for id, i := range c.interests {
		for _, req := range i.Meta.Requires {
			if !enabled[req] {
				c.logger.Debug("interest: filtered out (missing capability)",
					"id", id, "missing", req)
				delete(c.interests, id)
				break
			}
		}
	}
}

// Match returns the interest whose trigger best matches the user message,
// or nil if no trigger matches. It performs case-insensitive substring
// matching on trigger phrases and picks the longest match to prefer
// specific triggers over short ones.
func (c *Catalog) Match(message string) *Interest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lower := strings.ToLower(message)
	var best *Interest
	var bestLen int

	for _, i := range c.interests {
		for _, trigger := range i.Meta.Triggers {
			t := strings.ToLower(trigger)
			if len(t) > bestLen && strings.Contains(lower, t) {
				best = i
				bestLen = len(t)
			}
		}
	}
	return best
}

// Save persists a user-defined interest to disk and updates the in-memory
// catalog. It validates that the interest is user-sourced, does not shadow a
// built-in, and respects the user interest cap (for new interests).
func (c *Catalog) Save(dir string, i *Interest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if i.Meta.Source != "user" {
		return fmt.Errorf("interest: source must be \"user\"")
	}

	// Reject shadowing a built-in interest.
	if existing, ok := c.interests[i.Meta.ID]; ok && existing.Meta.Source == "builtin" {
		return fmt.Errorf("interest: cannot overwrite built-in interest %q", i.Meta.ID)
	}

	// Cap check for new interests (updates are exempt).
	if _, exists := c.interests[i.Meta.ID]; !exists {
		userCount := 0
		for _, existing := range c.interests {
			if existing.Meta.Source == "user" {
				userCount++
			}
		}
		if userCount >= MaxUserInterests {
			return fmt.Errorf("interest: user interest limit (%d) reached", MaxUserInterests)
		}
	}

	if err := SaveUserInterest(dir, i); err != nil {
		return fmt.Errorf("interest: save to disk: %w", err)
	}

	c.interests[i.Meta.ID] = i
	return nil
}

// Delete removes a user-defined interest from disk and the in-memory catalog.
// Built-in interests cannot be deleted.
func (c *Catalog) Delete(dir string, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing, ok := c.interests[id]
	if !ok {
		return fmt.Errorf("interest: %q not found", id)
	}
	if existing.Meta.Source == "builtin" {
		return fmt.Errorf("interest: cannot delete built-in interest %q", id)
	}

	if err := DeleteUserInterest(dir, id); err != nil {
		return fmt.Errorf("interest: delete from disk: %w", err)
	}

	delete(c.interests, id)
	return nil
}

// UserCount returns the number of user-defined interests in the catalog.
func (c *Catalog) UserCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := 0
	for _, i := range c.interests {
		if i.Meta.Source == "user" {
			count++
		}
	}
	return count
}

// SaveUserInterest validates and writes a user-defined interest file to dir.
func SaveUserInterest(dir string, i *Interest) error {
	if i.Meta.Source != "user" {
		return fmt.Errorf("interest: cannot save non-user interest %q", i.Meta.ID)
	}
	path := filepath.Join(dir, i.Meta.ID+".md")
	content := formatInterestFile(i)
	return os.WriteFile(path, []byte(content), 0644)
}

// DeleteUserInterest removes a user-defined interest file from dir.
func DeleteUserInterest(dir, id string) error {
	path := filepath.Join(dir, id+".md")
	return os.Remove(path)
}

func formatInterestFile(i *Interest) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", i.Meta.ID)
	fmt.Fprintf(&b, "name: %s\n", i.Meta.Name)
	fmt.Fprintf(&b, "source: %s\n", i.Meta.Source)
	if len(i.Meta.Triggers) > 0 {
		b.WriteString("triggers:\n")
		for _, t := range i.Meta.Triggers {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	if len(i.Meta.Requires) > 0 {
		b.WriteString("requires:\n")
		for _, r := range i.Meta.Requires {
			fmt.Fprintf(&b, "  - %s\n", r)
		}
	}
	if i.Meta.OutputTarget == "canvas" {
		b.WriteString("output_target: canvas\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(i.Body)
	b.WriteString("\n")
	return b.String()
}
