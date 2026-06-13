package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type excelToJSONTool struct{}

type excelToJSONArgs struct {
	Path      string `json:"path"`
	Sheet     string `json:"sheet"`
	HeaderRow int    `json:"header_row"`
}

// NewExcelToJSONTool returns a read-only Excel to JSON converter.
func NewExcelToJSONTool() Tool { return excelToJSONTool{} }

func (excelToJSONTool) Name() string { return "excel_to_json" }

func (excelToJSONTool) Description() string {
	return "Convert an Excel worksheet to a JSON array of objects using a header row."
}

func (excelToJSONTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "Excel workbook path."},
			"sheet":      map[string]any{"type": "string", "description": "Worksheet name. Defaults to the first sheet."},
			"header_row": map[string]any{"type": "integer", "description": "1-based header row number. Defaults to 1."},
		},
		"required": []string{"path"},
	}
}

func (excelToJSONTool) Mutating() bool { return false }

func (excelToJSONTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed excelToJSONArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid excel_to_json arguments: %w", err)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	headerRow := parsed.HeaderRow
	if headerRow <= 0 {
		headerRow = 1
	}

	f, err := excelize.OpenFile(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("open excel file %q: %w", parsed.Path, err)
	}
	defer f.Close()
	sheet := parsed.Sheet
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return "[]", nil
		}
		sheet = sheets[0]
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", fmt.Errorf("read sheet %q: %w", sheet, err)
	}
	if len(rows) < headerRow {
		return "[]", nil
	}

	headers := rows[headerRow-1]
	for i, h := range headers {
		headers[i] = strings.TrimSpace(h)
		if headers[i] == "" {
			headers[i] = fmt.Sprintf("column_%d", i+1)
		}
	}
	var objects []map[string]string
	for _, row := range rows[headerRow:] {
		obj := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(row) {
				obj[h] = row[i]
			} else {
				obj[h] = ""
			}
		}
		objects = append(objects, obj)
	}
	out, err := json.MarshalIndent(objects, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
