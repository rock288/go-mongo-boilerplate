package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/sync/errgroup"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/health"
	platformkafka "github.com/rock288/go-mongo-boilerplate/internal/platform/kafka"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
	platformsqs "github.com/rock288/go-mongo-boilerplate/internal/platform/sqs"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

// MainClient wraps the primary consumer client.
type MainClient struct{ *kgo.Client }

// RetryClient wraps the retry consumer client (different group).
type RetryClient struct{ *kgo.Client }

// ProducerClient wraps the producer used for retry/DLQ republish.
type ProducerClient struct{ *kgo.Client }

// SQSClient wraps the SQS API for Wire (distinguishable from raw SQSAPI).
type SQSClient struct{ platformsqs.SQSAPI }

// WorkerApp bundles dependencies needed to consume Kafka + SQS events.
type WorkerApp struct {
	Config       *config.Config
	Logger       *slog.Logger
	MainClient   MainClient
	RetryClient  RetryClient
	Producer     platformkafka.Producer
	Handler      platformkafka.MessageHandler
	SQSClient    SQSClient
	SQSProducer  platformsqs.Producer
	SQSHandler   platformsqs.MessageHandler // may be nil — see ProvideSQSHandler
	SQSChecker   *health.SQSChecker
	Health       *health.Registry
	KafkaChecker *health.KafkaChecker
	Shutdown     observability.Shutdown
}

func ProvideKafkaConfig(c *config.Config) config.KafkaConfig                 { return c.Kafka }
func ProvideLoggerConfig(c *config.Config) config.LoggerConfig               { return c.Logger }
func ProvideObservabilityConfig(c *config.Config) config.ObservabilityConfig { return c.Observability }
func ProvideWorkerConfig(c *config.Config) config.WorkerConfig               { return c.Worker }
func ProvideSQSConfig(c *config.Config) config.SQSConfig                     { return c.SQS }

// ProvideObservability initialises OTel for the worker process.
func ProvideObservability(cfg config.ObservabilityConfig) (observability.Shutdown, func(), error) {
	shutdown, err := observability.Init(context.Background(), cfg)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}
	return shutdown, cleanup, nil
}

// ProvideMainConsumer creates the primary consumer.
func ProvideMainConsumer(cfg config.KafkaConfig) (MainClient, func(), error) {
	client, err := platformkafka.NewConsumer(cfg)
	if err != nil {
		return MainClient{}, nil, err
	}
	return MainClient{Client: client}, platformkafka.CleanupClient(client), nil
}

// ProvideRetryConsumer creates the retry consumer bound to the .retry topic.
func ProvideRetryConsumer(cfg config.KafkaConfig) (RetryClient, func(), error) {
	suffix := cfg.Consumer.RetrySuffix
	if suffix == "" {
		suffix = ".retry"
	}
	group := cfg.GroupID + "-retry"
	client, err := platformkafka.NewConsumerForTopics(cfg, group, cfg.Topic+suffix)
	if err != nil {
		return RetryClient{}, nil, err
	}
	return RetryClient{Client: client}, platformkafka.CleanupClient(client), nil
}

// ProvideProducerClient creates the producer used for republish.
func ProvideProducerClient(cfg config.KafkaConfig) (ProducerClient, func(), error) {
	client, err := platformkafka.NewProducer(cfg)
	if err != nil {
		return ProducerClient{}, nil, err
	}
	return ProducerClient{Client: client}, platformkafka.CleanupClient(client), nil
}

// ProvideProducer wraps a ProducerClient as the platform Producer interface.
func ProvideProducer(c ProducerClient) platformkafka.Producer {
	return platformkafka.NewProducerWrapper(c.Client)
}

// ProvideMessageHandler binds the user.EventHandler into the kafka contract.
func ProvideMessageHandler(h *user.EventHandler) platformkafka.MessageHandler {
	return h
}

// ProvideKafkaChecker exposes the worker's poll heartbeat for /readyz.
func ProvideKafkaChecker() *health.KafkaChecker {
	return health.NewKafkaChecker(30 * time.Second)
}

// ProvideSQSChecker exposes the SQS consumer's poll heartbeat for /readyz.
// Registered as SeverityDegraded — see internal/platform/health/sqs.go.
func ProvideSQSChecker() *health.SQSChecker {
	return health.NewSQSChecker(30 * time.Second)
}

