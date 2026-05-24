package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// deletePaths removes each path (file or directory) under root.
// Missing paths are silently skipped so feature manifests can list optional
// files without breaking idempotency.
func deletePaths(root string, paths []string) error {
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.RemoveAll(full); err != nil {
			return fmt.Errorf("remove %s: %w", full, err)
		}
	}
	return nil
}
