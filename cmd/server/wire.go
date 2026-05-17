//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/database"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/httpserver"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/logger"
	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

var platformSet = wire.NewSet(
	config.Load,
	ProvideServerConfig,
	ProvideMongoConfig,
	ProvideLoggerConfig,
	ProvideObservabilityConfig,
	ProvideCORSConfig,
	ProvideRateLimitConfig,
	ProvideObservability,
	logger.New,
	database.NewMongoClient,
	database.NewDatabase,
	httpserver.New,
	ProvideHealthRegistry,
)

func InitializeServer(cfgPath string) (*ServerApp, func(), error) {
	wire.Build(
		platformSet,
		user.ProviderSet,
		role.ProviderSet,
		ProvideRouter,
		wire.Struct(new(ServerApp), "*"),
	)
	return nil, nil, nil
}
