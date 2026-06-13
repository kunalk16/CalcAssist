package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicNonStreamingMessagesAPI(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type":"text","text":"Done."},
				{"type":"tool_use","id":"tool-1","name":"lookup","input":{"query":"pi"}}
			]
		}`))
	}))
	defer server.Close()

	provider := NewAnthropic(Config{Model: "claude-test", APIKey: "test-key", BaseURL: server.URL, Temperature: 0.5})
	resp, err := provider.Chat(context.Background(), sampleMessages(), sampleTools(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Done." {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "tool-1" || resp.ToolCalls[0].Name != "lookup" || !jsonEqual(resp.ToolCalls[0].Args, `{"query":"pi"}`) {
		t.Fatalf("unexpected tool calls: %#v", resp.ToolCalls)
	}
	assertAnthropicRequest(t, gotBody)
}

func TestAnthropicStreamingMessagesAPI(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool-2","name":"lookup"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"e\"}"}}`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	provider := NewAnthropic(Config{Model: "claude-test", APIKey: "test-key", BaseURL: server.URL})
	var streamed strings.Builder
	resp, err := provider.Chat(context.Background(), sampleMessages(), sampleTools(), func(delta string) {
		streamed.WriteString(delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "Hello" {
		t.Fatalf("streamed text = %q", streamed.String())
	}
	if resp.Text != "Hello" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "tool-2" || !jsonEqual(resp.ToolCalls[0].Args, `{"query":"e"}`) {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if gotBody["stream"] != true {
		t.Fatalf("stream = %v, want true", gotBody["stream"])
	}
}

func assertAnthropicRequest(t *testing.T, body map[string]any) {
	t.Helper()
	if body["model"] != "claude-test" {
		t.Fatalf("model = %v", body["model"])
	}
	if body["max_tokens"].(float64) != 4096 {
		t.Fatalf("max_tokens = %v", body["max_tokens"])
	}
	if body["system"] != "system one\n\nsystem two" {
		t.Fatalf("system = %q", body["system"])
	}
	tools := body["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "lookup" {
		t.Fatalf("tool name = %v", tools[0].(map[string]any)["name"])
	}
	messages := body["messages"].([]any)
	first := messages[0].(map[string]any)
	if first["role"] != "user" {
		t.Fatalf("first message = %#v", first)
	}
	firstContent := first["content"].([]any)[0].(map[string]any)
	if firstContent["type"] != "text" || firstContent["text"] != "calculate" {
		t.Fatalf("first content = %#v", firstContent)
	}
	last := messages[len(messages)-1].(map[string]any)
	lastContent := last["content"].([]any)[0].(map[string]any)
	if last["role"] != "user" || lastContent["type"] != "tool_result" || lastContent["tool_use_id"] != "prior-call" || lastContent["content"] != `{"value":4}` {
		t.Fatalf("tool result message = %#v", last)
	}
}
