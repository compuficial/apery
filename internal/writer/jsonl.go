package writer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type JSONLWriter struct {
	file *os.File
	buf  *bufio.Writer
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

// WriteRecord writes a single JSONL record with an _entity field.
func (w *JSONLWriter) WriteRecord(entity string, record *OrderedMap) error {
	out := record.Clone()
	out.Prepend("_entity", entity)

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

// Close flushes buffered data and closes the file.
func (w *JSONLWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}

	return w.file.Close()
}
