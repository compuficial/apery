package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitWriter_CreatesPerEntityFiles(t *testing.T) {
	dir := t.TempDir()
	w := NewSplitWriter(dir, "jsonl")

	r1 := makeRecord("id", 1, "name", "Alice")
	r2 := makeRecord("id", 2, "name", "Bob")
	r3 := makeRecord("pid", 10, "title", "Widget")

	if err := w.WriteRecord("User", r1); err != nil {
		t.Fatalf("WriteRecord User: %v", err)
	}
	if err := w.WriteRecord("User", r2); err != nil {
		t.Fatalf("WriteRecord User: %v", err)
	}
	if err := w.WriteRecord("Product", r3); err != nil {
		t.Fatalf("WriteRecord Product: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	userLines := readLines(t, filepath.Join(dir, "User.jsonl"))
	if len(userLines) != 2 {
		t.Errorf("User.jsonl has %d lines, want 2", len(userLines))
	}

	productLines := readLines(t, filepath.Join(dir, "Product.jsonl"))
	if len(productLines) != 1 {
		t.Errorf("Product.jsonl has %d lines, want 1", len(productLines))
	}
}

func TestSplitWriter_CSV(t *testing.T) {
	dir := t.TempDir()
	w := NewSplitWriter(dir, "csv")

	r1 := makeRecord("id", 1, "name", "Alice")
	if err := w.WriteRecord("User", r1); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "User.csv"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (header + 1 row)", len(lines))
	}
	if lines[0] != "id,name" {
		t.Errorf("header = %q, want id,name", lines[0])
	}
}

func TestSplitWriter_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	w := NewSplitWriter(dir, "parquet")

	r := makeRecord("id", 1)
	err := w.WriteRecord("X", r)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestSplitWriter_JSONL_OmitsEntity(t *testing.T) {
	dir := t.TempDir()
	w := NewSplitWriter(dir, "jsonl")

	r := makeRecord("id", 1)
	if err := w.WriteRecord("User", r); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readLines(t, filepath.Join(dir, "User.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	// Should NOT contain _entity
	if strings.Contains(lines[0], "_entity") {
		t.Errorf("split JSONL should omit _entity, got: %s", lines[0])
	}
}
