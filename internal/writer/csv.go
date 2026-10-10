package writer

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type CSVWriter struct {
	file       *os.File
	w          *csv.Writer
	keys       []string
	entity     string // the stream's only entity, set by the first record
	initd      bool
	omitEntity bool
	row        []string // reused across records
}

// NewCSVWriter creates a CSV writer at the given path.
func NewCSVWriter(path string) (*CSVWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("csv: create %s: %w", path, err)
	}
	return &CSVWriter{file: f, w: csv.NewWriter(f)}, nil
}

// NewCSVWriterFromWriter creates a CSV writer that writes to any io.Writer.
func NewCSVWriterFromWriter(w io.Writer) *CSVWriter {
	return &CSVWriter{w: csv.NewWriter(w)}
}

// NewCSVWriterSplit creates a CSV writer at the given path that omits the _entity column.
func NewCSVWriterSplit(path string) (*CSVWriter, error) {
	w, err := NewCSVWriter(path)
	if err != nil {
		return nil, err
	}
	w.omitEntity = true
	return w, nil
}

// WriteRecord writes a CSV row and emits a header on the first record. A CSV
// stream has a single header, so every record must belong to the first
// record's entity.
func (w *CSVWriter) WriteRecord(entity string, record *OrderedMap) error {
	if record == nil {
		return fmt.Errorf("csv: record is nil")
	}

	if !w.initd {
		if err := w.writeHeader(entity, record); err != nil {
			return err
		}
	} else if entity != w.entity {
		return fmt.Errorf("csv: cannot write entity %q after %q: one CSV stream holds one entity; write one file per entity (--split-entities)", entity, w.entity)
	}

	w.row = w.row[:0]
	if !w.omitEntity {
		w.row = append(w.row, entity)
	}
	for _, key := range w.keys {
		val, _ := record.Get(key)
		cell, err := csvCell(val)
		if err != nil {
			return fmt.Errorf("csv: field %q: %w", key, err)
		}
		w.row = append(w.row, cell)
	}
	if err := w.w.Write(w.row); err != nil {
		return fmt.Errorf("csv: write: %w", err)
	}
	return nil
}

func (w *CSVWriter) writeHeader(entity string, record *OrderedMap) error {
	w.keys = make([]string, 0, len(record.keys))
	for _, key := range record.keys {
		if key != "_entity" {
			w.keys = append(w.keys, key)
		}
	}

	header := w.keys
	if !w.omitEntity {
		header = append([]string{"_entity"}, w.keys...)
	}
	if err := w.w.Write(header); err != nil {
		return fmt.Errorf("csv: header: %w", err)
	}
	w.entity = entity
	w.initd = true
	return nil
}

// csvCell formats one value. Floats use plain decimal notation and nested
// maps and lists are JSON, so cells parse the same way everywhere.
func csvCell(val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case map[string]any, []any:
		b, err := json.Marshal(v)
		return string(b), err
	default:
		return fmt.Sprint(v), nil
	}
}

// Close flushes buffered data and closes the file (if one was opened). The
// file is closed even when the flush fails.
func (w *CSVWriter) Close() error {
	w.w.Flush()
	var err error
	if flushErr := w.w.Error(); flushErr != nil {
		err = fmt.Errorf("csv: flush: %w", flushErr)
	}
	if w.file != nil {
		err = errors.Join(err, w.file.Close())
	}
	return err
}
