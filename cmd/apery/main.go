package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "apery",
	Short: "Deterministic synthetic data generator",
	Long: `Apery generates synthetic data from declarative YAML/JSON plans.
Given the same plan and seed, it produces identical output every time.

Use 'apery generate' to produce data, 'apery validate' to check a plan,
or 'apery list generators' to discover available generators.`,
	Version: Version,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("apery version", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func exitWithError(msg string, code int) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(code)
}

const (
	exitValidation = 1
	exitGeneration = 2
	exitIO         = 3
)
