package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

var residualMarkerRe = regexp.MustCompile(`feature:[a-zA-Z0-9_-]+:(start|end)`)

// stripFeatureFromFile removes every block delimited by
// `feature:<name>:start` ... `feature:<name>:end` markers in path.
// Lines may use any comment style (//, #, <!-- -->) — match is substring-based.
// A missing file is treated as no-op (idempotent across features sharing manifests).
// Nested blocks of *other* features inside a stripped block are removed with it;
// nested blocks of the SAME feature are not supported (markers must not nest with themselves).
func stripFeatureFromFile(path, feature string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	startToken := fmt.Sprintf("feature:%s:start", feature)
	endToken := fmt.Sprintf("feature:%s:end", feature)

	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	// Collected non-block lines are appended to `kept` so we can drop the
	// blank line that immediately precedes a `:start` marker — otherwise
	// stripping marker blocks inside a goimports-formatted file leaves
	// double blank lines that re-violate the import-grouping rules.
	var kept []string
	flush := func() {
		for _, l := range kept {
			out.WriteString(l)
			out.WriteByte('\n')
		}
		kept = kept[:0]
	}

	inBlock := false
	lineNo := 0
	startLine := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		switch {
		case !inBlock && containsToken(line, startToken):
			inBlock = true
			startLine = lineNo
			// If the line immediately before the start marker was blank,
			// drop it so the stripped output doesn't grow extra blank lines.
			if len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
				kept = kept[:len(kept)-1]
			}
			flush()
		case inBlock && containsToken(line, endToken):
			inBlock = false
		case inBlock:
			// drop
		default:
			kept = append(kept, line)
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	if inBlock {
		return fmt.Errorf("%s: unclosed feature:%s block opened at line %d", path, feature, startLine)
	}

	// Preserve trailing-newline behaviour: if original had no terminating newline,
	// strip the one we appended on the last write.
	result := out.Bytes()
	if len(data) > 0 && data[len(data)-1] != '\n' && len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	// Collapse 3+ consecutive newlines into 2 — gofmt/goimports treat double
	// blank lines as a violation, and removing a marker block can leave
	// stacked blanks when both surrounding sides were blank-padded.
	result = collapseBlankRuns(result)

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return os.WriteFile(path, result, info.Mode().Perm())
}

// stripAllResidualMarkers walks root and removes any leftover lines containing
// `feature:*:start` or `feature:*:end` from text files. Called by self-destruct
// to guarantee `grep -r "feature:" .` is clean post-init.
// Binary files and the project's own VCS / module-cache dirs are skipped.
func stripAllResidualMarkers(root string) error {
	skipDirs := map[string]bool{".git": true, "node_modules": true, "vendor": true, "bin": true}
	return fs.WalkDir(os.DirFS(root), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		full := joinRoot(root, rel)
		data, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		if !residualMarkerRe.Match(data) {
			return nil
		}
		// Drop matching lines only; keep everything else byte-for-byte.
		var out bytes.Buffer
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if residualMarkerRe.MatchString(line) {
				continue
			}
			out.WriteString(line)
			out.WriteByte('\n')
		}
		if scanner.Err() != nil {
			return nil
		}
		result := out.Bytes()
		if len(data) > 0 && data[len(data)-1] != '\n' && len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		info, statErr := os.Stat(full)
		if statErr != nil {
			return nil
		}
		return os.WriteFile(full, result, info.Mode().Perm())
	})
}

func containsToken(line, token string) bool {
	return bytes.Contains([]byte(line), []byte(token))
}

// collapseBlankRuns rewrites runs of 3+ consecutive newlines as exactly two,
// which is the maximum gofmt / goimports allow between top-level decls.
func collapseBlankRuns(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b))
	streak := 0
	for _, c := range b {
		if c == '\n' {
			streak++
			if streak <= 2 {
				out.WriteByte(c)
			}
		} else {
			streak = 0
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}

func joinRoot(root, rel string) string {
	if rel == "." {
		return root
	}
	return root + string(os.PathSeparator) + rel
}
