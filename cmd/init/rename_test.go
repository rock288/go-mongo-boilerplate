package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateModulePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"github.com/acme/svc", false},
		{"example.com/x/y", false},
		{"", true},
		{"justaword", true},
		{"/leading-slash/x", true},
		{"trailing-slash/x/", true},
		{"has space/x", true},
		{"newline/\nbad", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			err := validateModulePath(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateModulePath(%q) = %v, wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestDetectCurrentModule(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gomod := "module github.com/acme/foo\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := detectCurrentModule(root)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if got != "github.com/acme/foo" {
		t.Fatalf("got %q", got)
	}
}

func TestRenameModule_NoopWhenEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// renameModule with empty path should not invoke `go mod edit`.
	if err := renameModule(root, ""); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

func TestRenameModule_NoopWhenSame(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := renameModule(root, oldModule); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

func TestRenameModule_ReplacesAcrossFiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary required")
	}
	root := t.TempDir()
	mustWrite(t, root, "go.mod", "module "+oldModule+"\n\ngo 1.22\n")
	mustWrite(t, root, "a.go", "package x\nimport \""+oldModule+"/internal/user\"\n")
	mustWrite(t, root, "subdir/b.go", "package y\nimport \""+oldModule+"/internal/role\"\n")
	mustWrite(t, root, "README.md", "# Project\nuses "+oldModule+" right now.\n")
	mustWrite(t, root, "ignore.txt", oldModule+" should be ignored in .txt\n")
	mustWrite(t, root, "vendor/v.go", "package v // "+oldModule+"\n") // vendor skipped
	mustWrite(t, root, "cmd/init/rename.go", "const oldModule = \""+oldModule+"\"\n")

	newPath := "example.com/acme/new"
	if err := renameModule(root, newPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	cases := []struct {
		path        string
		mustContain string
		mustExclude string
	}{
		{"a.go", newPath, oldModule},
		{"subdir/b.go", newPath, oldModule},
		{"README.md", newPath, oldModule},
		{"go.mod", "module " + newPath, "module " + oldModule},
		// Skipped:
		{"ignore.txt", oldModule, newPath},
		{"vendor/v.go", oldModule, newPath},
		{"cmd/init/rename.go", oldModule, newPath},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(filepath.Join(root, tc.path))
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		body := string(data)
		if !strings.Contains(body, tc.mustContain) {
			t.Errorf("%s missing %q\n%s", tc.path, tc.mustContain, body)
		}
		if strings.Contains(body, tc.mustExclude) {
			t.Errorf("%s still contains %q\n%s", tc.path, tc.mustExclude, body)
		}
	}
}

func TestRenameModule_RejectsInvalid(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := renameModule(root, "no-slash"); err == nil {
		t.Fatal("expected validation error")
	}
}

func mustWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
