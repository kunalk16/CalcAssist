package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"calcassist/internal/llm"
	"calcassist/internal/tools"
)

// mockProvider returns scripted responses in order, clamping to the last one.
type mockProvider struct {
	responses   []*llm.Response
	idx         int
	calls       int
	lastTools   []llm.ToolDef
	lastHistory []llm.Message
	streamed    string
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDef, onText func(string)) (*llm.Response, error) {
	m.calls++
	m.lastTools = toolDefs
	m.lastHistory = append([]llm.Message(nil), messages...)
	r := m.responses[min(m.idx, len(m.responses)-1)]
	m.idx++
	if onText != nil && r.Text != "" {
		onText(r.Text)
		m.streamed += r.Text
	}
	return r, nil
}

// mockTool records invocations.
type mockTool struct {
	name     string
	mutating bool
	calls    int
	result   string
	lastArgs string
}

func (t *mockTool) Name() string           { return t.name }
func (t *mockTool) Description() string    { return "mock tool" }
func (t *mockTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (t *mockTool) Mutating() bool         { return t.mutating }
func (t *mockTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	t.calls++
	t.lastArgs = string(args)
	return t.result, nil
}

func newRegistry(tls ...tools.Tool) *tools.Registry {
	r := tools.NewRegistry()
	for _, t := range tls {
		r.Register(t)
	}
	return r
}

func TestRunExecutesToolThenAnswers(t *testing.T) {
	tool := &mockTool{name: "echo", result: "echoed!"}
	prov := &mockProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Args: `{"x":1}`}}},
		{Text: "All done."},
	}}
	a := New(prov, newRegistry(tool), "system", 12)

	out, err := a.Run(context.Background(), "do it", Hooks{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if out != "All done." {
		t.Fatalf("got %q want %q", out, "All done.")
	}
	if tool.calls != 1 {
		t.Fatalf("tool calls = %d want 1", tool.calls)
	}
	if tool.lastArgs != `{"x":1}` {
		t.Fatalf("tool args = %q", tool.lastArgs)
	}
	// The second Chat must have seen the tool result in history.
	var sawToolResult bool
	for _, msg := range prov.lastHistory {
		if msg.Role == llm.RoleTool && msg.ToolCallID == "c1" && strings.Contains(msg.Content, "echoed!") {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatalf("tool result not found in history: %+v", prov.lastHistory)
	}
	// Tool defs must have been advertised to the provider.
	if len(prov.lastTools) != 1 || prov.lastTools[0].Name != "echo" {
		t.Fatalf("tool defs not advertised: %+v", prov.lastTools)
	}
}

func TestRunStreamsText(t *testing.T) {
	prov := &mockProvider{responses: []*llm.Response{{Text: "hello world"}}}
	a := New(prov, newRegistry(), "", 12)
	var got strings.Builder
	out, err := a.Run(context.Background(), "hi", Hooks{OnText: func(s string) { got.WriteString(s) }})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello world" || got.String() != "hello world" {
		t.Fatalf("out=%q streamed=%q", out, got.String())
	}
}

func TestMaxIterations(t *testing.T) {
	tool := &mockTool{name: "loop", result: "again"}
	prov := &mockProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c", Name: "loop", Args: `{}`}}},
	}}
	a := New(prov, newRegistry(tool), "", 3)

	_, err := a.Run(context.Background(), "go", Hooks{})
	if err == nil || !strings.Contains(err.Error(), "maximum of 3") {
		t.Fatalf("expected max-iteration error, got %v", err)
	}
	if tool.calls != 3 {
		t.Fatalf("tool calls = %d want 3", tool.calls)
	}
}

func TestConfirmDecline(t *testing.T) {
	tool := &mockTool{name: "write", mutating: true, result: "written"}
	prov := &mockProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write", Args: `{}`}}},
		{Text: "ok, skipped"},
	}}
	a := New(prov, newRegistry(tool), "", 12)

	out, err := a.Run(context.Background(), "save", Hooks{
		Confirm: func(t tools.Tool, args string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if tool.calls != 0 {
		t.Fatalf("declined tool should not run, calls = %d", tool.calls)
	}
	if out != "ok, skipped" {
		t.Fatalf("got %q", out)
	}
	var sawDecline bool
	for _, msg := range prov.lastHistory {
		if msg.Role == llm.RoleTool && strings.Contains(strings.ToLower(msg.Content), "declined") {
			sawDecline = true
		}
	}
	if !sawDecline {
		t.Fatalf("decline message not fed back: %+v", prov.lastHistory)
	}
}

func TestConfirmApprove(t *testing.T) {
	tool := &mockTool{name: "write", mutating: true, result: "written"}
	prov := &mockProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write", Args: `{}`}}},
		{Text: "saved"},
	}}
	a := New(prov, newRegistry(tool), "", 12)

	var confirmedTool string
	out, err := a.Run(context.Background(), "save", Hooks{
		Confirm: func(t tools.Tool, args string) (bool, error) { confirmedTool = t.Name(); return true, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmedTool != "write" || tool.calls != 1 || out != "saved" {
		t.Fatalf("confirmed=%q calls=%d out=%q", confirmedTool, tool.calls, out)
	}
}

func TestUnknownToolFedBack(t *testing.T) {
	prov := &mockProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "ghost", Args: `{}`}}},
		{Text: "recovered"},
	}}
	a := New(prov, newRegistry(), "", 12)
	out, err := a.Run(context.Background(), "x", Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "recovered" {
		t.Fatalf("got %q", out)
	}
	var sawUnknown bool
	for _, msg := range prov.lastHistory {
		if msg.Role == llm.RoleTool && strings.Contains(msg.Content, "unknown tool") {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Fatalf("unknown tool not reported back: %+v", prov.lastHistory)
	}
}

func TestResetKeepsSystemPrompt(t *testing.T) {
	prov := &mockProvider{responses: []*llm.Response{{Text: "x"}}}
	a := New(prov, newRegistry(), "sys", 12)
	if _, err := a.Run(context.Background(), "hi", Hooks{}); err != nil {
		t.Fatal(err)
	}
	a.Reset()
	if len(a.history) != 1 || a.history[0].Role != llm.RoleSystem {
		t.Fatalf("reset should keep only system prompt, got %+v", a.history)
	}
}
