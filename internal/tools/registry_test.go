package tools

import "testing"

func TestDefaultRegistry(t *testing.T) {
	r := Default()
	if r.Len() != 9 {
		t.Fatalf("Default registry length = %d, want 9", r.Len())
	}
	expected := []struct {
		name     string
		mutating bool
	}{
		{"create_file", true},
		{"list_directory", false},
		{"read_file", false},
		{"search_files", false},
		{"calculate", false},
		{"statistics", false},
		{"excel_to_json", false},
		{"read_pdf", false},
		{"read_docx", false},
	}
	for i, want := range expected {
		got := r.All()[i]
		if got.Name() != want.name || got.Mutating() != want.mutating {
			t.Fatalf("tool %d = (%s, %v), want (%s, %v)", i, got.Name(), got.Mutating(), want.name, want.mutating)
		}
		if _, ok := r.Get(want.name); !ok {
			t.Fatalf("Get(%q) failed", want.name)
		}
	}
}
