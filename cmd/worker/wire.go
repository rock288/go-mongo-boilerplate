//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/logger"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

func InitializeWorker(cfgPath string) (*WorkerApp, func(), error) {
	wire.Build(
		config.Load,
		ProvideKafkaConfig,
		ProvideLoggerConfig,
		ProvideObservabilityConfig,
		ProvideSQSConfig,
		ProvideObservability,
		logger.New,
		ProvideMainConsumer,
		ProvideRetryConsumer,
		ProvideProducerClient,
		ProvideProducer,
		user.NewEventHandler,
		ProvideMessageHandler,
		ProvideKafkaChecker,
		ProvideSQSChecker,
		ProvideSQSClient,
		ProvideSQSProducer,
		ProvideSQSHandler,
		ProvideHealthRegistry,
		wire.Struct(new(WorkerApp), "*"),
	)
	return nil, nil, nil
}
