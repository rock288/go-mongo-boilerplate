package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Server        ServerConfig        `koanf:"server"`
	Mongo         MongoConfig         `koanf:"mongo"`
	Kafka         KafkaConfig         `koanf:"kafka"`
	SQS           SQSConfig           `koanf:"sqs"`
	Redis         RedisConfig         `koanf:"redis"`
	Logger        LoggerConfig        `koanf:"logger"`
	Observability ObservabilityConfig `koanf:"observability"`
	CORS          CORSConfig          `koanf:"cors"`
	Worker        WorkerConfig        `koanf:"worker"`
	RateLimit     RateLimitConfig     `koanf:"rate_limit"`
	HTTPClient    HTTPClientConfig    `koanf:"http_client"`
}

type ServerConfig struct {
	Port              int           `koanf:"port"`
	Mode              string        `koanf:"mode"`
	ShutdownTimeout   time.Duration `koanf:"shutdown_timeout"`
	ReadHeaderTimeout time.Duration `koanf:"read_header_timeout"`
	ReadTimeout       time.Duration `koanf:"read_timeout"`
	WriteTimeout      time.Duration `koanf:"write_timeout"`
	IdleTimeout       time.Duration `koanf:"idle_timeout"`
	MaxHeaderBytes    int           `koanf:"max_header_bytes"`
	BodyLimitBytes    int64         `koanf:"body_limit_bytes"`
	HandlerTimeout    time.Duration `koanf:"handler_timeout"`
	TrustedProxies    []string      `koanf:"trusted_proxies"`
}

type MongoConfig struct {
	URI      string `koanf:"uri"`
	Database string `koanf:"database"`
}

type KafkaConfig struct {
	Brokers  []string            `koanf:"brokers"`
	GroupID  string              `koanf:"group_id"`
	Topic    string              `koanf:"topic"`
	Consumer KafkaConsumerConfig `koanf:"consumer"`
}

type KafkaConsumerConfig struct {
	MaxRetries         int           `koanf:"max_retries"`
	RetrySuffix        string        `koanf:"retry_suffix"`
	DLQSuffix          string        `koanf:"dlq_suffix"`
	RetryBackoffBase   time.Duration `koanf:"retry_backoff_base"`
	RetryBackoffMax    time.Duration `koanf:"retry_backoff_max"`
	DLQMaxPayloadBytes int           `koanf:"dlq_max_payload_bytes"`
}

// SQSConfig holds AWS SQS connection + consumer settings. Set
// Consumer.QueueURL to enable the SQS consumer; leave empty to disable.
type SQSConfig struct {
	Region   string            `koanf:"region"`
	Endpoint string            `koanf:"endpoint"` // override for LocalStack dev; empty = SDK default
	Consumer SQSConsumerConfig `koanf:"consumer"`
}

// SQSConsumerConfig mirrors KafkaConsumerConfig with SQS-specific knobs.
// MessageDelaySeconds (used by retry path) is capped at 900s by AWS;
// RetryBackoffMax above this is silently clamped — see sqs.SQSMaxDelay.
type SQSConsumerConfig struct {
	QueueURL                 string        `koanf:"queue_url"`
	MaxRetries               int           `koanf:"max_retries"`
	RetrySuffix              string        `koanf:"retry_suffix"`
	DLQSuffix                string        `koanf:"dlq_suffix"`
	VisibilityTimeoutSeconds int32         `koanf:"visibility_timeout_seconds"`
	WaitTimeSeconds          int32         `koanf:"wait_time_seconds"`
	MaxMessages              int32         `koanf:"max_messages"`
	RetryBackoffBase         time.Duration `koanf:"retry_backoff_base"`
	RetryBackoffMax          time.Duration `koanf:"retry_backoff_max"`
	DLQMaxPayloadBytes       int           `koanf:"dlq_max_payload_bytes"`
}

type RedisConfig struct {
	Addr string `koanf:"addr"`
}

type LoggerConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// ObservabilityConfig controls OpenTelemetry exporter wiring.
// IngestionKey is redacted via slog.LogValuer when logged.
type ObservabilityConfig struct {
	Enabled      bool    `koanf:"enabled"`
	Endpoint     string  `koanf:"endpoint"`
	Insecure     bool    `koanf:"insecure"`
	IngestionKey string  `koanf:"ingestion_key"`
	ServiceName  string  `koanf:"service_name"`
	Environment  string  `koanf:"environment"`
	SampleRatio  float64 `koanf:"sample_ratio"`
	PublicFacing bool    `koanf:"public_facing"`
}

// LogValue redacts IngestionKey when the struct is logged via slog.
func (o ObservabilityConfig) LogValue() slog.Value {
	masked := ""
	if o.IngestionKey != "" {
		masked = "***"
	}
	return slog.GroupValue(
		slog.Bool("enabled", o.Enabled),
		slog.String("endpoint", o.Endpoint),
		slog.Bool("insecure", o.Insecure),
		slog.String("ingestion_key", masked),
		slog.String("service_name", o.ServiceName),
		slog.String("environment", o.Environment),
		slog.Float64("sample_ratio", o.SampleRatio),
		slog.Bool("public_facing", o.PublicFacing),
	)
}

type CORSConfig struct {
	AllowedOrigins   []string `koanf:"allowed_origins"`
	AllowedMethods   []string `koanf:"allowed_methods"`
	AllowedHeaders   []string `koanf:"allowed_headers"`
	AllowCredentials bool     `koanf:"allow_credentials"`
	MaxAgeSeconds    int      `koanf:"max_age_seconds"`
}

type WorkerConfig struct {
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
	HealthPort      int           `koanf:"health_port"`
}

type RateLimitConfig struct {
	PerIP         RateBucket    `koanf:"per_ip"`
	Global        RateBucket    `koanf:"global"`
	MapMaxEntries int           `koanf:"map_max_entries"`
	MapTTL        time.Duration `koanf:"map_ttl"`
}

type RateBucket struct {
	RPS   float64 `koanf:"rps"`
	Burst int     `koanf:"burst"`
}

type HTTPClientConfig struct {
	Timeout      time.Duration `koanf:"timeout"`
	MaxIdleConns int           `koanf:"max_idle_conns"`
	MaxRedirects int           `koanf:"max_redirects"`
}

// Load reads config from a YAML file then overlays env vars prefixed APP_.
// Env var nesting uses double-underscore (APP_SERVER__PORT → server.port).
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, err
	}

	envProvider := env.Provider(".", env.Opt{
		Prefix: "APP_",
		TransformFunc: func(key, value string) (string, any) {
			normalized := strings.ReplaceAll(
				strings.ToLower(strings.TrimPrefix(key, "APP_")),
				"__",
				".",
			)
			return normalized, value
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, err
	}

	var c Config
	if err := k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Tag: "koanf", DecoderConfig: nil}); err != nil {
		return nil, err
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate enforces invariants that prevent unsafe boot states.
func (c *Config) Validate() error {
	if c.Observability.Enabled && !c.Observability.Insecure {
		key := strings.TrimSpace(c.Observability.IngestionKey)
		if key == "" || key == "changeme" || strings.HasPrefix(key, "<") {
			return fmt.Errorf("observability: ingestion_key required when insecure=false")
		}
	}
	if c.CORS.AllowCredentials {
		for _, o := range c.CORS.AllowedOrigins {
			if o == "*" {
				return fmt.Errorf("cors: allow_credentials=true incompatible with origin '*'")
			}
		}
	}
	return nil
}
