package writer

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compareChunks writes data to a Comparer over a file holding file, size
// bytes at a time, and returns the first error from Write or Close.
func compareChunks(t *testing.T, file, data string, size int) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := OpenComparer(path)
	if err != nil {
		t.Fatal(err)
	}
	for len(data) > 0 {
		n := min(size, len(data))
		if _, err := c.Write([]byte(data[:n])); err != nil {
			_ = c.Close() // Write's mismatch is the answer
			return err
		}
		data = data[n:]
	}
	return c.Close()
}

func TestComparer(t *testing.T) {
	const want = "a1\nb22\nc333\n"
	cases := []struct {
		name, file string
		mismatch   *Mismatch // nil means the output matches
	}{
		{"equal", want, nil},
		{"content", "a1\nb22\nc3x3\n", &Mismatch{Line: 3, Reason: ReasonContent, Want: "c333", Got: "c3x3"}},
		{"first byte", "x1\nb22\nc333\n", &Mismatch{Line: 1, Reason: ReasonContent, Want: "a1", Got: "x1"}},
		{"file short", "a1\nb2", &Mismatch{Line: 2, Reason: ReasonFileShort, Want: "b22", Got: "b2"}},
		{"file long", want + "d4444\n", &Mismatch{Line: 4, Reason: ReasonFileLong, Got: "d4444"}},
		{"empty file", "", &Mismatch{Line: 1, Reason: ReasonFileShort, Want: "a1"}},
	}
	for _, c := range cases {
		// Every chunking must give the same answer.
		for _, size := range []int{1, 2, 3, 5, len(want)} {
			err := compareChunks(t, c.file, want, size)
			if c.mismatch == nil {
				if err != nil {
					t.Errorf("%s, chunk %d: %v", c.name, size, err)
				}
				continue
			}

			m, ok := errors.AsType[*Mismatch](err)
			if !ok || !errors.Is(err, ErrMismatch) {
				t.Fatalf("%s, chunk %d: error %v is not a *Mismatch", c.name, size, err)
			}
			got := *m
			got.File = ""
			// A want line cut at a chunk boundary is marked; drop the mark.
			got.Want = strings.TrimSuffix(got.Want, "...")
			if !strings.HasPrefix(c.mismatch.Want, got.Want) || got.Line != c.mismatch.Line || got.Reason != c.mismatch.Reason || got.Got != c.mismatch.Got {
				t.Errorf("%s, chunk %d: %+v, want %+v", c.name, size, got, *c.mismatch)
			}
		}
	}
}

func TestComparer_MissingFile(t *testing.T) {
	_, err := OpenComparer(filepath.Join(t.TempDir(), "nope.jsonl"))
	if m, ok := errors.AsType[*Mismatch](err); !ok || m.Reason != ReasonMissing {
		t.Fatalf("error = %v, want a missing-file mismatch", err)
	}
}

func TestComparer_LongLinesAreCut(t *testing.T) {
	line := strings.Repeat("x", 500)
	err := compareChunks(t, line+"y\n", line+"z\n", 4096)
	m, ok := errors.AsType[*Mismatch](err)
	if !ok || len(m.Got) != maxShownLine+3 || !strings.HasSuffix(m.Got, "...") {
		t.Fatalf("mismatch = %+v, want lines cut to %d bytes plus ...", m, maxShownLine)
	}
}

// A SplitWriter over comparers checks per-entity files written by a normal
// SplitWriter, record for record.
func TestSplitWriter_Compare(t *testing.T) {
	dir := t.TempDir()
	record := func(v any) *OrderedMap {
		m := NewOrderedMap()
		m.Set("v", v)
		return m
	}
	write := func(w Writer, last any) error {
		for _, rec := range []struct {
			entity string
			v      any
		}{{"A", 1}, {"B", "x"}, {"A", last}} {
			if err := w.WriteRecord(rec.entity, record(rec.v)); err != nil {
				return err
			}
		}
		return w.Close()
	}
	open := func(path string) (io.WriteCloser, error) {
		c, err := OpenComparer(path)
		if err != nil {
			return nil, err
		}
		return c, nil
	}

	for _, format := range []string{"jsonl", "csv"} {
		if err := write(NewSplitWriter(dir, format), 2); err != nil {
			t.Fatal(err)
		}
		if err := write(NewSplitWriterFunc(dir, format, open), 2); err != nil {
			t.Errorf("%s: same records: %v", format, err)
		}
		err := write(NewSplitWriterFunc(dir, format, open), 3)
		if m, ok := errors.AsType[*Mismatch](err); !ok || filepath.Base(m.File) != "A."+format {
			t.Errorf("%s: changed record: error %v, want a mismatch in A.%s", format, err, format)
		}
	}
}
