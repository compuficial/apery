package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// printer writes formatted output and keeps the first error, so a function
// that prints many lines checks once at the end ("errors are values").
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func (p *printer) println(args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintln(p.w, args...)
	}
}

// check records err unless an earlier error is already kept.
func (p *printer) check(err error) {
	if p.err == nil {
		p.err = err
	}
}

// markRequired marks flags as required. That fails only for a flag the
// command doesn't define, a programming error, so it panics.
func markRequired(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}
}
