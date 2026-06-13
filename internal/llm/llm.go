// Package llm defines provider-neutral types and the Provider interface used to talk
// to OpenAI, Azure OpenAI (both via the Responses API) and Anthropic (Messages API).
package llm

import "context"

// Role identifies the author of a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single conversation turn in a provider-neutral form. Each provider
// adapter is responsible for translating these into/out of its own wire format.
type Message struct {
	Role Role
	// Content is the text body for system/user/assistant messages, or the tool
	// result payload when Role == RoleTool.
	Content string
	// ToolCalls holds the tool invocations requested by the assistant
	// (only set when Role == RoleAssistant).
	ToolCalls []ToolCall
	// ToolCallID links a Role == RoleTool message to the ToolCall it answers.
	ToolCallID string
	// ToolName is the name of the tool that produced a Role == RoleTool result.
	ToolName string
}

// ToolDef describes a tool that is exposed to the model.
type ToolDef struct {
	Name        string
	Description string
	// Schema is the JSON Schema (as a decoded map) for the tool's arguments object.
	Schema map[string]any
}

// ToolCall is a request from the model to invoke a tool.
type ToolCall struct {
	ID   string
	Name string
	// Args is the raw JSON arguments object emitted by the model.
	Args string
}

// Response is the assistant turn returned by a Provider. When ToolCalls is
// non-empty the caller is expected to execute them and continue the loop.
type Response struct {
	Text      string
	ToolCalls []ToolCall
}

// Config holds provider-neutral connection settings. The application config is
// mapped into this struct so the llm package never imports the config package.
type Config struct {
	// Provider is one of: "azure", "openai", "anthropic".
	Provider string
	// Model is the OpenAI model id, the Azure deployment name, or the Anthropic model id.
	Model string
	// APIKey authenticates the request.
	APIKey string
	// BaseURL overrides the default endpoint. For Azure it must be the v1 base,
	// e.g. https://<resource>.openai.azure.com/openai/v1
	BaseURL string
	// MaxTokens caps the response length. Zero means use a provider default.
	MaxTokens int
	// Temperature controls sampling. Negative means use a provider default.
	Temperature float64
}

// Provider is implemented by each LLM backend.
type Provider interface {
	// Name returns a short identifier, e.g. "openai", "azure" or "anthropic".
	Name() string
	// Chat sends the conversation plus tool definitions and returns the assistant
	// turn. If onText is non-nil it is called with incremental assistant text as
	// it streams in; the same text is also accumulated into Response.Text.
	Chat(ctx context.Context, messages []Message, tools []ToolDef, onText func(string)) (*Response, error)
}
