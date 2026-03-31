package interest

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/ebarron/netapp-chat-service/llm"
)

// getInterestInput is the expected JSON input for the get_interest tool.
type getInterestInput struct {
	ID string `json:"id"`
}

// ToolDef returns the LLM tool definition for the get_interest tool.
func ToolDef() llm.ToolDef {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The interest ID to retrieve (e.g. \"morning-coffee\").",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Name:        "get_interest",
		Description: "Retrieve a built-in or user-defined interest by ID. Returns the interest body with instructions for how to structure the response including chart panels and analysis steps.",
		Schema:      schema,
	}
}

// NewHandler returns a tool handler function that looks up interests in
// the given catalog. The handler is safe for concurrent use.
func NewHandler(catalog *Catalog) func(ctx context.Context, input json.RawMessage) (string, error) {
	return func(_ context.Context, input json.RawMessage) (string, error) {
		var req getInterestInput
		if err := json.Unmarshal(input, &req); err != nil {
			return "", fmt.Errorf("get_interest: invalid input: %w", err)
		}
		if req.ID == "" {
			return "", fmt.Errorf("get_interest: 'id' is required")
		}

		interest := catalog.Get(req.ID)
		if interest == nil {
			return "", fmt.Errorf("get_interest: interest %q not found", req.ID)
		}

		return interest.Body, nil
	}
}

// --- save_interest tool ---

// saveInterestInput is the expected JSON input for the save_interest tool.
type saveInterestInput struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Triggers     []string `json:"triggers"`
	Requires     []string `json:"requires"`
	Body         string   `json:"body"`
	OutputTarget string   `json:"output_target"`
}

// validIDPattern matches a valid interest ID: starts with lowercase letter,
// may contain lowercase letters, digits, and hyphens, max 63 characters.
var validIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// SaveToolDef returns the LLM tool definition for the save_interest tool.
func SaveToolDef() llm.ToolDef {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Unique interest identifier (lowercase letters, digits, hyphens; e.g. \"backup-status\").",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Human-readable display name for the interest.",
			},
			"triggers": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Phrases that should trigger this interest (e.g. [\"backup status\", \"how are backups\"]).",
			},
			"requires": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Required capability IDs: \"harvest\", \"ontap\", and/or \"grafana\".",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Markdown body with instructions for how to structure the response.",
			},
			"output_target": map[string]any{
				"type":        "string",
				"enum":        []string{"chat", "canvas"},
				"description": "Where to render the output: \"chat\" (inline, default) or \"canvas\" (persistent tab).",
			},
		},
		"required": []string{"id", "name", "requires", "body"},
	})
	return llm.ToolDef{
		Name:        "save_interest",
		Description: "Save a user-defined response interest. Creates or updates an interest that provides structured dashboard layouts for specific topics. Built-in interests cannot be overwritten.",
		Schema:      schema,
	}
}

// NewSaveHandler returns a tool handler for saving user-defined interests.
// validCaps is the set of capability IDs that are valid for the requires field.
func NewSaveHandler(catalog *Catalog, dir string, validCaps map[string]bool) func(ctx context.Context, input json.RawMessage) (string, error) {
	return func(_ context.Context, input json.RawMessage) (string, error) {
		var req saveInterestInput
		if err := json.Unmarshal(input, &req); err != nil {
			return "", fmt.Errorf("save_interest: invalid input: %w", err)
		}

		if req.ID == "" || req.Name == "" || len(req.Requires) == 0 || req.Body == "" {
			return "", fmt.Errorf("save_interest: 'id', 'name', 'requires', and 'body' are required")
		}

		if !validIDPattern.MatchString(req.ID) {
			return "", fmt.Errorf("save_interest: id %q is invalid (must be lowercase letters, digits, hyphens, start with a letter)", req.ID)
		}

		for _, r := range req.Requires {
			if !validCaps[r] {
				return "", fmt.Errorf("save_interest: unknown capability %q in requires", r)
			}
		}

		// Validate output_target if provided.
		if req.OutputTarget != "" && req.OutputTarget != "chat" && req.OutputTarget != "canvas" {
			return "", fmt.Errorf("save_interest: output_target must be \"chat\" or \"canvas\", got %q", req.OutputTarget)
		}

		i := &Interest{
			Meta: InterestMeta{
				ID:           req.ID,
				Name:         req.Name,
				Source:       "user",
				Triggers:     req.Triggers,
				Requires:     req.Requires,
				OutputTarget: req.OutputTarget,
			},
			Body: req.Body,
		}

		if err := catalog.Save(dir, i); err != nil {
			return "", err
		}

		return fmt.Sprintf("Interest %q saved successfully.", req.ID), nil
	}
}

// --- delete_interest tool ---

// deleteInterestInput is the expected JSON input for the delete_interest tool.
type deleteInterestInput struct {
	ID string `json:"id"`
}

// DeleteToolDef returns the LLM tool definition for the delete_interest tool.
func DeleteToolDef() llm.ToolDef {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The interest ID to delete.",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Name:        "delete_interest",
		Description: "Delete a user-defined response interest. Built-in interests cannot be deleted.",
		Schema:      schema,
	}
}

// NewDeleteHandler returns a tool handler for deleting user-defined interests.
func NewDeleteHandler(catalog *Catalog, dir string) func(ctx context.Context, input json.RawMessage) (string, error) {
	return func(_ context.Context, input json.RawMessage) (string, error) {
		var req deleteInterestInput
		if err := json.Unmarshal(input, &req); err != nil {
			return "", fmt.Errorf("delete_interest: invalid input: %w", err)
		}
		if req.ID == "" {
			return "", fmt.Errorf("delete_interest: 'id' is required")
		}

		if err := catalog.Delete(dir, req.ID); err != nil {
			return "", err
		}

		return fmt.Sprintf("Interest %q deleted successfully.", req.ID), nil
	}
}
