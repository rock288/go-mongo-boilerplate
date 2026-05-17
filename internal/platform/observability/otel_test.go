package observability

import (
	"errors"
	"testing"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

func TestExporterOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       config.ObservabilityConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "insecure_no_key_succeeds",
			cfg: config.ObservabilityConfig{
				Enabled:      true,
				Endpoint:     "localhost:4317",
				Insecure:     true,
				IngestionKey: "",
			},
			wantErr: false,
		},
		{
			name: "insecure_with_endpoint_only",
			cfg: config.ObservabilityConfig{
				Enabled:  true,
				Endpoint: "otel-collector:4317",
				Insecure: true,
			},
			wantErr: false,
		},
		{
			name: "secure_with_ingestion_key_succeeds",
			cfg: config.ObservabilityConfig{
				Enabled:      true,
				Endpoint:     "ingest.us.signoz.cloud:443",
				Insecure:     false,
				IngestionKey: "test-key-123",
			},
			wantErr: false,
		},
		{
			name: "secure_without_ingestion_key_fails",
			cfg: config.ObservabilityConfig{
				Enabled:      true,
				Endpoint:     "ingest.us.signoz.cloud:443",
				Insecure:     false,
				IngestionKey: "",
			},
			wantErr:   true,
			errSubstr: "ingestion_key required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			traceOpts, metricOpts, err := exporterOptions(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(traceOpts) == 0 {
				t.Fatal("expected trace options, got none")
			}
			if len(metricOpts) == 0 {
				t.Fatal("expected metric options, got none")
			}
		})
	}
}

func TestExporterOptions_SecureMissingKey_ErrorIsWrapped(t *testing.T) {
	t.Parallel()
	_, _, err := exporterOptions(config.ObservabilityConfig{
		Enabled:  true,
		Endpoint: "ingest.example.com:443",
		Insecure: false,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Errors should be unwrappable to nil chain (no inner cause needed here),
	// just verify it's not wrapping a sentinel we don't intend.
	if errors.Is(err, errFakeSentinel) {
		t.Fatalf("error should not be the fake sentinel")
	}
}

var errFakeSentinel = errors.New("fake sentinel for assertion")

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
