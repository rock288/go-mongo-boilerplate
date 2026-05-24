package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

// version is overridden at link time for release builds.
var version = "dev"

func main() {
	var (
		rootFlag       = flag.String("root", "../..", "project root (resolved relative to cwd; default assumes `cd cmd/init`)")
		nonInteractive = flag.Bool("non-interactive", false, "skip prompts; rely on --no-* flags")
		force          = flag.Bool("force", false, "proceed even when the git tree is dirty")
		noKafka        = flag.Bool("no-kafka", false, "strip the Kafka message broker")
		noSQS          = flag.Bool("no-sqs", false, "strip the AWS SQS message broker")
		noObs          = flag.Bool("no-observability", false, "strip the OpenTelemetry/SigNoz wiring (keeps base API)")
		noSamples      = flag.Bool("no-samples", false, "strip the user/role sample features")
		modulePath     = flag.String("module", "", "rewrite the module path (e.g. github.com/acme/myservice)")
		showVersion    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("init", version)
		return
	}

	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		log.Fatalf("resolve --root: %v", err)
	}

	if !*force && !isCleanGitTree(root) {
		log.Fatalf("dirty git tree at %s; commit/stash first or pass --force", root)
	}

	selected := computeStripList(*noKafka, *noSQS, *noObs, *noSamples)

	defaultModule := *modulePath
	if defaultModule == "" {
		if detected, err := detectCurrentModule(root); err == nil && detected != "" && detected != oldModule {
			defaultModule = detected
		} else {
			defaultModule = "github.com/yourorg/your-service"
		}
	}

	if !*nonInteractive {
		sel, err := runPrompt(defaultModule)
		if err != nil {
			log.Fatalf("prompt: %v", err)
		}
		selected = stripListFromSelection(sel)
		*modulePath = sel.Module
		*noKafka = !sel.Keep["kafka"]
		*noSQS = !sel.Keep["sqs"]
	}

	// 1. Module rename first — touches source + go.mod before strip walks them.
	if *modulePath != "" && *modulePath != oldModule {
		if err := renameModule(root, *modulePath); err != nil {
			log.Fatalf("rename module: %v", err)
		}
	}

	// 2. Per-feature strip + delete.
	for _, name := range selected {
		feat := findFeature(name)
		if feat == nil {
			log.Fatalf("unknown feature %q", name)
		}
		if err := deletePaths(root, feat.DeletePaths); err != nil {
			log.Fatalf("delete %s: %v", name, err)
		}
		for _, f := range feat.MarkerFiles {
			if err := stripFeatureFromFile(filepath.Join(root, f), name); err != nil {
				log.Fatalf("strip %s from %s: %v", name, f, err)
			}
		}
	}

	// 3. Cascade: worker dies when both brokers go.
	if *noKafka && *noSQS {
		if err := deletePaths(root, []string{"cmd/worker"}); err != nil {
			log.Fatalf("cascade-delete cmd/worker: %v", err)
		}
		for _, f := range WorkerMarkerFiles {
			if err := stripFeatureFromFile(filepath.Join(root, f), "worker"); err != nil {
				log.Fatalf("strip worker from %s: %v", f, err)
			}
		}
	}

	moduleChanged := *modulePath != "" && *modulePath != oldModule
	if len(selected) == 0 && (!*noKafka || !*noSQS) && !moduleChanged {
		log.Println("nothing selected; skipping post-actions")
		return
	}

	// 4. Post-actions: wire → tidy → mockery → build.
	if err := runPostActions(root); err != nil {
		log.Fatalf("post-actions: %v", err)
	}

	// 5. Self-destruct — only reached when every prior step succeeded.
	kept, removed := keptAndRemoved(*noKafka, *noSQS, *noObs, *noSamples)
	if err := selfDestruct(root); err != nil {
		log.Printf("warning: self-destruct incomplete: %v", err)
		log.Println("  recover with: rm -rf cmd/init && git status")
	}
	printSummary(*modulePath, kept, removed)
}

// keptAndRemoved produces stable, deterministic kept/removed slices for the
// summary banner. Order matters for tests and human readability.
func keptAndRemoved(noKafka, noSQS, noObs, noSamples bool) (kept, removed []string) {
	all := []struct {
		name    string
		removed bool
	}{
		{"kafka", noKafka},
		{"sqs", noSQS},
		{"observability", noObs},
		{"samples", noSamples},
	}
	for _, f := range all {
		if f.removed {
			removed = append(removed, f.name)
		} else {
			kept = append(kept, f.name)
		}
	}
	if noKafka && noSQS {
		removed = append(removed, "worker (cascade)")
	}
	return kept, removed
}

// isCleanGitTree returns true when the working tree at root has no untracked
// or modified files. A missing git binary or non-repo path returns false to
// fail safe — users must pass --force to skip the guard.
func isCleanGitTree(root string) bool {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) == 0
}
