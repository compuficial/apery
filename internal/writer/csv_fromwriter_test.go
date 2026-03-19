package writer

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSVWriterFromWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewCSVWriterFromWriter(&buf)

	r1 := makeRecord("id", 1, "name", "Alice")
	r2 := makeRecord("id", 2, "name", "Bob")

	if err := w.WriteRecord("User", r1); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.WriteRecord("User", r2); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (header + 2 rows)", len(lines))
	}
	if lines[0] != "_entity,id,name" {
		t.Errorf("header = %q, want _entity,id,name", lines[0])
	}
	if lines[1] != "User,1,Alice" {
		t.Errorf("row1 = %q, want User,1,Alice", lines[1])
	}
}

func TestCSVWriterFromWriter_CloseDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	w := NewCSVWriterFromWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCSVWriterSplit_OmitsEntity(t *testing.T) {
	path := tempPath(t, "split.csv")
	w, err := NewCSVWriterSplit(path)
	if err != nil {
		t.Fatal(err)
	}

	r := makeRecord("id", 1, "name", "Alice")
	if err := w.WriteRecord("User", r); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0] != "id,name" {
		t.Errorf("header = %q, want id,name (no _entity)", lines[0])
	}
	if lines[1] != "1,Alice" {
		t.Errorf("row = %q, want 1,Alice", lines[1])
	}
}
