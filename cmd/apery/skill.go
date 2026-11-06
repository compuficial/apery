package main

import (
	"os"
	"path/filepath"

	"github.com/compuficial/apery"

	"github.com/spf13/cobra"
)

func newSkillCmd() *cobra.Command {
	skill := &cobra.Command{
		Use:   "skill",
		Short: "Manage the apery agent skill",
	}

	install := &cobra.Command{
		Use:   "install",
		Short: "Install the agent skill for coding agents",
		Long: `Write the apery agent skill (SKILL.md) bundled in this binary.

By default it goes to ~/.claude/skills (Claude Code) and ~/.agents/skills
(Codex, Cline, Warp, Zed, and other agents that read ~/.agents). Rerun after
upgrading apery to keep the skill in sync with the CLI.`,
		Example: `  apery skill install
  apery skill install --dir .claude/skills`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dirs, _ := cmd.Flags().GetStringArray("dir")
			if len(dirs) == 0 {
				home, err := os.UserHomeDir()
				if err != nil {
					return withCode(exitIO, err)
				}
				dirs = []string{
					filepath.Join(home, ".claude", "skills"),
					filepath.Join(home, ".agents", "skills"),
				}
			}

			out := &printer{w: cmd.OutOrStdout()}
			for _, dir := range dirs {
				path, err := writeSkill(dir)
				if err != nil {
					return withCode(exitIO, err)
				}
				out.printf("Installed skill → %s\n", path)
			}
			return out.err
		},
	}
	install.Flags().StringArray("dir", nil, "Skills directory to install into (repeatable; replaces the defaults)")

	skill.AddCommand(install)
	return skill
}

// writeSkill writes the embedded skill to dir/apery/SKILL.md and returns
// the file path.
func writeSkill(dir string) (string, error) {
	skillDir := filepath.Join(dir, "apery")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(skillDir, "SKILL.md")
	return path, os.WriteFile(path, []byte(apery.Skill), 0o644)
}
