package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelToJSON(t *testing.T) {
	path := writeTestWorkbook(t)
	out, err := NewExcelToJSONTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"path": path,
	}))
	if err != nil {
		t.Fatalf("excel_to_json failed: %v", err)
	}
	var rows []map[string]string
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("parse excel JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 || rows[0]["Name"] != "Alice" || rows[0]["Score"] != "10" || rows[1]["Name"] != "Bob" || rows[1]["Score"] != "20" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestStatisticsExcelSource(t *testing.T) {
	path := writeTestWorkbook(t)
	out, err := NewStatisticsTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"source": map[string]any{
			"type":   "excel",
			"path":   path,
			"column": "Score",
		},
		"operations": []string{"count", "sum", "mean"},
	}))
	if err != nil {
		t.Fatalf("statistics excel source failed: %v", err)
	}
	if !strings.Contains(out, `"count": 2`) || !strings.Contains(out, `"sum": 30`) || !strings.Contains(out, `"mean": 15`) {
		t.Fatalf("unexpected statistics output: %q", out)
	}
}

func writeTestWorkbook(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellValue("Sheet1", "A1", "Name"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "Score"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "A2", "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B2", 10); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "A3", "Bob"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B3", 20); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}
