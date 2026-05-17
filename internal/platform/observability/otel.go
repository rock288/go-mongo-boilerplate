package observability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"google.golang.org/grpc/credentials"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// Shutdown flushes and shuts down all OTel providers.
type Shutdown func(context.Context) error

// Build version metadata, settable via ldflags from main packages.
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
)

// Init wires TracerProvider + MeterProvider with OTLP/gRPC exporter.
// Returns a no-op shutdown when cfg.Enabled is false.
func Init(ctx context.Context, cfg config.ObservabilityConfig) (Shutdown, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(BuildVersion),
			semconv.DeploymentEnvironmentName(cfg.Environment),
			semconv.ServiceInstanceID(BuildCommit),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	traceOpts, metricOpts, err := exporterOptions(cfg)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(
		traceExp,
		sdktrace.WithMaxQueueSize(2048),
		sdktrace.WithBatchTimeout(5*time.Second),
		sdktrace.WithExportTimeout(30*time.Second),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithResource(res),
	)

	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("otlp metric exporter: %w", err)
	}
	reader := sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(60*time.Second))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer: %w", err))
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter: %w", err))
		}
		return errors.Join(errs...)
	}
	return shutdown, nil
}

func exporterOptions(cfg config.ObservabilityConfig) ([]otlptracegrpc.Option, []otlpmetricgrpc.Option, error) {
	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.Endpoint)}

	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
		return traceOpts, metricOpts, nil
	}

	if cfg.IngestionKey == "" {
		return nil, nil, fmt.Errorf("observability: ingestion_key required for non-insecure exporter")
	}
	headers := map[string]string{"signoz-ingestion-key": cfg.IngestionKey}
	tlsCreds := credentials.NewClientTLSFromCert(nil, "")
	traceOpts = append(traceOpts,
		otlptracegrpc.WithHeaders(headers),
		otlptracegrpc.WithTLSCredentials(tlsCreds),
	)
	metricOpts = append(metricOpts,
		otlpmetricgrpc.WithHeaders(headers),
		otlpmetricgrpc.WithTLSCredentials(tlsCreds),
	)
	return traceOpts, metricOpts, nil
}
