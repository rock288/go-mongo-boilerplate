package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestStripFeatureFromFile_SingleBlockGo(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"package x",
		"",
		"// feature:kafka:start",
		"import _ \"github.com/twmb/franz-go/pkg/kgo\"",
		"// feature:kafka:end",
		"",
		"func main() {}",
		"",
	}, "\n")
	path := writeTempFile(t, "x.go", src)

	if err := stripFeatureFromFile(path, "kafka"); err != nil {
		t.Fatalf("strip: %v", err)
	}
	got := read(t, path)
	if strings.Contains(got, "kafka") {
		t.Fatalf("kafka still present:\n%s", got)
	}
	if !strings.Contains(got, "package x") || !strings.Contains(got, "func main()") {
		t.Fatalf("non-feature code lost:\n%s", got)
	}
}

func TestStripFeatureFromFile_MultipleBlocksSameFeature(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"a",
		"// feature:k:start",
		"X",
		"// feature:k:end",
		"b",
		"// feature:k:start",
		"Y",
		"// feature:k:end",
		"c",
		"",
	}, "\n")
	path := writeTempFile(t, "f.go", src)

	if err := stripFeatureFromFile(path, "k"); err != nil {
		t.Fatalf("strip: %v", err)
	}
	got := read(t, path)
	want := "a\nb\nc\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripFeatureFromFile_CommentStyles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, start, end string
	}{
		{"slash", "// feature:o:start", "// feature:o:end"},
		{"hash", "# feature:o:start", "# feature:o:end"},
		{"html", "<!-- feature:o:start -->", "<!-- feature:o:end -->"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "before\n" + tc.start + "\nINSIDE\n" + tc.end + "\nafter\n"
			path := writeTempFile(t, "f", src)
			if err := stripFeatureFromFile(path, "o"); err != nil {
				t.Fatalf("strip: %v", err)
			}
			got := read(t, path)
			if got != "before\nafter\n" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestStripFeatureFromFile_OnlyTargetFeatureRemoved(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"x",
		"// feature:sqs:start",
		"SQS",
		"// feature:sqs:end",
		"// feature:kafka:start",
		"KAFKA",
		"// feature:kafka:end",
		"y",
		"",
	}, "\n")
	path := writeTempFile(t, "f.go", src)

	if err := stripFeatureFromFile(path, "kafka"); err != nil {
		t.Fatalf("strip: %v", err)
	}
	got := read(t, path)
	if strings.Contains(got, "KAFKA") {
		t.Fatalf("kafka content still present:\n%s", got)
	}
	if !strings.Contains(got, "SQS") {
		t.Fatalf("sqs block was incorrectly stripped:\n%s", got)
	}
}

func TestStripFeatureFromFile_UnclosedBlockReturnsError(t *testing.T) {
	t.Parallel()
	src := "// feature:k:start\nbody\n"
	path := writeTempFile(t, "f.go", src)
	err := stripFeatureFromFile(path, "k")
	if err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("expected unclosed error, got %v", err)
	}
}

func TestStripFeatureFromFile_NoMarkersUnchanged(t *testing.T) {
	t.Parallel()
	src := "line1\nline2\n"
	path := writeTempFile(t, "f.go", src)
	if err := stripFeatureFromFile(path, "k"); err != nil {
		t.Fatalf("strip: %v", err)
	}
	if got := read(t, path); got != src {
		t.Fatalf("unexpected change: %q", got)
	}
}

func TestStripFeatureFromFile_MissingFileNoop(t *testing.T) {
	t.Parallel()
	if err := stripFeatureFromFile(filepath.Join(t.TempDir(), "absent"), "k"); err != nil {
		t.Fatalf("missing file should be no-op, got %v", err)
	}
}

func TestStripFeatureFromFile_EmptyFile(t *testing.T) {
	t.Parallel()
	path := writeTempFile(t, "f.go", "")
	if err := stripFeatureFromFile(path, "k"); err != nil {
		t.Fatalf("strip: %v", err)
	}
	if got := read(t, path); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestStripAllResidualMarkers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	if err := os.WriteFile(good, []byte("a\n// feature:zzz:start\nb\n// feature:zzz:end\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean := filepath.Join(root, "clean.go")
	if err := os.WriteFile(clean, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := stripAllResidualMarkers(root); err != nil {
		t.Fatalf("strip residual: %v", err)
	}

	if got := read(t, good); strings.Contains(got, "feature:") {
		t.Fatalf("residual markers remain: %q", got)
	}
	if got := read(t, clean); got != "untouched\n" {
		t.Fatalf("clean file mutated: %q", got)
	}
}
