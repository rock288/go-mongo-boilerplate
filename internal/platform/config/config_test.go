package config

import (
	"os"
	"testing"
)

func TestValidate_RejectsWildcardOriginsWithCredentials(t *testing.T) {
	c := &Config{
		CORS: CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for '*' + credentials")
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

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
