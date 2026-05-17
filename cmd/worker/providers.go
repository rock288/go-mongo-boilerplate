package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/health"
	platformkafka "github.com/rock288/go-mongo-boilerplate/internal/platform/kafka"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

// MainClient wraps the primary consumer client.
type MainClient struct{ *kgo.Client }

// RetryClient wraps the retry consumer client (different group).
type RetryClient struct{ *kgo.Client }

// ProducerClient wraps the producer used for retry/DLQ republish.
type ProducerClient struct{ *kgo.Client }

// WorkerApp bundles dependencies needed to consume Kafka events.
type WorkerApp struct {
	Config       *config.Config
	Logger       *slog.Logger
	MainClient   MainClient
	RetryClient  RetryClient
	Producer     platformkafka.Producer
	Handler      platformkafka.MessageHandler
	Health       *health.Registry
	KafkaChecker *health.KafkaChecker
	Shutdown     observability.Shutdown
}

func ProvideKafkaConfig(c *config.Config) config.KafkaConfig                 { return c.Kafka }
func ProvideLoggerConfig(c *config.Config) config.LoggerConfig               { return c.Logger }
func ProvideObservabilityConfig(c *config.Config) config.ObservabilityConfig { return c.Observability }
func ProvideWorkerConfig(c *config.Config) config.WorkerConfig               { return c.Worker }

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

// ProvideHealthRegistry registers the Kafka heartbeat checker.
func ProvideHealthRegistry(k *health.KafkaChecker) *health.Registry {
	r := health.NewRegistry()
	r.Register(k)
	return r
}

// Run starts both consumer loops and the worker health HTTP listener.
// Two-phase shutdown: ctx cancel → consumer exits, in-flight handlers drain
// up to cfg.Worker.ShutdownTimeout, then offsets commit and clients close.
func (w *WorkerApp) Run(ctx context.Context) error {
	w.Health.Start(ctx)

	var wg sync.WaitGroup
	errs := make(chan error, 3)

	healthSrv := w.startHealthServer(ctx)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- platformkafka.Consume(ctx, w.MainClient.Client, w.Handler, w.Logger, platformkafka.ConsumeOptions{
			Consumer:  w.Config.Kafka.Consumer,
			Producer:  w.Producer,
			Heartbeat: w.KafkaChecker.Heartbeat,
		})
	}()
	go func() {
		defer wg.Done()
		errs <- platformkafka.ConsumeRetry(ctx, w.RetryClient.Client, w.Producer, w.Config.Kafka.Consumer, w.Logger, w.KafkaChecker.Heartbeat)
	}()

	wg.Wait()
	close(errs)

	if healthSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = healthSrv.Shutdown(shutdownCtx)
		cancel()
	}

	var firstErr error
	for e := range errs {
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
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
