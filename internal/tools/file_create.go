package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type createFileTool struct{}

type createFileArgs struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite"`
}

// NewCreateFileTool returns a tool that writes text content to a file.
func NewCreateFileTool() Tool { return createFileTool{} }

func (createFileTool) Name() string { return "create_file" }

func (createFileTool) Description() string {
	return "Create a file with the provided content, creating parent directories as needed."
}

func (createFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Path of the file to create."},
			"content":   map[string]any{"type": "string", "description": "Text content to write."},
			"overwrite": map[string]any{"type": "boolean", "description": "Overwrite the file if it already exists. Defaults to false."},
		},
		"required": []string{"path", "content"},
	}
}

func (createFileTool) Mutating() bool { return true }

func (createFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	var parsed createFileArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid create_file arguments: %w", err)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	if !parsed.Overwrite {
		if _, err := os.Stat(parsed.Path); err == nil {
			return "", fmt.Errorf("file %q already exists; set overwrite=true to replace it", parsed.Path)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("cannot inspect %q: %w", parsed.Path, err)
		}
	}

	parent := filepath.Dir(parsed.Path)
	if parent != "." && parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", fmt.Errorf("create parent directories for %q: %w", parsed.Path, err)
		}
	}
	if err := os.WriteFile(parsed.Path, []byte(parsed.Content), 0o644); err != nil {
		return "", fmt.Errorf("write %q: %w", parsed.Path, err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len([]byte(parsed.Content)), parsed.Path), nil
}
