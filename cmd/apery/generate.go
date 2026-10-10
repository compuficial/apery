package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/runtime"
	"github.com/compuficial/apery/internal/writer"

	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate synthetic data from a plan file",
		Long: `Generate reads a YAML or JSON plan file and produces synthetic data.
Output goes to stdout by default (pipe-friendly). Use --output-dir to write files instead.

By default all entities are written to a single stream. Use --split-entities with
--output-dir to write one file per entity. CSV holds one entity per stream, so
multi-entity CSV needs --split-entities.

Files in --output-dir appear only when the whole run succeeds; a failed or
interrupted run leaves the directory untouched.`,
		Example: `  # Generate JSONL to stdout
  apery generate -f plan.yaml

  # Generate CSV to stdout (single-entity plans)
  apery generate -f plan.yaml -o csv

  # Write to a directory
  apery generate -f plan.yaml --output-dir ./out

  # One file per entity
  apery generate -f plan.yaml --output-dir ./out --split-entities

  # Preview: validate and show row estimates and columns, no data
  apery generate -f plan.yaml --dry-run

  # Quick sample: at most 5 rows per entity
  apery generate -f plan.yaml --limit 5

  # Only the Order table (entities it references are generated, not written)
  apery generate -f plan.yaml --entities Order -o csv

  # Override seed and worker count
  apery generate -f plan.yaml --seed 123 --workers 8`,
		Args: cobra.NoArgs,
		RunE: runGenerate,
	}

	f := cmd.Flags()
	f.StringP("file", "f", "", "Plan file path (required)")
	f.StringP("output", "o", "jsonl", "Output format: jsonl, csv")
	f.String("output-dir", "", "Write output to directory instead of stdout")
	f.Bool("split-entities", false, "Write one file per entity (requires --output-dir)")
	f.Bool("dry-run", false, "Validate the plan and print row estimates and columns; generate nothing")
	f.Int64("seed", 0, "Override the seed defined in the plan file")
	f.Int("workers", 0, "Number of parallel workers, auto-detected if not set")
	f.Int64("chunk-size", 0, "Rows per chunk, defaults to 10000 if not set")
	f.Int64("limit", 0, "Generate at most N rows per entity (references sample from the capped rows)")
	f.StringSlice("entities", nil, "Write only these entities (comma-separated); entities they reference are generated but not written")
	f.Bool("verbose", false, "Show entity progress on stderr")
	f.Bool("debug", false, "Show detailed debug output on stderr")
	cmd.MarkFlagRequired("file")
	return cmd
}

// generateOptions are the parsed generate flags.
type generateOptions struct {
	file      string
	format    string
	outputDir string
	split     bool
	dryRun    bool
	workers   int
	chunkSize int64
	limit     int64
	entities  []string    // nil = all
	logLevel  *slog.Level // nil = no logging
}

func parseGenerateFlags(cmd *cobra.Command) (generateOptions, error) {
	f := cmd.Flags()
	var o generateOptions
	o.file, _ = f.GetString("file")
	o.format, _ = f.GetString("output")
	o.outputDir, _ = f.GetString("output-dir")
	o.split, _ = f.GetBool("split-entities")
	o.dryRun, _ = f.GetBool("dry-run")
	o.workers, _ = f.GetInt("workers")
	o.chunkSize, _ = f.GetInt64("chunk-size")
	o.limit, _ = f.GetInt64("limit")
	names, _ := f.GetStringSlice("entities")
	o.entities = uniqueNames(names)

	verbose, _ := f.GetBool("verbose")
	debug, _ := f.GetBool("debug")
	switch {
	case debug:
		level := slog.LevelDebug
		o.logLevel = &level
	case verbose:
		level := slog.LevelInfo
		o.logLevel = &level
	}

	if o.format != "jsonl" && o.format != "csv" {
		return o, fmt.Errorf("unsupported output format %q (use jsonl or csv)", o.format)
	}
	if o.split && o.outputDir == "" {
		return o, errors.New("--split-entities requires --output-dir")
	}
	if o.limit < 0 {
		return o, errors.New("--limit must be >= 0")
	}
	return o, nil
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	o, err := parseGenerateFlags(cmd)
	if err != nil {
		return withCode(exitValidation, err)
	}

	p, err := plan.LoadFile(o.file)
	if err != nil {
		return withCode(exitValidation, err)
	}
	if cmd.Flags().Changed("seed") {
		p.Seed, _ = cmd.Flags().GetInt64("seed")
	}
	written := len(p.Entities)
	if len(o.entities) > 0 {
		written = len(o.entities)
	}
	if o.format == "csv" && !o.split && written > 1 {
		return withCode(exitValidation, fmt.Errorf(
			"CSV output holds one entity per stream and this run writes %d; use --output-dir with --split-entities, pick one with --entities, or use -o jsonl", written))
	}

	if o.dryRun {
		sel, err := runtime.Select(p, o.entities)
		if err != nil {
			return err
		}
		printSummary(cmd.OutOrStdout(), p, o.limit, sel)
		return nil
	}

	out, err := newOutput(o, cmd.OutOrStdout())
	if err != nil {
		return withCode(exitIO, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := runtime.New(out.writer, executorOptions(o, cmd.ErrOrStderr())...).Run(ctx, p); err != nil {
		out.discard()
		var invalid *plan.ValidationError
		if errors.As(err, &invalid) || errors.Is(err, context.Canceled) || errors.Is(err, runtime.ErrWrite) {
			return err
		}
		return withCode(exitGeneration, err)
	}

	if err := out.commit(); err != nil {
		return withCode(exitIO, err)
	}
	return nil
}

func executorOptions(o generateOptions, logOut io.Writer) []runtime.Option {
	var opts []runtime.Option
	if o.workers > 0 {
		opts = append(opts, runtime.WithWorkers(o.workers))
	}
	if o.chunkSize > 0 {
		opts = append(opts, runtime.WithChunkSize(o.chunkSize))
	}
	if o.limit > 0 {
		opts = append(opts, runtime.WithRowLimit(o.limit))
	}
	if len(o.entities) > 0 {
		opts = append(opts, runtime.WithEntities(o.entities...))
	}
	if o.logLevel != nil {
		logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: *o.logLevel}))
		opts = append(opts, runtime.WithLogger(logger))
	}
	return opts
}

