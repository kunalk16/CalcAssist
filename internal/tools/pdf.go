package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

type readPDFTool struct{}

type readPDFArgs struct {
	Path    string `json:"path"`
	MaxChar int    `json:"max_chars"`
}

// NewReadPDFTool returns a read-only PDF text extraction tool.
func NewReadPDFTool() Tool { return readPDFTool{} }

func (readPDFTool) Name() string { return "read_pdf" }

func (readPDFTool) Description() string {
	return "Extract plain text from a PDF. OCR for scanned PDFs is unsupported."
}

func (readPDFTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "PDF file path."},
			"max_chars": map[string]any{"type": "integer", "description": "Maximum characters to return. Defaults to 200000."},
		},
		"required": []string{"path"},
	}
}

func (readPDFTool) Mutating() bool { return false }

func (readPDFTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed readPDFArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid read_pdf arguments: %w", err)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	maxChars := parsed.MaxChar
	if maxChars <= 0 {
		maxChars = 200000
	}
	f, r, err := pdf.Open(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("open PDF %q: %w", parsed.Path, err)
	}
	defer f.Close()
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract PDF text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return "", fmt.Errorf("read PDF text: %w", err)
	}
	text := strings.TrimSpace(buf.String())
	if text == "" {
		return "No text could be extracted. The PDF may be scanned or encrypted; OCR is unsupported.", nil
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		return string(runes[:maxChars]) + fmt.Sprintf("\n\n[truncated after %d characters]", maxChars), nil
	}
	return text, nil
}
