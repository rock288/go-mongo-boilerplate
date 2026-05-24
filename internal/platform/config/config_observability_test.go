package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestObservabilityConfig_LogValue_RedactsKey(t *testing.T) {
	cfg := ObservabilityConfig{
		Enabled:      true,
		Endpoint:     "ingest.signoz.cloud:443",
		IngestionKey: "super-secret-123",
		ServiceName:  "svc",
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("cfg", slog.Any("obs", cfg))

	out := buf.String()
	if strings.Contains(out, "super-secret-123") {
		t.Fatalf("ingestion_key leaked in log output: %s", out)
	}
	if !strings.Contains(out, `"ingestion_key":"***"`) {
		t.Fatalf("expected redacted key marker, got: %s", out)
	}
}

func TestValidate_RejectsMissingIngestionKeyWhenSecure(t *testing.T) {
	c := &Config{
		Observability: ObservabilityConfig{Enabled: true, Insecure: false, IngestionKey: ""},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for missing ingestion_key")
	}
	c.Observability.IngestionKey = "changeme"
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for placeholder ingestion_key")
	}
	c.Observability.IngestionKey = "valid-key"
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidate_AllowsInsecureWithoutKey(t *testing.T) {
	c := &Config{
		Observability: ObservabilityConfig{Enabled: true, Insecure: true, IngestionKey: ""},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("insecure local should not require key, got: %v", err)
	}
}

func TestValidate_RejectsPlaceholderKey(t *testing.T) {
	c := &Config{
		Observability: ObservabilityConfig{Enabled: true, Insecure: false, IngestionKey: "<your-key-here>"},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for placeholder key starting with '<'")
	}
}

func TestLoad_ValidationErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cfg.yaml"
	yamlBody := `
observability:
  enabled: true
  insecure: false
  ingestion_key: ""
`
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error for missing ingestion_key")
	}
}
