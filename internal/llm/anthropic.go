package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

const defaultAnthropicBaseURL = "https://api.anthropic.com"

type anthropicProvider struct {
	cfg     Config
	client  *http.Client
	baseURL string
}

// NewAnthropic returns an Anthropic Messages API provider.
func NewAnthropic(cfg Config) Provider {
	return newAnthropicProvider(cfg, nil)
}

func newAnthropicProvider(cfg Config, client *http.Client) *anthropicProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	if client == nil {
		client = defaultHTTPClient()
	}
	return &anthropicProvider{cfg: cfg, client: client, baseURL: baseURL}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef, onText func(string)) (*Response, error) {
	body, err := p.buildRequest(messages, tools, onText != nil)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpStatusError(resp)
	}
	if onText != nil {
		return parseAnthropicStream(resp.Body, onText)
	}
	return parseAnthropicResponse(resp.Body)
}

func (p *anthropicProvider) buildRequest(messages []Message, tools []ToolDef, stream bool) (map[string]any, error) {
	maxTokens := p.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	req := map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": maxTokens,
		"messages":   []map[string]any{},
		"stream":     stream,
	}
	if system := systemInstructions(messages); system != "" {
		req["system"] = system
	}
	if p.cfg.Temperature >= 0 {
		req["temperature"] = p.cfg.Temperature
	}
	anthropicMessages, err := anthropicMessages(messages)
	if err != nil {
		return nil, err
	}
	req["messages"] = anthropicMessages
	reqTools := anthropicTools(tools)
	if p.cfg.WebSearch {
		reqTools = append(reqTools, map[string]any{
			"type":     "web_search_20250305",
			"name":     "web_search",
			"max_uses": 5,
		})
	}
	if len(reqTools) > 0 {
		req["tools"] = reqTools
	}
	return req, nil
}

func anthropicMessages(messages []Message) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			continue
		case RoleUser:
			out = append(out, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": msg.Content}}})
		case RoleAssistant:
			content := make([]map[string]any, 0, 1+len(msg.ToolCalls))
			if msg.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, call := range msg.ToolCalls {
				input, err := rawJSONArg(call.Args)
				if err != nil {
					return nil, err
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
		case RoleTool:
			out = append(out, map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": msg.ToolCallID, "content": msg.Content}}})
		}
	}
	return out, nil
}

func rawJSONArg(args string) (json.RawMessage, error) {
	if strings.TrimSpace(args) == "" {
		return json.RawMessage(`{}`), nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func anthropicTools(tools []ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.Schema})
	}
	return out
}

func parseAnthropicResponse(r io.Reader) (*Response, error) {
	var envelope struct {
		Content []anthropicContentBlock `json:"content"`
	}
	if err := json.NewDecoder(r).Decode(&envelope); err != nil {
		return nil, err
	}
	return anthropicBlocksToResponse(envelope.Content)
}

type anthropicContentBlock struct {
	Type      string              `json:"type"`
	Text      string              `json:"text"`
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Input     json.RawMessage     `json:"input"`
	Citations []anthropicCitation `json:"citations"`
}

type anthropicCitation struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func anthropicBlocksToResponse(blocks []anthropicContentBlock) (*Response, error) {
	var response Response
	var citations []Citation
	for _, block := range blocks {
		switch block.Type {
		case "text":
			response.Text += block.Text
			for _, c := range block.Citations {
				citations = append(citations, Citation{Title: c.Title, URL: c.URL})
			}
		case "tool_use":
			args := string(block.Input)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			response.ToolCalls = append(response.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Args: args})
		case "server_tool_use", "web_search_tool_result":
			// Server-side web search; not a client tool call. Ignore.
		}
	}
	response.Citations = dedupeCitations(citations)
	response.Text = appendSources(response.Text, response.Citations)
	return &response, nil
}

func parseAnthropicStream(r io.Reader, onText func(string)) (*Response, error) {
	var text strings.Builder
	var citations []Citation
	tools := map[int]*streamToolUse{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				Citation    struct {
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"citation"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, err
		}
		switch event.Type {
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				tools[event.Index] = &streamToolUse{id: event.ContentBlock.ID, name: event.ContentBlock.Name}
			}
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				text.WriteString(event.Delta.Text)
				onText(event.Delta.Text)
			case "input_json_delta":
				// Only accumulate input for client tool_use blocks; server-side
				// blocks (e.g. server_tool_use) have no entry and are skipped.
				if tool := tools[event.Index]; tool != nil {
					tool.args.WriteString(event.Delta.PartialJSON)
				}
			case "citations_delta":
				citations = append(citations, Citation{Title: event.Delta.Citation.Title, URL: event.Delta.Citation.URL})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	response := &Response{Text: text.String()}
	indexes := make([]int, 0, len(tools))
	for index := range tools {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		tool := tools[index]
		args := strings.TrimSpace(tool.args.String())
		if args == "" {
			args = "{}"
		}
		response.ToolCalls = append(response.ToolCalls, ToolCall{ID: tool.id, Name: tool.name, Args: args})
	}
	response.Citations = dedupeCitations(citations)
	response.Text = appendSources(response.Text, response.Citations)
	return response, nil
}

type streamToolUse struct {
	id   string
	name string
	args strings.Builder
}
