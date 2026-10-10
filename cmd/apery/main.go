package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/runtime"
	"github.com/compuficial/apery/internal/writer"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Exit codes are part of the CLI contract (docs/spec.md §10.6).
const (
	exitOK          = 0
	exitValidation  = 1   // plan, flag, or usage error
	exitGeneration  = 2   // failure while generating rows
	exitIO          = 3   // cannot create, write, or finalize output
	exitMismatch    = 4   // verify: files differ from what the plan generates
	exitInterrupted = 130 // stopped by SIGINT (128 + 2)
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// exitError carries the exit code for an error returned by a command.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func withCode(code int, err error) error {
	return &exitError{code: code, err: err}
}

// errReported marks an error whose details a command already printed.
var errReported = errors.New("reported")

// run executes the CLI and returns the process exit code. Commands are built
// per call so flag state never leaks between invocations (or tests).
func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return exitOK
	}
	if !errors.Is(err, errReported) {
		_, _ = fmt.Fprintln(stderr, "Error:", err) // if stderr fails, nothing is left to tell
	}
	return exitCode(err)
}

// exitCode maps an error to the CLI's exit-code contract.
func exitCode(err error) int {
	var coded *exitError
	var invalid *plan.ValidationError
	switch {
	case errors.As(err, &coded):
		return coded.code
	case errors.As(err, &invalid):
		return exitValidation
	case errors.Is(err, context.Canceled):
		return exitInterrupted
	case errors.Is(err, writer.ErrMismatch): // before ErrWrite, which wraps it
		return exitMismatch
	case errors.Is(err, runtime.ErrWrite):
		return exitIO
	default:
		return exitValidation // cobra usage and flag errors
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "apery",
		Short: "Deterministic synthetic data generator",
		Long: `Apery generates synthetic data from declarative YAML/JSON plans.
Given the same plan and seed, it produces identical output every time.

Use 'apery generate' to produce data, 'apery validate' to check a plan,
or 'apery list generators' to discover available generators.`,
		Version:       Version,
		SilenceErrors: true,
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print the version",
			RunE: func(cmd *cobra.Command, args []string) error {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "apery version", Version)
				return err
			},
		},
		newGenerateCmd(),
		newVerifyCmd(),
		newValidateCmd(),
		newListCmd(),
		newDescribeCmd(),
	)
	return root
}
