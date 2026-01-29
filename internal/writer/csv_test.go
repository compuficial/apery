package writer

import "testing"

func TestCSVWriter(t *testing.T) {
	path := tempPath(t, "out.csv")

	w, err := NewCSVWriter(path)
	if err != nil {
		t.Fatalf("NewCSVWriter: %v", err)
	}

	if err := w.WriteRecord("User", makeRecord("id", int64(1), "name", "alice")); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	if err := w.WriteRecord("User", makeRecord("id", int64(2), "name", "bob")); err != nil {
		t.Fatalf("WriteRecord(2): %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if lines[0] != "_entity,id,name" {
		t.Fatalf("header mismatch: %q", lines[0])
	}
	if lines[1] != "User,1,alice" {
		t.Fatalf("row1 mismatch: %q", lines[1])
	}
	if lines[2] != "User,2,bob" {
		t.Fatalf("row2 mismatch: %q", lines[2])
	}
}

func TestCSVWriterNilRecord(t *testing.T) {
	path := tempPath(t, "out.csv")

	w, err := NewCSVWriter(path)
	if err != nil {
		t.Fatalf("NewCSVWriter: %v", err)
	}

	if err := w.WriteRecord("User", nil); err == nil {
		t.Fatal("expected error for nil record")
	}
}
