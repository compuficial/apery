package writer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// ErrMismatch marks output that differs from the files it is checked
// against. Every *Mismatch matches it with errors.Is.
var ErrMismatch = errors.New("output differs")

// Mismatch reasons.
const (
	ReasonContent    = "content differs"
	ReasonFileShort  = "file ends early"
	ReasonFileLong   = "file has extra data"
	ReasonMissing    = "missing file"
	ReasonUnexpected = "unexpected file"
)

// maxShownLine caps how much of a differing line a Mismatch carries.
const maxShownLine = 200

// Mismatch is the first place generated output differs from a file.
type Mismatch struct {
	File   string `json:"file"`
	Line   int64  `json:"line,omitempty"` // 1-based; 0 for whole-file problems
	Reason string `json:"reason"`
	Want   string `json:"want,omitempty"` // the generated line, truncated
	Got    string `json:"got,omitempty"`  // the file's line, truncated
}

func (m *Mismatch) Error() string {
	if m.Line == 0 {
		return fmt.Sprintf("%s: %s", m.File, m.Reason)
	}
	return fmt.Sprintf("%s:%d: %s", m.File, m.Line, m.Reason)
}

// Unwrap makes every Mismatch match ErrMismatch.
func (m *Mismatch) Unwrap() error { return ErrMismatch }

// Comparer is an io.WriteCloser that checks the bytes written to it against
// a file instead of writing them. The first difference comes back as a
// *Mismatch from Write or Close, and later writes return it again.
type Comparer struct {
	path    string
	file    *os.File
	r       *bufio.Reader
	got     []byte // scratch for the file's bytes
	line    int64  // 1-based line of the next byte
	partial []byte // the current line's bytes so far, equal in both streams
	err     error  // the first mismatch, once found
}

// OpenComparer opens path for comparison. A missing file is a *Mismatch.
func OpenComparer(path string) (*Comparer, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &Mismatch{File: path, Reason: ReasonMissing}
	}
	if err != nil {
		return nil, err
	}
	return &Comparer{path: path, file: f, r: bufio.NewReaderSize(f, 64<<10), line: 1}, nil
}

// Write compares p with the file's next len(p) bytes.
func (c *Comparer) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if cap(c.got) < len(p) {
		c.got = make([]byte, len(p))
	}
	got := c.got[:len(p)]
	n, err := io.ReadFull(c.r, got)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, err
	}
	got = got[:n]

	i := firstDifference(p, got)
	if i < 0 {
		c.advance(p)
		return len(p), nil
	}
	c.advance(p[:i])
	c.err = c.mismatch(p[i:], got[i:])
	return i, c.err
}

// Close reports extra data after the generated output, then closes the file.
func (c *Comparer) Close() error {
	err := c.err
	if err == nil {
		if _, peekErr := c.r.Peek(1); peekErr == nil {
			err = &Mismatch{File: c.path, Line: c.line, Reason: ReasonFileLong, Got: c.restOfLine(nil)}
		}
	}
	return errors.Join(err, c.file.Close())
}

// firstDifference returns the first index where want and got differ, or -1
// if they are equal. A shorter got differs at its end.
func firstDifference(want, got []byte) int {
	for i := range want {
		if i >= len(got) || want[i] != got[i] {
			return i
		}
	}
	return -1
}

// advance records matched bytes: the line count and the current line.
func (c *Comparer) advance(matched []byte) {
	c.line += int64(bytes.Count(matched, []byte{'\n'}))
	if nl := bytes.LastIndexByte(matched, '\n'); nl >= 0 {
		c.partial = append(c.partial[:0], matched[nl+1:]...)
		return
	}
	c.partial = append(c.partial, matched...)
}

// mismatch describes the difference at the current position, given the
// rest of the generated chunk and the file bytes read with it.
func (c *Comparer) mismatch(want, got []byte) *Mismatch {
	m := &Mismatch{File: c.path, Line: c.line, Reason: ReasonContent, Want: shownLine(c.partial, want)}
	if !bytes.Contains(want, []byte{'\n'}) && len(m.Want) <= maxShownLine {
		m.Want += "..." // the line continues in a later write
	}
	if len(got) == 0 {
		m.Reason = ReasonFileShort
	}
	m.Got = c.restOfLine(got)
	return m
}

// restOfLine is the current line in the file: the matched prefix, the bytes
// already read, and whatever the file holds up to the next newline.
func (c *Comparer) restOfLine(read []byte) string {
	line := append(bytes.Clone(c.partial), read...)
	if !bytes.Contains(read, []byte{'\n'}) {
		more, _ := c.r.ReadSlice('\n')
		line = append(line, more...)
	}
	return shownLine(line, nil)
}

// shownLine joins prefix and rest, cut at the first newline and capped.
func shownLine(prefix, rest []byte) string {
	line := append(bytes.Clone(prefix), rest...)
	if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	if len(line) > maxShownLine {
		return string(line[:maxShownLine]) + "..."
	}
	return string(line)
}
