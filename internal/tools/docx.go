package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type readDocxTool struct{}

type readDocxArgs struct {
	Path string `json:"path"`
}

// NewReadDocxTool returns a read-only DOCX text extraction tool.
func NewReadDocxTool() Tool { return readDocxTool{} }

func (readDocxTool) Name() string { return "read_docx" }

func (readDocxTool) Description() string {
	return "Extract text from a .docx file using the standard DOCX document XML."
}

func (readDocxTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "DOCX file path."},
		},
		"required": []string{"path"},
	}
}

func (readDocxTool) Mutating() bool { return false }

func (readDocxTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed readDocxArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid read_docx arguments: %w", err)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	zr, err := zip.OpenReader(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("open DOCX %q: %w", parsed.Path, err)
	}
	defer zr.Close()
	for _, file := range zr.File {
		if file.Name == "word/document.xml" {
			return extractDocxText(ctx, file)
		}
	}
	return "", fmt.Errorf("%q is not a supported .docx file: word/document.xml was not found", parsed.Path)
}

func extractDocxText(ctx context.Context, file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open word/document.xml: %w", err)
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	var b strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse word/document.xml: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				var s string
				if err := decoder.DecodeElement(&s, &t); err != nil {
					return "", fmt.Errorf("read text element: %w", err)
				}
				b.WriteString(s)
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				b.WriteByte('\n')
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