// ProvideSQSClient builds the SDK v2 SQS client. When cfg.Consumer.QueueURL
// is empty, the client is still constructed (cheap) but Run() will skip the
// SQS consumer goroutines.
func ProvideSQSClient(cfg config.SQSConfig) (SQSClient, func(), error) {
	c, err := platformsqs.NewClient(context.Background(), cfg)
	if err != nil {
		return SQSClient{}, nil, err
	}
	return SQSClient{SQSAPI: c}, func() {}, nil
}

// ProvideSQSProducer wraps SQSClient as the Producer interface.
func ProvideSQSProducer(c SQSClient) platformsqs.Producer {
	return platformsqs.NewProducer(c.SQSAPI)
}

// ProvideSQSHandler returns the SQS MessageHandler the worker should use.
// Boilerplate has no SQS-consuming feature; this returns nil so Run() skips
// the SQS goroutines. To enable: add `internal/<feature>/` exporting an
// sqs.MessageHandler, return it here, and add Wire entries — see CLAUDE.md
// "Adding an SQS-consuming feature".
func ProvideSQSHandler() platformsqs.MessageHandler {
	return nil
}

// ProvideHealthRegistry registers Kafka + (when configured) SQS heartbeat checkers.
func ProvideHealthRegistry(k *health.KafkaChecker, s *health.SQSChecker, cfg *config.Config) *health.Registry {
	r := health.NewRegistry()
	r.Register(k)
	if cfg.SQS.Consumer.QueueURL != "" {
		r.Register(s)
	}
	return r
}

// Run starts Kafka (and optionally SQS) consumer loops + the worker health
// HTTP listener. Each consumer is a goroutine inside an errgroup — first
// non-nil return cancels the shared context, so a fatal failure on one broker
// stops all consumers.
//
// SQS goroutines are skipped when cfg.SQS.Consumer.QueueURL is empty or no
// SQSHandler was provided (boilerplate ships infra only — feature owners
// wire their handler in via Wire).
func (w *WorkerApp) Run(ctx context.Context) error {
	w.Health.Start(ctx)
	healthSrv := w.startHealthServer(ctx)

	g, gctx := errgroup.WithContext(ctx)

	// Kafka main consumer.
	g.Go(func() error {
		return platformkafka.Consume(gctx, w.MainClient.Client, w.Handler, w.Logger, platformkafka.ConsumeOptions{
			Consumer:  w.Config.Kafka.Consumer,
			Producer:  w.Producer,
			Heartbeat: w.KafkaChecker.Heartbeat,
		})
	})
	// Kafka retry consumer.
	g.Go(func() error {
		return platformkafka.ConsumeRetry(gctx, w.RetryClient.Client, w.Producer, w.Config.Kafka.Consumer, w.Logger, w.KafkaChecker.Heartbeat)
	})

	// SQS — opt-in. Skip if not configured or no handler wired.
	if w.Config.SQS.Consumer.QueueURL != "" && w.SQSHandler != nil {
		g.Go(func() error {
			return platformsqs.Consume(gctx, w.SQSClient, w.SQSHandler, w.Logger, platformsqs.ConsumeOptions{
				Consumer:  w.Config.SQS.Consumer,
				Producer:  w.SQSProducer,
				Heartbeat: w.SQSChecker.Heartbeat,
			})
		})
		g.Go(func() error {
			return platformsqs.ConsumeRetry(gctx, w.SQSClient, w.SQSProducer, w.Config.SQS.Consumer, w.Logger, w.SQSChecker.Heartbeat)
		})
	} else {
		w.Logger.Info("SQS consumer skipped — queue_url empty or handler nil",
			slog.String("queue_url", w.Config.SQS.Consumer.QueueURL))
	}

	err := g.Wait()

	if healthSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = healthSrv.Shutdown(shutdownCtx)
		cancel()
	}
	return err
}

func (w *WorkerApp) startHealthServer(ctx context.Context) *http.Server {
	port := w.Config.Worker.HealthPort
	if port == 0 {
		return nil
	}
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.GET("/healthz", w.Health.Liveness)
	e.GET("/readyz", w.Health.Readiness)
	e.GET("/internal/readyz", w.Health.DetailedReadiness)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			w.Logger.Error("worker health server failed", slog.String("error", err.Error()))
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv
}
