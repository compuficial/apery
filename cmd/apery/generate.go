package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"apery/internal/plan"
	"apery/internal/runtime"
	"apery/internal/writer"

	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate synthetic data from a plan file",
	Long: `Generate reads a YAML or JSON plan file and produces synthetic data.
Output goes to stdout by default (pipe-friendly). Use --output-dir to write files instead.

By default all entities are written to a single stream. Use --split-entities with
--output-dir to write one file per entity.`,
	Example: `  # Generate JSONL to stdout
  apery generate -f plan.yaml

  # Generate CSV to stdout
  apery generate -f plan.yaml -o csv

  # Write to a directory
  apery generate -f plan.yaml --output-dir ./out

  # One file per entity
  apery generate -f plan.yaml --output-dir ./out --split-entities

  # Validate only (dry run)
  apery generate -f plan.yaml --dry-run

  # Override seed and worker count
  apery generate -f plan.yaml --seed 123 --workers 8`,
	RunE: runGenerate,
}

func init() {
	f := generateCmd.Flags()
	f.StringP("file", "f", "", "Plan file path (required)")
	f.StringP("output", "o", "jsonl", "Output format: jsonl, csv")
	f.String("output-dir", "", "Write output to directory instead of stdout")
	f.Bool("split-entities", false, "Write one file per entity (requires --output-dir)")
	f.Bool("dry-run", false, "Validate plan without generating")
	f.Int64("seed", 0, "Override the seed defined in the plan file")
	f.Int("workers", 0, "Number of parallel workers, auto-detected if not set")
	f.Int64("chunk-size", 0, "Rows per chunk, defaults to 50000 if not set")
	f.Bool("verbose", false, "Show entity progress on stderr")
	f.Bool("debug", false, "Show detailed debug output on stderr")

	generateCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	format, _ := cmd.Flags().GetString("output")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	splitEntities, _ := cmd.Flags().GetBool("split-entities")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	seedOverride, _ := cmd.Flags().GetInt64("seed")
	workers, _ := cmd.Flags().GetInt("workers")
	chunkSize, _ := cmd.Flags().GetInt64("chunk-size")
	verbose, _ := cmd.Flags().GetBool("verbose")
	debug, _ := cmd.Flags().GetBool("debug")

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	if format != "jsonl" && format != "csv" {
		exitWithError(fmt.Sprintf("unsupported output format %q (use jsonl or csv)", format), exitValidation)
	}

	if splitEntities && outputDir == "" {
		exitWithError("--split-entities requires --output-dir", exitValidation)
	}

	p, err := plan.LoadFile(file)
	if err != nil {
		exitWithError(err.Error(), exitValidation)
	}

	if cmd.Flags().Changed("seed") {
		p.Seed = seedOverride
	}

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Plan is valid.")
		return nil
	}

	w, err := createWriter(format, outputDir, splitEntities)
	if err != nil {
		exitWithError(err.Error(), exitIO)
	}

	var opts []runtime.Option
	if workers > 0 {
		opts = append(opts, runtime.WithWorkers(workers))
	}
	if chunkSize > 0 {
		opts = append(opts, runtime.WithChunkSize(chunkSize))
	}
	if verbose || debug {
		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		opts = append(opts, runtime.WithLogger(logger))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	executor := runtime.New(w, opts...)
	if err := executor.Run(ctx, p); err != nil {
		exitWithError(err.Error(), exitGeneration)
	}

	return nil
}

func createWriter(format, outputDir string, split bool) (writer.Writer, error) {
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return nil, fmt.Errorf("create output directory: %w", err)
		}
	}

	if split {
		return writer.NewSplitWriter(outputDir, format), nil
	}

	if outputDir != "" {
		path := filepath.Join(outputDir, "output."+format)
		switch format {
		case "jsonl":
			return writer.NewJSONLWriter(path)
		case "csv":
			return writer.NewCSVWriter(path)
		}
	}

	switch format {
	case "jsonl":
		return writer.NewJSONLWriterFromWriter(os.Stdout), nil
	case "csv":
		return writer.NewCSVWriterFromWriter(os.Stdout), nil
	}

	return nil, fmt.Errorf("unsupported format %q", format)
}
