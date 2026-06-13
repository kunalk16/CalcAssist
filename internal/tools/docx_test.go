package tools

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDocx(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>First paragraph</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second paragraph</w:t></w:r></w:p>
  </w:body>
</w:document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := NewReadDocxTool().Execute(context.Background(), mustArgs(t, map[string]any{"path": path}))
	if err != nil {
		t.Fatalf("read_docx failed: %v", err)
	}
	if !strings.Contains(out, "First paragraph") || !strings.Contains(out, "Second paragraph") {
		t.Fatalf("unexpected docx text: %q", out)
	}
}
