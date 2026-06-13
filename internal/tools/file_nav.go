package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type listDirectoryTool struct{}
type readFileTool struct{}
type searchFilesTool struct{}

type listDirectoryArgs struct {
	Path string `json:"path"`
}

type readFileArgs struct {
	Path    string `json:"path"`
	MaxByte int    `json:"max_bytes"`
}

type searchFilesArgs struct {
	Root         string `json:"root"`
	NameGlob     string `json:"name_glob"`
	ContentRegex string `json:"content_regex"`
	MaxResults   int    `json:"max_results"`
}

// NewListDirectoryTool returns a read-only directory listing tool.
func NewListDirectoryTool() Tool { return listDirectoryTool{} }

func (listDirectoryTool) Name() string { return "list_directory" }

func (listDirectoryTool) Description() string {
	return "List a directory, sorted with directories before files."
}

func (listDirectoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory to list. Defaults to current directory."},
		},
	}
}

func (listDirectoryTool) Mutating() bool { return false }

func (listDirectoryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed listDirectoryArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &parsed); err != nil {
			return "", fmt.Errorf("invalid list_directory arguments: %w", err)
		}
	}
	path := parsed.Path
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list %q: %w", path, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var b strings.Builder
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if entry.IsDir() {
			fmt.Fprintf(&b, "%s\t(dir)\n", entry.Name())
			continue
		}
		info, err := entry.Info()
		if err != nil {
			fmt.Fprintf(&b, "%s\t(file, size unavailable: %v)\n", entry.Name(), err)
			continue
		}
		fmt.Fprintf(&b, "%s\t(file, %d bytes)\n", entry.Name(), info.Size())
	}
	if b.Len() == 0 {
		return fmt.Sprintf("%s is empty", path), nil
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// NewReadFileTool returns a read-only text file reader.
func NewReadFileTool() Tool { return readFileTool{} }

func (readFileTool) Name() string { return "read_file" }

func (readFileTool) Description() string {
	return "Read a text file up to a maximum number of bytes."
}

func (readFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Path to the file to read."},
			"max_bytes": map[string]any{"type": "integer", "description": "Maximum bytes to read. Defaults to 100000."},
		},
		"required": []string{"path"},
	}
}

func (readFileTool) Mutating() bool { return false }

func (readFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	var parsed readFileArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	maxBytes := parsed.MaxByte
	if maxBytes <= 0 {
		maxBytes = 100000
	}
	f, err := os.Open(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", parsed.Path, err)
	}
	defer f.Close()

	buf := make([]byte, maxBytes+1)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", fmt.Errorf("read %q: %w", parsed.Path, err)
	}
	out := string(buf[:min(n, maxBytes)])
	if n > maxBytes {
		out += fmt.Sprintf("\n\n[truncated after %d bytes]", maxBytes)
	}
	return out, nil
}

// NewSearchFilesTool returns a tool that searches file names and/or contents.
func NewSearchFilesTool() Tool { return searchFilesTool{} }

func (searchFilesTool) Name() string { return "search_files" }

func (searchFilesTool) Description() string {
	return "Search files by base-name glob and/or content regular expression."
}

func (searchFilesTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root":          map[string]any{"type": "string", "description": "Root directory to search. Defaults to current directory."},
			"name_glob":     map[string]any{"type": "string", "description": "filepath.Match glob matched against each base file name."},
			"content_regex": map[string]any{"type": "string", "description": "Go regular expression matched against file contents."},
			"max_results":   map[string]any{"type": "integer", "description": "Maximum matches to return. Defaults to 100."},
		},
	}
}

func (searchFilesTool) Mutating() bool { return false }

func (searchFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed searchFilesArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &parsed); err != nil {
			return "", fmt.Errorf("invalid search_files arguments: %w", err)
		}
	}
	root := parsed.Root
	if root == "" {
		root = "."
	}
	maxResults := parsed.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	var re *regexp.Regexp
	if parsed.ContentRegex != "" {
		compiled, err := regexp.Compile(parsed.ContentRegex)
		if err != nil {
			return "", fmt.Errorf("invalid content_regex: %w", err)
		}
		re = compiled
	}

	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxResults {
			return fs.SkipAll
		}
		if parsed.NameGlob != "" {
			ok, err := filepath.Match(parsed.NameGlob, filepath.Base(path))
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		if re == nil {
			matches = append(matches, path)
			return nil
		}
		line, ok := firstContentMatch(path, re)
		if ok {
			matches = append(matches, fmt.Sprintf("%s: %s", path, line))
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search %q: %w", root, err)
	}
	if len(matches) == 0 {
		return "No matches found", nil
	}
	return strings.Join(matches, "\n"), nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".hg", ".svn":
		return true
	default:
		return false
	}
}

func firstContentMatch(path string, re *regexp.Regexp) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1<<20 {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil || likelyBinary(data) {
		return "", false
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}

func likelyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sample := data
	if len(sample) > 8000 {
		sample = sample[:8000]
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return false
}
