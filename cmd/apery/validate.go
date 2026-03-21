package main

import (
	"fmt"

	"apery/internal/plan"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a plan file without generating",
	Long: `Validate checks a YAML or JSON plan file for structural errors,
relational constraint violations, and generator configuration problems.

Exit code 0 means the plan is valid. Exit code 1 means validation failed.`,
	Example: `  # Validate a YAML plan
  apery validate -f plan.yaml

  # Validate a JSON plan
  apery validate -f plan.json`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringP("file", "f", "", "Plan file path (required)")
	validateCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	file, _ := cmd.Flags().GetString("file")

	_, err := plan.LoadFile(file)
	if err != nil {
		exitWithError(err.Error(), exitValidation)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Plan is valid.")
	return nil
}
