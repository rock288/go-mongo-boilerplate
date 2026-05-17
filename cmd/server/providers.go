package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/rock288/go-mongo-boilerplate/internal/middleware"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/health"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/httpserver"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

// ServerApp bundles long-lived dependencies returned by Wire.
type ServerApp struct {
	Config   *config.Config
	Logger   *slog.Logger
	Mongo    *mongo.Client
	Server   *httpserver.Server
	Router   *gin.Engine
	Health   *health.Registry
	Shutdown observability.Shutdown
}

func ProvideServerConfig(c *config.Config) config.ServerConfig               { return c.Server }
func ProvideMongoConfig(c *config.Config) config.MongoConfig                 { return c.Mongo }
func ProvideLoggerConfig(c *config.Config) config.LoggerConfig               { return c.Logger }
func ProvideObservabilityConfig(c *config.Config) config.ObservabilityConfig { return c.Observability }
func ProvideCORSConfig(c *config.Config) config.CORSConfig                   { return c.CORS }
func ProvideRateLimitConfig(c *config.Config) config.RateLimitConfig         { return c.RateLimit }

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
	uh *user.UserHandler,
	rh *role.RoleHandler,
) *gin.Engine {
	e := s.Engine()
	e.Use(
		otelgin.Middleware(cfg.Observability.ServiceName),
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

	api := e.Group("/api/v1")
	user.RegisterRoutes(api, uh)
	role.RegisterRoutes(api, rh)
	return e
}
