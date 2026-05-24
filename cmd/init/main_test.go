package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsCleanGitTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "t@example.com")
	mustRun(t, root, "git", "config", "user.name", "t")
	mustRun(t, root, "git", "commit", "--allow-empty", "-m", "init")

	if !isCleanGitTree(root) {
		t.Fatalf("expected clean tree right after empty commit")
	}

	if err := os.WriteFile(filepath.Join(root, "x.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isCleanGitTree(root) {
		t.Fatalf("expected dirty tree after creating untracked file")
	}
}

func TestIsCleanGitTree_NonRepoIsDirty(t *testing.T) {
	t.Parallel()
	if isCleanGitTree(t.TempDir()) {
		t.Fatalf("non-git directory must be treated as dirty (fail-safe)")
	}
}

func mustRun(t *testing.T, dir, bin string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
}
