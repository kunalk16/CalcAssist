package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAINonStreamingResponsesAPI(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		wantAuth      string
		wantAPIKey    string
		wantNoAuthHdr bool
	}{
		{name: "openai", provider: "openai", wantAuth: "Bearer test-key"},
		{name: "azure", provider: "azure", wantAPIKey: "test-key", wantNoAuthHdr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" {
					t.Fatalf("path = %q, want /responses", r.URL.Path)
				}
				if tt.wantAuth != "" && r.Header.Get("Authorization") != tt.wantAuth {
					t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), tt.wantAuth)
				}
				if tt.wantAPIKey != "" && r.Header.Get("api-key") != tt.wantAPIKey {
					t.Fatalf("api-key = %q, want %q", r.Header.Get("api-key"), tt.wantAPIKey)
				}
				if tt.wantNoAuthHdr && r.Header.Get("Authorization") != "" {
					t.Fatalf("Authorization header was set for Azure: %q", r.Header.Get("Authorization"))
				}
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"output": [
						{"type":"message","content":[{"type":"output_text","text":"The answer is 4."}]},
						{"type":"function_call","call_id":"call-1","name":"lookup","arguments":"{\"query\":\"pi\"}"}
					]
				}`))
			}))
			defer server.Close()

			provider, err := New(Config{Provider: tt.provider, Model: "model-1", APIKey: "test-key", BaseURL: server.URL, MaxTokens: 100, Temperature: 0.2})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := provider.Chat(context.Background(), sampleMessages(), sampleTools(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Text != "The answer is 4." {
				t.Fatalf("Text = %q", resp.Text)
			}
			if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call-1" || resp.ToolCalls[0].Name != "lookup" || !jsonEqual(resp.ToolCalls[0].Args, `{"query":"pi"}`) {
				t.Fatalf("unexpected tool calls: %#v", resp.ToolCalls)
			}
			assertOpenAIRequest(t, gotBody)
		})
	}
}

func TestOpenAIStreamingResponsesAPI(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"Hel"}`,
			`data: {"type":"response.output_text.delta","delta":"lo"}`,
			`data: {"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"Hello"}]},{"type":"function_call","call_id":"call-2","name":"lookup","arguments":"{\"query\":\"e\"}"}]}}`,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	provider := NewOpenAI(Config{Model: "model-1", APIKey: "test-key", BaseURL: server.URL})
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
	if resp.Text != "Hello" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call-2" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if gotBody["stream"] != true {
		t.Fatalf("stream = %v, want true", gotBody["stream"])
	}
}

func assertOpenAIRequest(t *testing.T, body map[string]any) {
	t.Helper()
	if body["model"] != "model-1" {
		t.Fatalf("model = %v", body["model"])
	}
	if body["instructions"] != "system one\n\nsystem two" {
		t.Fatalf("instructions = %q", body["instructions"])
	}
	tools := body["tools"].([]any)
	if !hasOpenAIFunctionTool(tools, "lookup") {
		t.Fatalf("missing lookup function tool: %#v", tools)
	}
	if hasOpenAIToolType(tools, "web_search") {
		t.Fatalf("web_search tool present when WebSearch disabled: %#v", tools)
	}
	input := body["input"].([]any)
	if input[0].(map[string]any)["role"] != "user" || input[0].(map[string]any)["content"] != "calculate" {
		t.Fatalf("first input item = %#v", input[0])
	}
	last := input[len(input)-1].(map[string]any)
	if last["type"] != "function_call_output" || last["call_id"] != "prior-call" || last["output"] != `{"value":4}` {
		t.Fatalf("tool output input item = %#v", last)
	}
}

func sampleMessages() []Message {
	return []Message{
		{Role: RoleSystem, Content: "system one"},
		{Role: RoleSystem, Content: "system two"},
		{Role: RoleUser, Content: "calculate"},
		{Role: RoleAssistant, Content: "using a tool", ToolCalls: []ToolCall{{ID: "prior-call", Name: "lookup", Args: `{"query":"two plus two"}`}}},
		{Role: RoleTool, ToolCallID: "prior-call", ToolName: "lookup", Content: `{"value":4}`},
	}
}

func sampleTools() []ToolDef {
	return []ToolDef{{
		Name:        "lookup",
		Description: "look something up",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
	}}
}

func jsonEqual(a, b string) bool {
	var av any
	var bv any
	return json.Unmarshal([]byte(a), &av) == nil && json.Unmarshal([]byte(b), &bv) == nil && reflect.DeepEqual(av, bv)
}

func hasOpenAIFunctionTool(tools []any, name string) bool {
	for _, raw := range tools {
		if m, ok := raw.(map[string]any); ok && m["type"] == "function" && m["name"] == name {
			return true
		}
	}
	return false
}

func hasOpenAIToolType(tools []any, typ string) bool {
	for _, raw := range tools {
		if m, ok := raw.(map[string]any); ok && m["type"] == typ {
			return true
		}
	}
	return false
}

func TestOpenAIWebSearchCitations(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": [
				{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go release"}},
				{"type":"message","content":[{"type":"output_text","text":"Go 1.26 is out.","annotations":[
					{"type":"url_citation","url":"https://go.dev/a","title":"A","start_index":0,"end_index":5},
					{"type":"url_citation","url":"https://go.dev/b","title":"B","start_index":6,"end_index":10},
					{"type":"url_citation","url":"https://go.dev/a","title":"A","start_index":11,"end_index":14}
				]}]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewOpenAI(Config{Model: "model-1", APIKey: "test-key", BaseURL: server.URL, WebSearch: true})
	resp, err := provider.Chat(context.Background(), sampleMessages(), sampleTools(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := gotBody["tools"].([]any)
	if !hasOpenAIToolType(tools, "web_search") {
		t.Fatalf("web_search tool not advertised: %#v", tools)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("expected no client tool calls, got %#v", resp.ToolCalls)
	}
	if len(resp.Citations) != 2 || resp.Citations[0].URL != "https://go.dev/a" || resp.Citations[1].URL != "https://go.dev/b" {
		t.Fatalf("unexpected citations: %#v", resp.Citations)
	}
	if !strings.Contains(resp.Text, "Go 1.26 is out.") || !strings.Contains(resp.Text, "**Sources:**") {
		t.Fatalf("missing text or sources: %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "[A](https://go.dev/a)") || !strings.Contains(resp.Text, "[B](https://go.dev/b)") {
		t.Fatalf("missing source links: %q", resp.Text)
	}
}

func TestOpenAIWebSearchStreamingCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"Latest"}`,
			`data: {"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"search"}},{"type":"message","content":[{"type":"output_text","text":"Latest","annotations":[{"type":"url_citation","url":"https://ex.com/x","title":"X","start_index":0,"end_index":6}]}]}]}}`,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	provider := NewOpenAI(Config{Model: "model-1", APIKey: "test-key", BaseURL: server.URL, WebSearch: true})
	var streamed strings.Builder
	resp, err := provider.Chat(context.Background(), sampleMessages(), sampleTools(), func(delta string) {
		streamed.WriteString(delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "Latest" {
		t.Fatalf("streamed text = %q", streamed.String())
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %#v", resp.ToolCalls)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].URL != "https://ex.com/x" {
		t.Fatalf("citations = %#v", resp.Citations)
	}
	if !strings.Contains(resp.Text, "**Sources:**") || !strings.Contains(resp.Text, "[X](https://ex.com/x)") {
		t.Fatalf("missing sources in text: %q", resp.Text)
	}
}
