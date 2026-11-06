package apery

import _ "embed"

// Skill is the agent skill (SKILL.md, Agent Skills format) that teaches
// coding agents to use apery. It ships in the binary so the skill always
// matches the CLI version.
//
//go:embed .claude/skills/apery/SKILL.md
var Skill string
