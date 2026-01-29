package writer

import (
	"encoding/csv"
	"fmt"
	"os"
)

type CSVWriter struct {
	file  *os.File
	w     *csv.Writer
	keys  []string
	initd bool
}

func NewCSVWriter(path string) (*CSVWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("csv: create %s: %w", path, err)
	}
	return &CSVWriter{file: f, w: csv.NewWriter(f)}, nil
}

func (w *CSVWriter) WriteRecord(entity string, record *OrderedMap) error {
	if record == nil {
		return fmt.Errorf("csv: record is nil")
	}

	if !w.initd {
		keys := make([]string, 0, len(record.keys))
		for _, key := range record.Keys() {
			if key == "_entity" {
				continue
			}
			keys = append(keys, key)
		}
		w.keys = keys
		header := append([]string{"_entity"}, w.keys...)
		if err := w.w.Write(header); err != nil {
			return fmt.Errorf("csv: header: %w", err)
		}
		w.initd = true
	}

	row := make([]string, 0, len(w.keys)+1)
	row = append(row, entity)
	for _, key := range w.keys {
		val, ok := record.Get(key)
		if !ok || val == nil {
			row = append(row, "")
			continue
		}
		row = append(row, fmt.Sprint(val))
	}
	if err := w.w.Write(row); err != nil {
		return fmt.Errorf("csv: write: %w", err)
	}
	return nil
}

func (w *CSVWriter) Close() error {
	w.w.Flush()
	if err := w.w.Error(); err != nil {
		return fmt.Errorf("csv: flush: %w", err)
	}
	return w.file.Close()
}