// uniqueNames trims names and drops empty and repeated ones, keeping order.
func uniqueNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// printSummary renders the dry-run preview: rows (exact, or min-max with
// the expected count for driven_by) and columns per entity. Entities outside
// the selection are left out, and dependencies are marked.
func printSummary(w io.Writer, p *plan.Plan, limit int64, sel runtime.Selection) {
	fmt.Fprintln(w, "Plan is valid.")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ENTITY\tROWS\tCOLUMNS")
	for _, s := range plan.Summarize(p) {
		name := s.Name
		switch {
		case slices.Contains(sel.Dependencies, s.Name):
			name += " (dependency)"
		case !slices.Contains(sel.Written, s.Name):
			continue
		}

		rows := s.Rows
		if limit > 0 {
			rows.Min, rows.Max = min(rows.Min, limit), min(rows.Max, limit)
			rows.Expected = min(rows.Expected, float64(limit))
		}
		count := fmt.Sprint(rows.Min)
		if rows.Max != rows.Min {
			count = fmt.Sprintf("%d-%d (~%.0f)", rows.Min, rows.Max, rows.Expected)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", name, count, strings.Join(s.Columns, ", "))
	}
	tw.Flush()

	if len(sel.Dependencies) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "(dependency): generated because written entities reference it, but not written.")
	}
}

// output is where generated records go. Files are written to a staging
// directory inside --output-dir and moved into place only on success, so a
// failed run never leaves partial files.
type output struct {
	writer  writer.Writer
	staging string // "" when writing to stdout
	dest    string
}

func newOutput(o generateOptions, stdout io.Writer) (*output, error) {
	if o.outputDir == "" {
		if o.format == "csv" {
			return &output{writer: writer.NewCSVWriterFromWriter(stdout)}, nil
		}
		return &output{writer: writer.NewJSONLWriterFromWriter(stdout)}, nil
	}

	if err := os.MkdirAll(o.outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	staging, err := os.MkdirTemp(o.outputDir, ".apery-staging-")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	out := &output{staging: staging, dest: o.outputDir}

	if o.split {
		out.writer = writer.NewSplitWriter(staging, o.format)
		return out, nil
	}

	path := filepath.Join(staging, "output."+o.format)
	if o.format == "csv" {
		out.writer, err = writer.NewCSVWriter(path)
	} else {
		out.writer, err = writer.NewJSONLWriter(path)
	}
	if err != nil {
		out.discard()
		return nil, err
	}
	return out, nil
}

// commit moves staged files into the output directory.
func (o *output) commit() error {
	if o.staging == "" {
		return nil
	}
	defer os.RemoveAll(o.staging)

	entries, err := os.ReadDir(o.staging)
	if err != nil {
		return fmt.Errorf("read staging directory: %w", err)
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(o.staging, e.Name()), filepath.Join(o.dest, e.Name())); err != nil {
			return fmt.Errorf("move %s into place: %w", e.Name(), err)
		}
	}
	return nil
}

// discard removes staged files after a failed run.
func (o *output) discard() {
	if o.staging != "" {
		os.RemoveAll(o.staging)
	}
}
