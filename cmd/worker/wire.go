//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/logger"

	// feature:kafka:start
	// feature:samples:start
	"github.com/rock288/go-mongo-boilerplate/internal/user"
	// feature:samples:end
	// feature:kafka:end
)

func InitializeWorker(cfgPath string) (*WorkerApp, func(), error) {
	wire.Build(
		config.Load,
		// feature:kafka:start
		ProvideKafkaConfig,
		// feature:kafka:end
		ProvideLoggerConfig,
		// feature:observability:start
		ProvideObservabilityConfig,
		ProvideObservability,
		// feature:observability:end
		// feature:sqs:start
		ProvideSQSConfig,
		// feature:sqs:end
		logger.New,
		// feature:kafka:start
		ProvideMainConsumer,
		ProvideRetryConsumer,
		ProvideProducerClient,
		ProvideProducer,
		// feature:samples:start
		user.NewEventHandler,
		ProvideMessageHandler,
		// feature:samples:end
		ProvideKafkaChecker,
		// feature:kafka:end
		// feature:sqs:start
		ProvideSQSChecker,
		ProvideSQSClient,
		ProvideSQSProducer,
		ProvideSQSHandler,
		// feature:sqs:end
		ProvideHealthRegistry,
		wire.Struct(new(WorkerApp), "*"),
	)
	return nil, nil, nil
}
