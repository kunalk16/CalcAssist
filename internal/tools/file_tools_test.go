package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustArgs(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateFileWritesAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "hello.txt")
	tool := NewCreateFileTool()

	out, err := tool.Execute(context.Background(), mustArgs(t, map[string]any{
		"path":    path,
		"content": "hello",
	}))
	if err != nil {
		t.Fatalf("create_file failed: %v", err)
	}
	if !strings.Contains(out, "Wrote 5 bytes") {
		t.Fatalf("unexpected output: %q", out)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "hello" {
		t.Fatalf("file content = %q, %v", got, err)
	}
	if _, err := tool.Execute(context.Background(), mustArgs(t, map[string]any{
		"path":    path,
		"content": "replace",
	})); err == nil || !strings.Contains(err.Error(), "overwrite=true") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestReadFileRespectsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "read.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := NewReadFileTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"path":      path,
		"max_bytes": 3,
	}))
	if err != nil {
		t.Fatalf("read_file failed: %v", err)
	}
	if !strings.Contains(out, "abc") || !strings.Contains(out, "truncated") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestListDirectoryListsEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := NewListDirectoryTool().Execute(context.Background(), mustArgs(t, map[string]any{"path": dir}))
	if err != nil {
		t.Fatalf("list_directory failed: %v", err)
	}
	if !strings.Contains(out, "subdir\t(dir)") || !strings.Contains(out, "file.txt\t(file, 3 bytes)") {
		t.Fatalf("unexpected listing: %q", out)
	}
}

func TestSearchFilesFindsByNameAndContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.log"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	byName, err := NewSearchFilesTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"root":      dir,
		"name_glob": "*.txt",
	}))
	if err != nil {
		t.Fatalf("search by name failed: %v", err)
	}
	if !strings.Contains(byName, "alpha.txt") || strings.Contains(byName, "beta.log") {
		t.Fatalf("unexpected name search output: %q", byName)
	}

	byContent, err := NewSearchFilesTool().Execute(context.Background(), mustArgs(t, map[string]any{
		"root":          dir,
		"content_regex": "needle",
	}))
	if err != nil {
		t.Fatalf("search by content failed: %v", err)
	}
	if !strings.Contains(byContent, "alpha.txt") || !strings.Contains(byContent, "needle here") {
		t.Fatalf("unexpected content search output: %q", byContent)
	}
}
