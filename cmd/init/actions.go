package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PostAction is one command run after strip/delete to bring the project back to
// a compilable state. Order is significant — see PostActions.
type PostAction struct {
	Name string
	Bin  string
	Args []string
	// SkipIf, when non-nil and returning true, causes the action to be skipped.
	SkipIf func(root string) bool
}

// PostActions is the canonical post-strip pipeline.
//
// Order rationale (locked by red-team C2):
//  1. wire ./...     — regenerate wire_gen.go from the new, stripped graph.
//  2. go mod tidy    — only safe AFTER wire_gen.go exists, otherwise the
//     import graph is broken and tidy will fail.
//  3. mockery        — regenerate mocks against the new interfaces.
//  4. go build ./... — final fail-loud check.
var PostActions = []PostAction{
	// gofmt first: stripping marker blocks can leave grouped declarations
	// (e.g. consecutive `func ProvideXxx`) with stale column alignment that
	// gofmt + goimports flag in CI. Reformat before wire reads the source.
	{Name: "gofmt", Bin: "gofmt", Args: []string{"-w", "."}},
	{Name: "wire regen", Bin: "wire", Args: []string{"./..."}},
	{Name: "go mod tidy", Bin: "go", Args: []string{"mod", "tidy"}},
	{Name: "mockery regen", Bin: "mockery", Args: []string{}, SkipIf: skipIfNoMockeryConfig},
	{Name: "go build", Bin: "go", Args: []string{"build", "./..."}},
}

// checkPrereqs verifies the CLIs that PostActions depends on are reachable.
// Returns a human-readable install hint on failure (per validate V2).
func checkPrereqs() error {
	for _, bin := range []string{"wire", "mockery"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf(
				"%s not found in PATH; install with:\n"+
					"  go install github.com/google/wire/cmd/wire@latest\n"+
					"  go install github.com/vektra/mockery/v3@latest\n"+
					"and ensure $(go env GOPATH)/bin is on PATH",
				bin,
			)
		}
	}
	return nil
}

// runPostActions executes the pipeline above against root.
// Pre-step: stale wire_gen.go files are deleted so wire regenerates cleanly;
// when cmd/worker/ was removed by cascade the stale file is simply absent.
func runPostActions(root string) error {
	if err := checkPrereqs(); err != nil {
		return err
	}

	for _, f := range []string{"cmd/server/wire_gen.go", "cmd/worker/wire_gen.go"} {
		_ = os.Remove(filepath.Join(root, f))
	}

	for _, a := range PostActions {
		if a.SkipIf != nil && a.SkipIf(root) {
			continue
		}
		cmd := exec.Command(a.Bin, a.Args...)
		cmd.Dir = root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s (cd %s && %s %v): %w", a.Name, root, a.Bin, a.Args, err)
		}
	}
	return nil
}

// skipIfNoMockeryConfig returns true when .mockery.yaml is missing OR has no
// remaining `packages:` entries after a strip. Mockery fails with
// "no packages specified in config" on an empty map, so detecting that and
// skipping is necessary for end-to-end strip runs.
func skipIfNoMockeryConfig(root string) bool {
	path := filepath.Join(root, ".mockery.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// A package entry under `packages:` is indented and ends with ":".
		if strings.HasPrefix(line, "  ") && strings.HasSuffix(trimmed, ":") && strings.Contains(trimmed, "/") {
			return false
		}
	}
	return true
}
