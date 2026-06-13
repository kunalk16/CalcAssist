// Package agent implements the multi-step agentic loop: it sends the conversation
// plus tool definitions to an llm.Provider, executes any requested tools (asking for
// confirmation on mutating ones), feeds the results back, and repeats until the model
// returns a final text answer or a hard iteration cap is reached.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"calcassist/internal/llm"
	"calcassist/internal/tools"
)

// Hooks lets the caller (TUI or one-shot CLI) observe and control the loop.
// Any field may be nil.
type Hooks struct {
	// OnText streams incremental assistant text as it arrives from the model.
	OnText func(string)
	// OnToolCall is invoked just before a tool executes (after any confirmation).
	OnToolCall func(name, args string)
	// OnToolResult is invoked after a tool finishes.
	OnToolResult func(name, result string, err error)
	// Confirm is consulted before a mutating tool runs. Return true to proceed.
	// If nil, mutating tools run without confirmation.
	Confirm func(t tools.Tool, args string) (bool, error)
}

// Agent orchestrates a single conversation against a provider and tool registry.
type Agent struct {
	provider llm.Provider
	registry *tools.Registry
	toolDefs []llm.ToolDef
	history  []llm.Message
	maxIters int
}

// New creates an Agent. If systemPrompt is non-empty it seeds the conversation.
// maxIters caps tool-call rounds (defaults to 12 when <= 0).
func New(provider llm.Provider, registry *tools.Registry, systemPrompt string, maxIters int) *Agent {
	if maxIters <= 0 {
		maxIters = 12
	}
	defs := make([]llm.ToolDef, 0, registry.Len())
	for _, t := range registry.All() {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	a := &Agent{
		provider: provider,
		registry: registry,
		toolDefs: defs,
		maxIters: maxIters,
	}
	if strings.TrimSpace(systemPrompt) != "" {
		a.history = append(a.history, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})
	}
	return a
}

// Reset clears the conversation history, preserving the system prompt.
func (a *Agent) Reset() {
	if len(a.history) > 0 && a.history[0].Role == llm.RoleSystem {
		a.history = a.history[:1]
		return
	}
	a.history = nil
}

// Provider returns the underlying provider name (for status display).
func (a *Agent) Provider() string { return a.provider.Name() }

// Run processes one user input, looping over tool calls until the model produces a
// final text answer or maxIters is reached. It returns the final assistant text.
func (a *Agent) Run(ctx context.Context, userInput string, h Hooks) (string, error) {
	a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: userInput})

	for i := 0; i < a.maxIters; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		resp, err := a.provider.Chat(ctx, a.history, a.toolDefs, h.OnText)
		if err != nil {
			return "", err
		}
		a.history = append(a.history, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		if len(resp.ToolCalls) == 0 {
			return resp.Text, nil
		}

		for _, tc := range resp.ToolCalls {
			result, err := a.execTool(ctx, tc, h)
			if err != nil {
				// Hard failures (context cancelled, user-confirm error) abort the loop.
				return "", err
			}
			a.history = append(a.history, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			})
		}
	}
	return "", fmt.Errorf("reached the maximum of %d tool iterations without a final answer", a.maxIters)
}

// execTool runs a single tool call. Tool-level errors are returned as result strings
// (so the model can recover); only fatal conditions return a non-nil error.
func (a *Agent) execTool(ctx context.Context, tc llm.ToolCall, h Hooks) (string, error) {
	t, ok := a.registry.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q. Available tools: %s", tc.Name, a.toolNames()), nil
	}

	if t.Mutating() && h.Confirm != nil {
		approved, err := h.Confirm(t, tc.Args)
		if err != nil {
			return "", err
		}
		if !approved {
			if h.OnToolResult != nil {
				h.OnToolResult(tc.Name, "declined by user", nil)
			}
			return "The user declined to run this tool. Do not retry it automatically; ask the user how they would like to proceed.", nil
		}
	}

	if h.OnToolCall != nil {
		h.OnToolCall(tc.Name, tc.Args)
	}
	res, execErr := t.Execute(ctx, json.RawMessage(tc.Args))
	if h.OnToolResult != nil {
		h.OnToolResult(tc.Name, res, execErr)
	}
	if execErr != nil {
		return fmt.Sprintf("Tool %q failed: %v", tc.Name, execErr), nil
	}
	return res, nil
}

func (a *Agent) toolNames() string {
	names := make([]string, 0, len(a.toolDefs))
	for _, d := range a.toolDefs {
		names = append(names, d.Name)
	}
	return strings.Join(names, ", ")
}

// SystemPrompt builds the default system prompt, embedding runtime context.
func SystemPrompt(cwd string) string {
	var b strings.Builder
	b.WriteString("You are CalcAssist, a helpful command-line calculation and file assistant.\n")
	b.WriteString("You operate inside a user's terminal and can call tools to inspect and modify their files.\n\n")
	b.WriteString("Guidelines:\n")
	b.WriteString("- Use the provided tools to do real work instead of guessing; read files, sheets, PDFs and documents via tools rather than assuming their contents.\n")
	b.WriteString("- For any calculation, use the calculate or statistics tools rather than doing arithmetic yourself.\n")
	b.WriteString("- To create or save files (including saving converted JSON), use the create_file tool.\n")
	b.WriteString("- Break multi-step requests into a sequence of tool calls, then give a short, clear final answer in Markdown.\n")
	b.WriteString("- Be concise. Do not invent file paths; if unsure, list the directory first.\n\n")
	b.WriteString(fmt.Sprintf("Environment: OS=%s, working directory=%s\n", runtime.GOOS, cwd))
	return b.String()
}
