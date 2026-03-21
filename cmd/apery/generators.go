package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"apery/internal/registry"

	"github.com/spf13/cobra"
)

var generatorsCmd = &cobra.Command{
	Use:   "generators",
	Short: "List and describe available generators",
	Long:  `Discover available generators, their configuration, and usage examples.`,
}

var generatorsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available generators",
	Long:  `Print a table of all registered generators with their names and descriptions.`,
	Example: `  apery generators list`,
	Run: func(cmd *cobra.Command, args []string) {
		infos := registry.ListGenerators()
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tDESCRIPTION")
		for _, info := range infos {
			fmt.Fprintf(tw, "%s\t%s\n", info.Name, info.Description)
		}
		tw.Flush()
	},
}

var generatorsDescribeCmd = &cobra.Command{
	Use:   "describe <generator>",
	Short: "Show detailed info for a generator",
	Long: `Print the full configuration schema, defaults, and a YAML usage example
for the specified generator.`,
	Example: `  apery generators describe int
  apery generators describe rel_ref`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		info, ok := registry.GetInfo(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown generator %q\n", name)
			fmt.Fprintln(os.Stderr, "Run 'apery generators list' to see available generators.")
			os.Exit(exitValidation)
		}

		fmt.Printf("Generator: %s\n", info.Name)
		fmt.Printf("Description: %s\n", info.Description)

		if len(info.ConfigKeys) > 0 {
			fmt.Println("\nConfig:")
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  KEY\tTYPE\tREQUIRED\tDEFAULT\tDESCRIPTION")
			for _, ck := range info.ConfigKeys {
				req := ""
				if ck.Required {
					req = "yes"
				}
				def := ck.Default
				if def == "" {
					def = "-"
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", ck.Name, ck.Type, req, def, ck.Desc)
			}
			tw.Flush()
		} else {
			fmt.Println("\nConfig: none")
		}

		if info.Example != "" {
			fmt.Println("\nExample:")
			for _, line := range strings.Split(info.Example, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
	},
}

func init() {
	generatorsCmd.AddCommand(generatorsListCmd)
	generatorsCmd.AddCommand(generatorsDescribeCmd)
	rootCmd.AddCommand(generatorsCmd)
}
