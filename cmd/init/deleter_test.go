package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeletePaths_NestedDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := deletePaths(root, []string{"a"}); err != nil {
		t.Fatalf("deletePaths: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, got err=%v", err)
	}
}

func TestDeletePaths_MissingPathIsIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := deletePaths(root, []string{"never-existed"}); err != nil {
		t.Fatalf("expected no error for missing path, got %v", err)
	}
}

func TestDeletePaths_SingleFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "file.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := deletePaths(root, []string{"file.txt"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}
