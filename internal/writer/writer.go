// Package writer provides output format abstraction for synthetic data generation.
//
// The Writer interface defines how generated records are written to output. Implementations
// include JSONLWriter for newline-delimited JSON. OrderedMap preserves field order in output,
// ensuring consistent formatting across runs.
package writer

type Writer interface {
	WriteRecord(entity string, record *OrderedMap) error
	Close() error
}
