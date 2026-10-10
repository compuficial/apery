package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/runtime"
	"github.com/compuficial/apery/internal/writer"

	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check that files in --output-dir still match what a plan generates",
		Long: `Verify regenerates a plan and compares the result, byte for byte, with the files
an earlier 'apery generate' wrote to --output-dir. It writes nothing. It takes the
same flags as generate, so swap the verb on the command that made the files.

Verify stops at the first difference and reports the file, the line, and both
versions of it. With --split-entities (and no --entities), a file for an entity
the plan no longer has is a difference too. Exit code 4 means the files differ.`,
		Example: `  # Files made by: apery generate -f plan.yaml --output-dir ./fixtures --split-entities
  apery verify -f plan.yaml --output-dir ./fixtures --split-entities

  # Machine-readable report
  apery verify -f plan.yaml --output-dir ./fixtures --split-entities --report json`,
		Args: cobra.NoArgs,
		RunE: runVerify,
	}
	addRunFlags(cmd)
	cmd.Flags().String("report", "text", "Report format: text, json")
	cmd.MarkFlagRequired("output-dir")
	return cmd
}

// verifyResult is the report verify prints.
type verifyResult struct {
	Match    bool             `json:"match"`
	Dir      string           `json:"dir"`
	Entities []entityRows     `json:"entities,omitempty"` // rows checked, when the files match
	Mismatch *writer.Mismatch `json:"mismatch,omitempty"`
}

// entityRows is how many rows of an entity verify checked.
type entityRows struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

func runVerify(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	o, err := parseRunFlags(cmd)
	if err != nil {
		return withCode(exitValidation, err)
	}
	report, _ := cmd.Flags().GetString("report")
	if report != "text" && report != "json" {
		return withCode(exitValidation, fmt.Errorf("unsupported report format %q (use text or json)", report))
	}
	p, err := loadRunPlan(cmd, o)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	counter := &rowCounter{}
	err = compareRun(ctx, p, o, counter, cmd.ErrOrStderr())
	if err == nil && o.split && len(o.entities) == 0 {
		err = findUnexpectedFile(o.outputDir, o.format, p)
	}

	result := verifyResult{Match: err == nil, Dir: o.outputDir}
	if m, ok := errors.AsType[*writer.Mismatch](err); ok {
		result.Mismatch = m
	} else if err != nil {
		return runFailure(err)
	} else {
		result.Entities = counter.entities
	}

	if err := printVerifyResult(cmd.OutOrStdout(), report, result); err != nil {
		return withCode(exitIO, err)
	}
	if !result.Match {
		return withCode(exitMismatch, errReported)
	}
	return nil
}

// compareRun runs the plan into comparers over the files in o.outputDir.
// The first difference comes back as a *writer.Mismatch.
func compareRun(ctx context.Context, p *plan.Plan, o runOptions, counter *rowCounter, logOut io.Writer) error {
	run := func(w writer.Writer) error {
		counter.next = w
		return runtime.New(counter, executorOptions(o, logOut)...).Run(ctx, p)
	}

	if o.split {
		return run(writer.NewSplitWriterFunc(o.outputDir, o.format, openComparer))
	}

	c, err := writer.OpenComparer(filepath.Join(o.outputDir, "output."+o.format))
	if err != nil {
		return err
	}
	w := writer.Writer(writer.NewJSONLWriterFromWriter(c))
	if o.format == "csv" {
		w = writer.NewCSVWriterFromWriter(c)
	}

	// Run flushes w into c; Close then checks for extra data.
	if err := run(w); err != nil {
		_ = c.Close() // the run's error explains the failure
		return err
	}
	return c.Close()
}

// openComparer adapts writer.OpenComparer for NewSplitWriterFunc, keeping
// a failed open a nil interface rather than a nil *Comparer.
func openComparer(path string) (io.WriteCloser, error) {
	c, err := writer.OpenComparer(path)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// findUnexpectedFile reports a file in dir that looks like split output but
// matches no entity in the plan, such as one left from a removed entity.
func findUnexpectedFile(dir, format string, p *plan.Plan) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return withCode(exitIO, err)
	}
	expected := make(map[string]bool, len(p.Entities))
	for _, e := range p.Entities {
		expected[e.Name+"."+format] = true
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || filepath.Ext(name) != "."+format || expected[name] {
			continue
		}
		return &writer.Mismatch{File: filepath.Join(dir, name), Reason: writer.ReasonUnexpected}
	}
	return nil
}

func printVerifyResult(w io.Writer, format string, r verifyResult) error {
	if format == "json" {
		return printStructured(w, "json", r)
	}

	if r.Match {
		counts := make([]string, len(r.Entities))
		for i, e := range r.Entities {
			counts[i] = fmt.Sprintf("%s %d", e.Name, e.Rows)
		}
		_, err := fmt.Fprintf(w, "OK: %s matches the plan (rows: %s)\n", r.Dir, strings.Join(counts, ", "))
		return err
	}

	m := r.Mismatch
	if _, err := fmt.Fprintf(w, "MISMATCH: %v\n", m); err != nil {
		return err
	}
	if m.Want != "" || m.Got != "" {
		_, err := fmt.Fprintf(w, "  want: %s\n  got:  %s\n", m.Want, m.Got)
		return err
	}
	return nil
}

// rowCounter passes records through to next, counting rows per entity in
// the order entities first appear.
type rowCounter struct {
	next     writer.Writer
	entities []entityRows
}

func (c *rowCounter) WriteRecord(entity string, record *writer.OrderedMap) error {
	if n := len(c.entities); n == 0 || c.entities[n-1].Name != entity {
		c.entities = append(c.entities, entityRows{Name: entity})
	}
	c.entities[len(c.entities)-1].Rows++
	return c.next.WriteRecord(entity, record)
}

func (c *rowCounter) Close() error { return c.next.Close() }
