package writer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

// referenceJSONL is the byte format the original encoder produced: each key
// and value through encoding/json, joined in insertion order.
func referenceJSONL(t *testing.T, entity string, keys []string, values []any) string {
	t.Helper()
	parts := []string{`"_entity":` + mustMarshal(t, entity)}
	for i, k := range keys {
		parts = append(parts, mustMarshal(t, k)+":"+mustMarshal(t, values[i]))
	}
	return "{" + strings.Join(parts, ",") + "}\n"
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %#v: %v", v, err)
	}
	return string(b)
}

// The fast encoder must stay byte-identical to encoding/json, since golden
// files and users' fixtures depend on exact output.
func TestJSONLWriter_MatchesEncodingJSON(t *testing.T) {
	values := []any{
		"plain", "", `quote " and \ slash`, "<tag> & amp", "tab\tnew\nline\x01", "héllo ✓", "  ", "bad\xffutf8",
		int64(0), int64(-42), int64(math.MaxInt64), int64(math.MinInt64), 7, uint64(math.MaxUint64),
		0.0, math.Copysign(0, -1), 1.5, -2.25, 1e-7, 1e-6, 123456789.0, 1e20, 1e21, 1.7976931348623157e308, 5e-324,
		true, false, nil,
		map[string]any{"b": 1, "a": "<x>"}, []any{int64(1), "two", 3.5, nil},
		json.Number("12"),
	}
	keys := make([]string, len(values))
	for i := range values {
		keys[i] = "k" + string(rune('a'+i%26)) + strings.Repeat("_", i/26)
	}
	keys[0] = "<html key>"

	record := NewOrderedMap()
	for i, k := range keys {
		record.Set(k, values[i])
	}

	var buf bytes.Buffer
	w := NewJSONLWriterFromWriter(&buf)
	if err := w.WriteRecord("E&<>", record); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	want := referenceJSONL(t, "E&<>", keys, values)
	if buf.String() != want {
		t.Fatalf("output differs from encoding/json\n got: %s\nwant: %s", buf.String(), want)
	}
}

// Non-finite floats can't be JSON; the error must name the field.
func TestJSONLWriter_NonFiniteFloatNamesField(t *testing.T) {
	w := NewJSONLWriterFromWriter(io.Discard)
	err := w.WriteRecord("E", makeRecord("score", math.Inf(1)))
	if err == nil || !strings.Contains(err.Error(), `"score"`) || !strings.Contains(err.Error(), "+Inf") {
		t.Fatalf("error = %v, want field name and value", err)
	}
}

func TestOrderedMapMarshalJSON_PropagatesErrors(t *testing.T) {
	if _, err := json.Marshal(makeRecord("x", math.NaN())); err == nil {
		t.Fatal("expected error for NaN")
	}
	b, err := json.Marshal(makeRecord("b", 1, "a", "<"))
	if want := `{"b":1,"a":` + mustMarshal(t, "<") + `}`; err != nil || string(b) != want {
		t.Fatalf("MarshalJSON = %s, %v", b, err)
	}
}

// Records wider than the linear-scan threshold must still behave as a map.
func TestOrderedMap_WideRecord(t *testing.T) {
	record := NewOrderedMapSize(4)
	for i := range 100 {
		record.Set(string(rune('A'+i%26))+strings.Repeat("x", i/26), i)
	}
	record.Set("A", -1) // update, not append
	if len(record.Keys()) != 100 {
		t.Fatalf("keys = %d, want 100", len(record.Keys()))
	}
	if v, _ := record.Get("A"); v != -1 {
		t.Fatalf("A = %v, want -1", v)
	}
	if v, _ := record.Get("Zxx"); v != 77 {
		t.Fatalf("Zxx = %v, want 77", v)
	}
	if _, ok := record.Get("missing"); ok {
		t.Fatal("unexpected key")
	}
}

// One CSV stream has one header, so a second entity can't be written to it.
func TestCSVWriter_RejectsSecondEntity(t *testing.T) {
	w := NewCSVWriterFromWriter(io.Discard)
	if err := w.WriteRecord("User", makeRecord("id", 1)); err != nil {
		t.Fatal(err)
	}
	err := w.WriteRecord("Order", makeRecord("order_id", 1))
	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("error = %v, want a pointer to split output", err)
	}
}

// Nested values are JSON, and floats never switch to exponent notation.
func TestCSVWriter_EncodesValuesPortably(t *testing.T) {
	var buf bytes.Buffer
	w := NewCSVWriterFromWriter(&buf)
	record := makeRecord("o", map[string]any{"b": "x", "a": 1}, "l", []any{1, 2}, "f", 1432716.4583540987, "small", 0.00001, "n", nil)
	if err := w.WriteRecord("U", record); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := "_entity,o,l,f,small,n\nU,\"{\"\"a\"\":1,\"\"b\"\":\"\"x\"\"}\",\"[1,2]\",1432716.4583540987,0.00001,\n"
	if buf.String() != want {
		t.Fatalf("csv:\n got %q\nwant %q", buf.String(), want)
	}
}

type failingWriter struct{ err error }

func (f failingWriter) WriteRecord(string, *OrderedMap) error { return nil }
func (f failingWriter) Close() error                          { return f.err }

func TestSplitWriter_CloseReportsEveryError(t *testing.T) {
	errA, errB := errors.New("close a"), errors.New("close b")
	sw := NewSplitWriter(t.TempDir(), "jsonl")
	sw.writers["A"] = failingWriter{errA}
	sw.writers["B"] = failingWriter{errB}

	err := sw.Close()
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Close = %v, want both errors", err)
	}
}
