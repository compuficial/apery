package writer

import (
	"fmt"
	"path/filepath"
)

// SplitWriter routes records to per-entity files. Each entity gets its own
// file created lazily on first WriteRecord. The _entity column/field is omitted
// since the filename carries the entity identity.
type SplitWriter struct {
	dir     string
	format  string
	writers map[string]Writer
}

// NewSplitWriter creates a writer that produces one file per entity in dir.
func NewSplitWriter(dir, format string) *SplitWriter {
	return &SplitWriter{
		dir:     dir,
		format:  format,
		writers: make(map[string]Writer),
	}
}

func (sw *SplitWriter) writerFor(entity string) (Writer, error) {
	if w, ok := sw.writers[entity]; ok {
		return w, nil
	}

	path := filepath.Join(sw.dir, entity+"."+sw.format)

	var w Writer
	var err error
	switch sw.format {
	case "jsonl":
		w, err = NewJSONLWriterSplit(path)
	case "csv":
		w, err = NewCSVWriterSplit(path)
	default:
		return nil, fmt.Errorf("split writer: unsupported format %q", sw.format)
	}
	if err != nil {
		return nil, fmt.Errorf("split writer: create %s: %w", path, err)
	}

	sw.writers[entity] = w
	return w, nil
}

func (sw *SplitWriter) WriteRecord(entity string, record *OrderedMap) error {
	w, err := sw.writerFor(entity)
	if err != nil {
		return err
	}
	return w.WriteRecord(entity, record)
}

func (sw *SplitWriter) Close() error {
	var firstErr error
	for _, w := range sw.writers {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
