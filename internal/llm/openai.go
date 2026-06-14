package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type openAIProvider struct {
	cfg     Config
	client  *http.Client
	baseURL string
	isAzure bool
}

// NewOpenAI returns an OpenAI Responses API provider.
func NewOpenAI(cfg Config) Provider {
	return newOpenAIProvider(cfg, false, nil)
}

// NewAzure returns an Azure OpenAI Responses API provider.
func NewAzure(cfg Config) Provider {
	return newOpenAIProvider(cfg, true, nil)
}

func newOpenAIProvider(cfg Config, isAzure bool, client *http.Client) *openAIProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" && !isAzure {
		baseURL = defaultOpenAIBaseURL
	}
	if client == nil {
		client = defaultHTTPClient()
	}
	return &openAIProvider{cfg: cfg, client: client, baseURL: baseURL, isAzure: isAzure}
}

func (p *openAIProvider) Name() string {
	if p.isAzure {
		return "azure"
	}
	return "openai"
}

func (p *openAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef, onText func(string)) (*Response, error) {
	body := p.buildRequest(messages, tools, onText != nil)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.isAzure {
		req.Header.Set("api-key", p.cfg.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpStatusError(resp)
	}
	if onText != nil {
		return parseOpenAIStream(resp.Body, onText)
	}
	return parseOpenAIResponse(resp.Body)
}

func (p *openAIProvider) buildRequest(messages []Message, tools []ToolDef, stream bool) map[string]any {
	req := map[string]any{
		"model":  p.cfg.Model,
		"input":  openAIInput(messages),
		"stream": stream,
	}
	if instructions := systemInstructions(messages); instructions != "" {
		req["instructions"] = instructions
	}
	if reqTools := p.openAIRequestTools(tools); len(reqTools) > 0 {
		req["tools"] = reqTools
	}
	if p.cfg.MaxTokens > 0 {
		req["max_output_tokens"] = p.cfg.MaxTokens
	}
	if p.cfg.Temperature >= 0 {
		req["temperature"] = p.cfg.Temperature
	}
	return req
}

// openAIRequestTools maps the local tools and, when web search is enabled, appends
// the hosted web_search tool (last) so the model can search the web.
func (p *openAIProvider) openAIRequestTools(tools []ToolDef) []map[string]any {
	reqTools := openAITools(tools)
	if p.cfg.WebSearch {
		reqTools = append(reqTools, map[string]any{"type": "web_search"})
	}
	return reqTools
}

func openAIInput(messages []Message) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			continue
		case RoleUser:
			input = append(input, map[string]any{"role": "user", "content": msg.Content})
		case RoleAssistant:
			if msg.Content != "" {
				input = append(input, map[string]any{"role": "assistant", "content": msg.Content})
			}
			for _, call := range msg.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Name,
					"arguments": call.Args,
				})
			}
		case RoleTool:
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
		}
	}
	return input
}

func openAITools(tools []ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Schema,
		})
	}
	return out
}

func parseOpenAIResponse(r io.Reader) (*Response, error) {
	var envelope struct {
		Output []json.RawMessage `json:"output"`
	}
	if err := json.NewDecoder(r).Decode(&envelope); err != nil {
		return nil, err
	}
	return parseOpenAIOutput(envelope.Output)
}

func parseOpenAIOutput(output []json.RawMessage) (*Response, error) {
	var response Response
	var citations []Citation
	for _, raw := range output {
		var item struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments string          `json:"arguments"`
			Content   []openAIContent `json:"content"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					response.Text += content.Text
					for _, a := range content.Annotations {
						if a.Type == "url_citation" {
							citations = append(citations, Citation{Title: a.Title, URL: a.URL})
						}
					}
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			response.ToolCalls = append(response.ToolCalls, ToolCall{ID: id, Name: item.Name, Args: item.Arguments})
		case "web_search_call":
			// Hosted web-search invocation; not a client tool call. Ignore.
		}
	}
	response.Citations = dedupeCitations(citations)
	response.Text = appendSources(response.Text, response.Citations)
	return &response, nil
}

type openAIContent struct {
	Type        string             `json:"type"`
	Text        string             `json:"text"`
	Annotations []openAIAnnotation `json:"annotations"`
}

type openAIAnnotation struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func parseOpenAIStream(r io.Reader, onText func(string)) (*Response, error) {
	var accumulated strings.Builder
	var final *Response
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response struct {
				Output []json.RawMessage `json:"output"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, err
		}
		switch event.Type {
		case "response.output_text.delta":
			accumulated.WriteString(event.Delta)
			onText(event.Delta)
		case "response.completed":
			parsed, err := parseOpenAIOutput(event.Response.Output)
			if err != nil {
				return nil, err
			}
			final = parsed
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if final == nil {
		return &Response{Text: accumulated.String()}, nil
	}
	if final.Text == "" {
		final.Text = accumulated.String()
	}
	return final, nil
}

func systemInstructions(messages []Message) string {
	parts := make([]string, 0)
	for _, msg := range messages {
		if msg.Role == RoleSystem && msg.Content != "" {
			parts = append(parts, msg.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func httpStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("llm request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
}
