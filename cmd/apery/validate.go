package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/runtime"

	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a plan file without generating",
		Long: `Validate checks a YAML or JSON plan file for structural errors,
relational constraint violations, unknown generators, and generator
configuration problems. Every problem is reported, not just the first.

Exit code 0 means the plan is valid. Exit code 1 means validation failed.
With -o json, the result is printed to stdout as
{"valid": bool, "issues": [...], "entities": [...]}; each issue has a path,
a stable code, a message, and an optional hint.`,
		Example: `  # Validate a YAML plan
  apery validate -f plan.yaml

  # Machine-readable result for agents
  apery validate -f plan.json -o json`,
		Args: cobra.NoArgs,
		RunE: runValidate,
	}
	cmd.Flags().StringP("file", "f", "", "Plan file path (required)")
	cmd.Flags().StringP("output", "o", "text", "Output format: text, json")
	cmd.MarkFlagRequired("file")
	return cmd
}

// validationResult is the -o json document.
type validationResult struct {
	Valid    bool                 `json:"valid"`
	Issues   []plan.Issue         `json:"issues"`
	Entities []plan.EntitySummary `json:"entities,omitempty"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	file, _ := cmd.Flags().GetString("file")
	format, _ := cmd.Flags().GetString("output")
	if format != "text" && format != "json" {
		return withCode(exitValidation, fmt.Errorf("unsupported output format %q (use text or json)", format))
	}

	p, err := loadAndValidate(file)
	if format == "text" {
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Plan is valid.")
		return nil
	}

	result := validationResult{Valid: err == nil, Issues: issuesOf(err)}
	if err == nil {
		result.Entities = plan.Summarize(p)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(result); encErr != nil {
		return withCode(exitIO, encErr)
	}
	if err != nil {
		return withCode(exitValidation, errReported)
	}
	return nil
}

// loadAndValidate loads a plan and runs full validation, including building
// every generator.
func loadAndValidate(file string) (*plan.Plan, error) {
	p, err := plan.LoadFile(file)
	if err != nil {
		return nil, err
	}
	if err := runtime.Validate(p); err != nil {
		return nil, err
	}
	return p, nil
}

// issuesOf converts any load or validation error into issues; read and
// parse failures become a single parse_error issue.
func issuesOf(err error) []plan.Issue {
	if err == nil {
		return []plan.Issue{}
	}
	var invalid *plan.ValidationError
	if errors.As(err, &invalid) {
		return invalid.Issues
	}
	return []plan.Issue{{Code: "parse_error", Message: err.Error()}}
}
