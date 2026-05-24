package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// selfDestruct strips residual feature markers, reformats the source with
// gofmt (alignment of consecutive decls can drift once marker comments are
// removed), removes cmd/init/, and runs a final `go mod tidy` to drop any
// leftover deps (most notably ensures the root go.mod stays huh-free).
//
// MUST run AFTER renameModule + per-feature strip + runPostActions all succeed.
// Until that point, init recovery is `git restore .`.
func selfDestruct(root string) error {
	// 1. Remove cmd/init FIRST. Its test files carry marker patterns inside
	//    string literals (test fixtures); leaving them in place would let
	//    stripAllResidualMarkers eat real source lines below.
	//    The currently running binary was compiled to a tmp dir by `go run`,
	//    so removing its source dir is safe.
	if err := os.RemoveAll(filepath.Join(root, "cmd/init")); err != nil {
		return fmt.Errorf("remove cmd/init: %w", err)
	}

	// 2. Strip any leftover `feature:*:start|end` lines from the rest of
	//    the repo (orphaned outer markers when nested-marker blocks were
	//    partially stripped, or stale markers in files that survived).
	if err := stripAllResidualMarkers(root); err != nil {
		return fmt.Errorf("strip residual markers: %w", err)
	}

	// 3. Marker removal can re-trigger gofmt alignment changes (e.g. struct
	//    fields that were in different alignment groups merge once their
	//    separating comment is gone). gofmt now so the final source is
	//    formatter-clean for both gofmt and goimports.
	if err := runOne(root, "gofmt", "-w", "."); err != nil {
		return fmt.Errorf("final gofmt: %w", err)
	}

	// 4. Final tidy drops cmd/init's transitive deps from the root go.mod.
	if err := runOne(root, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("final go mod tidy: %w", err)
	}
	return nil
}

// printSummary prints a friendly end-of-run banner. It MUST be called from
// main BEFORE cmd/init's source disappears; the binary is fine, but the
// summary text relies on the parsed Selection.
func printSummary(modulePath string, kept, removed []string) {
	fmt.Println()
	fmt.Println("✓ Bootstrap complete")
	if modulePath != "" {
		fmt.Printf("  Module: %s\n", modulePath)
	}
	if len(kept) > 0 {
		fmt.Println("  Features kept:    " + strings.Join(kept, ", "))
	}
	if len(removed) > 0 {
		fmt.Println("  Features removed: " + strings.Join(removed, ", "))
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  make migrate-up   # apply Mongo migrations")
	fmt.Println("  make run          # start the HTTP server")
	for _, k := range kept {
		if k == "kafka" || k == "sqs" {
			fmt.Println("  make worker       # start the message consumer")
			break
		}
	}
	fmt.Println()
}

// runOne is a thin wrapper used by selfDestruct's final tidy step. Defined
// here (not in actions.go) so destruct can be unit-tested independently.
func runOne(dir, bin string, args ...string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not in PATH: %w", bin, err)
	}
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
