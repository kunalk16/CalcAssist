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
	if tools[0].(map[string]any)["name"] != "lookup" {
		t.Fatalf("tool name = %v", tools[0].(map[string]any)["name"])
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
