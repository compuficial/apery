package writer

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SplitWriter routes records to per-entity files. Each entity gets its own
// file created lazily on first WriteRecord. The _entity column/field is omitted
// since the filename carries the entity identity.
type SplitWriter struct {
	dir     string
	format  string
	open    func(path string) (io.WriteCloser, error)
	writers map[string]Writer
}

// NewSplitWriter creates a writer that produces one file per entity in dir.
func NewSplitWriter(dir, format string) *SplitWriter {
	return NewSplitWriterFunc(dir, format, func(path string) (io.WriteCloser, error) {
		return os.Create(path)
	})
}

// NewSplitWriterFunc is NewSplitWriter with open in place of os.Create, for
// example OpenComparer to check existing files instead of writing them.
func NewSplitWriterFunc(dir, format string, open func(path string) (io.WriteCloser, error)) *SplitWriter {
	return &SplitWriter{
		dir:     dir,
		format:  format,
		open:    open,
		writers: make(map[string]Writer),
	}
}

func (sw *SplitWriter) writerFor(entity string) (Writer, error) {
	if w, ok := sw.writers[entity]; ok {
		return w, nil
	}

	// Plans validate entity names; this guards library callers too.
	if entity == "" || entity == "." || entity == ".." || strings.ContainsAny(entity, `/\`) {
		return nil, fmt.Errorf("split writer: entity name %q cannot be used as a file name", entity)
	}
	if sw.format != "jsonl" && sw.format != "csv" {
		return nil, fmt.Errorf("split writer: unsupported format %q", sw.format)
	}
	path := filepath.Join(sw.dir, entity+"."+sw.format)
	f, err := sw.open(path)
	if err != nil {
		return nil, fmt.Errorf("split writer: open %s: %w", path, err)
	}

	var w Writer = &JSONLWriter{file: f, buf: bufio.NewWriter(f), omitEntity: true}
	if sw.format == "csv" {
		w = &CSVWriter{file: f, w: csv.NewWriter(f), omitEntity: true}
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

// Close closes every per-entity file and reports all failures.
func (sw *SplitWriter) Close() error {
	entities := make([]string, 0, len(sw.writers))
	for entity := range sw.writers {
		entities = append(entities, entity)
	}
	slices.Sort(entities)

	var errs []error
	for _, entity := range entities {
		if err := sw.writers[entity].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
