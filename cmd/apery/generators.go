package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/compuficial/apery/internal/registry"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newListCmd() *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List resources",
		Long:  `List available resources such as generators.`,
	}

	generators := &cobra.Command{
		Use:   "generators",
		Short: "List all available generators",
		Long: `Print all registered generators. The default table shows names and
descriptions; -o json or -o yaml prints full metadata (config keys, types,
defaults, examples) for programmatic use.`,
		Example: `  apery list generators
  apery list generators -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("output")
			infos := registry.ListGenerators()
			if format != "table" {
				return printStructured(cmd.OutOrStdout(), format, infos)
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tDESCRIPTION")
			for _, info := range infos {
				fmt.Fprintf(tw, "%s\t%s\n", info.Name, info.Description)
			}
			return tw.Flush()
		},
	}
	generators.Flags().StringP("output", "o", "table", "Output format: table, json, yaml")

	list.AddCommand(generators)
	return list
}

func newDescribeCmd() *cobra.Command {
	describe := &cobra.Command{
		Use:   "describe",
		Short: "Describe a resource",
		Long:  `Show detailed information about a resource.`,
	}

	generator := &cobra.Command{
		Use:   "generator <name>",
		Short: "Show detailed info for a generator",
		Long: `Print the full configuration schema, defaults, and a YAML usage example
for the specified generator.`,
		Example: `  apery describe generator int
  apery describe generator rel_ref -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			info, ok := registry.GetInfo(args[0])
			if !ok {
				return withCode(exitValidation, fmt.Errorf("unknown generator %q; run 'apery list generators' to see available generators", args[0]))
			}

			format, _ := cmd.Flags().GetString("output")
			if format != "text" {
				return printStructured(cmd.OutOrStdout(), format, info)
			}
			return printGeneratorInfo(cmd.OutOrStdout(), info)
		},
	}
	generator.Flags().StringP("output", "o", "text", "Output format: text, json, yaml")

	planSchema := &cobra.Command{
		Use:   "plan-schema",
		Short: "Print the JSON Schema for plan files",
		Long: `Print a JSON Schema (draft 2020-12) describing plan files, built from
the registered generators' metadata. Editors and agents can use it to check
plans before running 'apery validate', which performs the full checks.`,
		Example: `  apery describe plan-schema > apery.schema.json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("output")
			return printStructured(cmd.OutOrStdout(), format, planSchema())
		},
	}
	planSchema.Flags().StringP("output", "o", "json", "Output format: json, yaml")

	describe.AddCommand(generator, planSchema)
	return describe
}

func printGeneratorInfo(w io.Writer, info registry.GeneratorInfo) error {
	fmt.Fprintf(w, "Generator: %s\n", info.Name)
	fmt.Fprintf(w, "Description: %s\n", info.Description)

	if len(info.ConfigKeys) > 0 {
		fmt.Fprintln(w, "\nConfig:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
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
		if err := tw.Flush(); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(w, "\nConfig: none")
	}

	if info.Example != "" {
		fmt.Fprintln(w, "\nExample:")
		for _, line := range strings.Split(info.Example, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	return nil
}

// printStructured writes v as indented JSON or as YAML.
func printStructured(w io.Writer, format string, v any) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "yaml":
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return err
		}
		return enc.Close()
	default:
		return withCode(exitValidation, fmt.Errorf("unsupported output format %q", format))
	}
}
