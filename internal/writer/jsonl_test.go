package writer

import (
	"bytes"
	"testing"
)

func TestJSONLWriter(t *testing.T) {
	path := tempPath(t, "out.jsonl")

	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter: %v", err)
	}

	record := makeRecord("id", int64(1), "name", "alice")

	if err := w.WriteRecord("User", record); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if _, ok := record.Get("_entity"); ok {
		t.Fatal("expected record to be unchanged after WriteRecord")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	want := `{"_entity":"User","id":1,"name":"alice"}`
	if lines[0] != want {
		t.Fatalf("jsonl mismatch: got %q want %q", lines[0], want)
	}
}

func TestJSONLWriterFromWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSONLWriterFromWriter(&buf)

	record := makeRecord("id", int64(1), "name", "alice")
	if err := w.WriteRecord("User", record); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	want := `{"_entity":"User","id":1,"name":"alice"}` + "\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}
