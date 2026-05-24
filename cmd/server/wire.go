//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/database"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/httpserver"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/logger"

	// feature:samples:start
	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
	// feature:samples:end
)

var platformSet = wire.NewSet(
	config.Load,
	ProvideServerConfig,
	ProvideMongoConfig,
	ProvideLoggerConfig,
	// feature:observability:start
	ProvideObservabilityConfig,
	// feature:observability:end
	ProvideCORSConfig,
	ProvideRateLimitConfig,
	// feature:observability:start
	ProvideObservability,
	// feature:observability:end
	logger.New,
	database.NewMongoClient,
	database.NewDatabase,
	httpserver.New,
	ProvideHealthRegistry,
)

func InitializeServer(cfgPath string) (*ServerApp, func(), error) {
	wire.Build(
		platformSet,
		// feature:samples:start
		user.ProviderSet,
		role.ProviderSet,
		// feature:samples:end
		ProvideRouter,
		wire.Struct(new(ServerApp), "*"),
	)
	return nil, nil, nil
}
