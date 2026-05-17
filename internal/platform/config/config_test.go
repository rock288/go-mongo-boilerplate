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

func TestValidate_RejectsWildcardOriginsWithCredentials(t *testing.T) {
	c := &Config{
		CORS: CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for '*' + credentials")
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

func TestValidate_AllowsCredentialsWithSpecificOrigin(t *testing.T) {
	c := &Config{
		CORS: CORSConfig{AllowedOrigins: []string{"https://app.example.com"}, AllowCredentials: true},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("specific origin + credentials should be valid, got: %v", err)
	}
}

func TestValidate_AllowsCredentialsFalseWithWildcard(t *testing.T) {
	c := &Config{
		CORS: CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: false},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("wildcard without credentials should be valid, got: %v", err)
	}
}

func TestLoad_ParsesYAMLAndAppliesEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"
	yamlBody := `
server:
  port: 8000
  mode: release
mongo:
  uri: "mongodb://localhost:27017"
  database: "test_db"
kafka:
  brokers: ["localhost:9092"]
  group_id: "test"
  topic: "events"
observability:
  enabled: false
cors:
  allowed_origins: ["http://localhost"]
  allow_credentials: false
`
	if err := writeFile(path, yamlBody); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_SERVER__PORT", "9999")
	t.Setenv("APP_MONGO__URI", "mongodb://override:27017")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("env override APP_SERVER__PORT=9999, got %d", cfg.Server.Port)
	}
	if cfg.Mongo.URI != "mongodb://override:27017" {
		t.Errorf("env override mongo URI, got %q", cfg.Mongo.URI)
	}
	if cfg.Mongo.Database != "test_db" {
		t.Errorf("yaml mongo database, got %q", cfg.Mongo.Database)
	}
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	if _, err := Load("/nonexistent/path/does-not-exist.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.yaml"
	if err := writeFile(path, "{not valid yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed YAML")
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
	if err := writeFile(path, yamlBody); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error for missing ingestion_key")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
