package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func makeRecord(pairs ...any) *OrderedMap {
	if len(pairs)%2 != 0 {
		panic("makeRecord requires even number of args")
	}
	record := NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			panic("makeRecord key must be string")
		}
		record.Set(key, pairs[i+1])
	}
	return record
}
