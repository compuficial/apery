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

func (w *JSONLWriter) WriteRecord(entity string, record *OrderedMap) error {
	record.Prepend("_entity", entity)

	data, err := json.Marshal(record)
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

func (w *JSONLWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}

	return w.file.Close()
}
