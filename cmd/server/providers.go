package main

import (
	// feature:observability:start
	"context"
	// feature:observability:end
	"log/slog"
	"net/http"

	// feature:observability:start
	"time"
	// feature:observability:end

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"

	// feature:observability:start
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	// feature:observability:end

	"github.com/rock288/go-mongo-boilerplate/internal/middleware"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/health"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/httpserver"

	// feature:observability:start
	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
	// feature:observability:end
	// feature:samples:start
	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
	// feature:samples:end
)

// ServerApp bundles long-lived dependencies returned by Wire.
type ServerApp struct {
	Config *config.Config
	Logger *slog.Logger
	Mongo  *mongo.Client
	Server *httpserver.Server
	Router *gin.Engine
	Health *health.Registry
	// feature:observability:start
	Shutdown observability.Shutdown
	// feature:observability:end
}

func ProvideServerConfig(c *config.Config) config.ServerConfig { return c.Server }
func ProvideMongoConfig(c *config.Config) config.MongoConfig   { return c.Mongo }
func ProvideLoggerConfig(c *config.Config) config.LoggerConfig { return c.Logger }

// feature:observability:start
func ProvideObservabilityConfig(c *config.Config) config.ObservabilityConfig {
	return c.Observability
}

// feature:observability:end
func ProvideCORSConfig(c *config.Config) config.CORSConfig           { return c.CORS }
func ProvideRateLimitConfig(c *config.Config) config.RateLimitConfig { return c.RateLimit }

// feature:observability:start

// ProvideObservability initialises OTel and returns the shutdown func.
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

// feature:observability:end

// ProvideHealthRegistry wires the registry with the standard checkers.
func ProvideHealthRegistry(mongoClient *mongo.Client) *health.Registry {
	r := health.NewRegistry()
	r.Register(health.NewMongoChecker(mongoClient))
	return r
}

// ProvideRouter wires HTTP routes onto the gin engine of the server.
// otelgin must run before Recovery so panic events attach to the active span.
func ProvideRouter(
	s *httpserver.Server,
	logger *slog.Logger,
	cfg *config.Config,
	registry *health.Registry,
	// feature:samples:start
	uh *user.UserHandler,
	rh *role.RoleHandler,
	// feature:samples:end
) *gin.Engine {
	e := s.Engine()
	e.Use(
		// feature:observability:start
		otelgin.Middleware(cfg.Observability.ServiceName),
		// feature:observability:end
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.RequestLogger(logger),
		middleware.RateLimit(cfg.RateLimit),
		middleware.BodyLimit(cfg.Server.BodyLimitBytes),
		middleware.Timeout(cfg.Server.HandlerTimeout),
		middleware.CORS(cfg.CORS),
	)

	e.GET("/healthz", registry.Liveness)
	e.GET("/readyz", registry.Readiness)
	e.GET("/internal/readyz", registry.DetailedReadiness)

	e.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "pong"})
	})

	// feature:samples:start
	api := e.Group("/api/v1")
	user.RegisterRoutes(api, uh)
	role.RegisterRoutes(api, rh)
	// feature:samples:end
	return e
}
