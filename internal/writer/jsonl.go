package writer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

type JSONLWriter struct {
	file       *os.File
	buf        *bufio.Writer
	omitEntity bool
	line       []byte // encode buffer reused across records
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
	if w.omitEntity {
		entity = ""
	}

	line, err := appendObject(w.line[:0], record, entity)
	if err != nil {
		return fmt.Errorf("jsonl: entity %q: %w", entity, err)
	}
	w.line = append(line, '\n')

	_, err = w.buf.Write(w.line)
	return err
}

// Close flushes buffered data and closes the file (if one was opened). The
// file is closed even when the flush fails.
func (w *JSONLWriter) Close() error {
	err := w.buf.Flush()
	if w.file != nil {
		err = errors.Join(err, w.file.Close())
	}
	return err
}
