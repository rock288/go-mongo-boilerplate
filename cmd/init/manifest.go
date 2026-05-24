package main

// Feature describes an opt-out toggle: paths to delete wholesale and files
// that carry feature:<name>:start/end marker blocks to strip.
type Feature struct {
	Name        string
	DeletePaths []string
	MarkerFiles []string
}

// allMarkerFiles is every file that contains markers for ANY feature.
// The stripper is a no-op when the named feature has no block in a file, so
// passing the union to every feature is safe and avoids per-feature drift.
// Generated files (wire_gen.go, mocks/*) are NOT listed — they are recreated
// by `make wire`/`make mocks` after strip.
var allMarkerFiles = []string{
	"cmd/server/wire.go",
	"cmd/server/providers.go",
	"cmd/server/main.go",
	"cmd/worker/wire.go",
	"cmd/worker/providers.go",
	"cmd/worker/main.go",
	"internal/platform/config/config.go",
	"internal/platform/database/mongo.go",
	"internal/user/wire.go",
	"config/config.yaml",
	".mockery.yaml",
	"Makefile",
	"docker-compose.yml",
	".env.example",
	"README.md",
	"CLAUDE.md",
}

// Features is the canonical list of opt-out toggles.
var Features = []Feature{
	{
		Name: "kafka",
		DeletePaths: []string{
			"internal/platform/kafka",
			"internal/user/event_handler.go",
			"internal/user/event_handler_test.go",
			"internal/platform/health/kafka_checker.go",
			"internal/platform/health/kafka_checker_test.go",
		},
		MarkerFiles: allMarkerFiles,
	},
	{
		Name: "sqs",
		DeletePaths: []string{
			"internal/platform/sqs",
			"internal/platform/health/sqs_checker.go",
			"internal/platform/health/sqs_checker_test.go",
			"docker-compose.localstack.yml",
			"scripts/sqs-create-dev-queues.sh",
		},
		MarkerFiles: allMarkerFiles,
	},
	{
		Name: "observability",
		DeletePaths: []string{
			"internal/platform/observability",
			"internal/platform/database/mongo_otel.go",
			"internal/platform/config/config_observability_test.go",
			"docker-compose.signoz.yml",
			"deploy/signoz",
		},
		MarkerFiles: allMarkerFiles,
	},
	{
		Name: "samples",
		DeletePaths: []string{
			"internal/user",
			"internal/role",
			"scripts/scaffold.sh",
		},
		MarkerFiles: allMarkerFiles,
	},
}

// WorkerMarkerFiles is the synthetic feature stripped only when both kafka
// and sqs are disabled (cascade). The DELETE of cmd/worker/ happens via the
// computeStripList → cascade branch in main.go.
var WorkerMarkerFiles = allMarkerFiles

// findFeature returns the manifest entry for name, or nil if not found.
func findFeature(name string) *Feature {
	for i := range Features {
		if Features[i].Name == name {
			return &Features[i]
		}
	}
	return nil
}

// computeStripList returns feature names to strip given opt-out flags.
// Order is deterministic for reproducible runs.
func computeStripList(noKafka, noSQS, noObs, noSamples bool) []string {
	var out []string
	if noKafka {
		out = append(out, "kafka")
	}
	if noSQS {
		out = append(out, "sqs")
	}
	if noObs {
		out = append(out, "observability")
	}
	if noSamples {
		out = append(out, "samples")
	}
	return out
}
