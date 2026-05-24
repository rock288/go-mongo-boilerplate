package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeptAndRemoved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		nk, ns, no, nsa       bool
		wantKept, wantRemoved []string
	}{
		{
			name:        "all-kept",
			wantKept:    []string{"kafka", "sqs", "observability", "samples"},
			wantRemoved: nil,
		},
		{
			name:        "no-samples-only",
			nsa:         true,
			wantKept:    []string{"kafka", "sqs", "observability"},
			wantRemoved: []string{"samples"},
		},
		{
			name:        "no-brokers-cascade-worker",
			nk:          true,
			ns:          true,
			wantKept:    []string{"observability", "samples"},
			wantRemoved: []string{"kafka", "sqs", "worker (cascade)"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kept, removed := keptAndRemoved(tc.nk, tc.ns, tc.no, tc.nsa)
			if !sliceEq(kept, tc.wantKept) {
				t.Errorf("kept = %v, want %v", kept, tc.wantKept)
			}
			if !sliceEq(removed, tc.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

func TestSelfDestruct_RemovesCmdInit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd/init"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd/init/main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stub a go.mod so the final `go mod tidy` does not explode.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File with residual marker — must be cleaned. Use a valid Go source
	// so the gofmt step inside selfDestruct accepts it; the marker lives
	// inside a struct field comment that gofmt happily preserves until
	// stripAllResidualMarkers removes it.
	residual := filepath.Join(root, "x.go")
	body := "package x\n\ntype X struct {\n\t// feature:kafka:start\n\tA int\n\t// feature:kafka:end\n\tB int\n}\n"
	if err := os.WriteFile(residual, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := selfDestruct(root); err != nil {
		t.Fatalf("selfDestruct: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "cmd/init")); !os.IsNotExist(err) {
		t.Fatalf("cmd/init not removed: err=%v", err)
	}
	out, err := os.ReadFile(residual)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "feature:") {
		t.Fatalf("residual marker remains: %q", out)
	}
	if !strings.Contains(string(out), "A int") || !strings.Contains(string(out), "B int") {
		t.Fatalf("non-marker lines lost: %q", out)
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
