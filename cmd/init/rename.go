package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// oldModule is the path imprinted on the template; the find/replace walk
// rewrites every occurrence inside .go / .md / .yaml / .yml / Makefile-style
// files. Keep this in sync with the root go.mod's `module` directive.
const oldModule = "github.com/rock288/go-mongo-boilerplate"

// replaceableExtensions lists files where the module path may appear besides
// .go (docs, configs, helper scripts). Marker files in MarkerFiles already
// include these — but those are scoped to feature blocks; this walk is
// independent (covers the whole repo).
var replaceableExtensions = map[string]bool{
	".go":      true,
	".md":      true,
	".yaml":    true,
	".yml":     true,
	".sh":      true,
	".example": true,
}

// renameModule updates go.mod via `go mod edit` then replaces every literal
// occurrence of oldModule inside source / docs / configs. Called BEFORE
// feature strip so wire_gen.go does not exist yet and only the user-authored
// source carries old paths.
//
// no-op when newModule is empty or already matches oldModule.
func renameModule(root, newModule string) error {
	if newModule == "" {
		return nil
	}
	if newModule == oldModule {
		return nil
	}
	if err := validateModulePath(newModule); err != nil {
		return fmt.Errorf("invalid module path %q: %w", newModule, err)
	}

	cmd := exec.Command("go", "mod", "edit", "-module="+newModule)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit -module=%s: %s: %w", newModule, strings.TrimSpace(string(out)), err)
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "vendor", ".git", "node_modules", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		ext := strings.ToLower(filepath.Ext(name))
		// Special filenames we want to touch even without a recognised extension.
		isSpecial := name == "Makefile" || name == "Dockerfile" || name == "go.mod" || strings.HasPrefix(name, ".env")
		if !replaceableExtensions[ext] && !isSpecial {
			return nil
		}
		// Skip the rename CLI itself — its own oldModule constant must stay.
		if strings.HasSuffix(filepath.ToSlash(path), "/cmd/init/rename.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), oldModule) {
			return nil
		}
		replaced := strings.ReplaceAll(string(data), oldModule, newModule)
		return os.WriteFile(path, []byte(replaced), info.Mode().Perm())
	})
}

// validateModulePath enforces the minimum shape go-mod considers valid.
// It does NOT do full RFC validation — Go's own `go mod edit` will reject
// truly malformed paths.
func validateModulePath(s string) error {
	if s == "" {
		return fmt.Errorf("module path required")
	}
	if !strings.Contains(s, "/") {
		return fmt.Errorf("must contain /, e.g. github.com/you/repo")
	}
	if strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return fmt.Errorf("no leading/trailing slash")
	}
	if strings.ContainsAny(s, " \t\n") {
		return fmt.Errorf("no whitespace allowed")
	}
	return nil
}

// detectCurrentModule reads the project's go.mod to seed the prompt default.
func detectCurrentModule(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("no module directive in go.mod")
}
