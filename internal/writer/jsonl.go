package writer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type JSONLWriter struct {
	file       *os.File
	buf        *bufio.Writer
	omitEntity bool
}

// NewJSONLWriter creates a JSONL writer at the given path.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	return &JSONLWriter{
		file: f,
		buf:  bufio.NewWriter(f),
	}, nil
}

// NewJSONLWriterFromWriter creates a JSONL writer that writes to any io.Writer.
func NewJSONLWriterFromWriter(w io.Writer) *JSONLWriter {
	return &JSONLWriter{
		buf: bufio.NewWriter(w),
	}
}

// NewJSONLWriterSplit creates a JSONL writer at the given path that omits the _entity field.
func NewJSONLWriterSplit(path string) (*JSONLWriter, error) {
	w, err := NewJSONLWriter(path)
	if err != nil {
		return nil, err
	}
	w.omitEntity = true
	return w, nil
}

// WriteRecord writes a single JSONL record, prepending _entity unless omitEntity is set.
func (w *JSONLWriter) WriteRecord(entity string, record *OrderedMap) error {
	out := record.Clone()
	if !w.omitEntity {
		out.Prepend("_entity", entity)
	}

	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	if _, err := w.buf.Write(data); err != nil {
		return err
	}

	if err := w.buf.WriteByte('\n'); err != nil {
		return err
	}

	return nil
}

// Close flushes buffered data and closes the file (if one was opened).
func (w *JSONLWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
