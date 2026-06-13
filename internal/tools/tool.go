// Package tools defines the Tool interface, a registry, and the concrete tools
// the agent can invoke (file operations, math/statistics, Excel→JSON, PDF and DOCX reading).
package tools

import (
	"context"
	"encoding/json"
)

// Tool is a single capability the agent can invoke.
type Tool interface {
	// Name is the unique snake_case identifier exposed to the LLM.
	Name() string
	// Description tells the model when and how to use the tool.
	Description() string
	// Schema returns the JSON Schema (as a decoded map) for the arguments object.
	Schema() map[string]any
	// Mutating reports whether executing the tool modifies the filesystem and
	// therefore requires user confirmation before it runs.
	Mutating() bool
	// Execute runs the tool with the raw JSON arguments object and returns a
	// human/LLM-readable result string.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}
