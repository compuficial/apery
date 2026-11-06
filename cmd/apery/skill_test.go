package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/compuficial/apery"
)

// assertSkillAt fails unless dir/apery/SKILL.md holds the embedded skill.
func assertSkillAt(t *testing.T, dir string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "apery", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != apery.Skill {
		t.Errorf("%s: SKILL.md differs from the embedded skill", dir)
	}
}

func TestSkillInstallDefaultDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, stderr, code := runCLI(t, "skill", "install")
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	assertSkillAt(t, filepath.Join(home, ".claude", "skills"))
	assertSkillAt(t, filepath.Join(home, ".agents", "skills"))
}

func TestSkillInstallCustomDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(t.TempDir(), "skills")

	_, stderr, code := runCLI(t, "skill", "install", "--dir", dir)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	assertSkillAt(t, dir)
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Error("--dir should replace the default directories")
	}
}

func TestSkillInstallUnwritableDir(t *testing.T) {
	file := writeFile(t, t.TempDir(), "not-a-dir", "")

	_, _, code := runCLI(t, "skill", "install", "--dir", file)
	if code != exitIO {
		t.Errorf("exit %d, want %d", code, exitIO)
	}
}
