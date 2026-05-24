package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// NewMongoClient connects, pings, and returns the v2 client together with
// a cleanup function that disconnects with a bounded timeout. The client
// is configured with an OTel CommandMonitor when observability is enabled.
func NewMongoClient(
	cfg config.MongoConfig,
	// feature:observability:start
	obs config.ObservabilityConfig,
	// feature:observability:end
) (*mongo.Client, func(), error) {
	// feature:observability:start
	stop := make(chan struct{})
	// feature:observability:end
	clientOpts := options.Client().ApplyURI(cfg.URI)
	// feature:observability:start
	if obs.Enabled {
		clientOpts = clientOpts.SetMonitor(NewCommandMonitor(stop))
	}
	// feature:observability:end

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		// feature:observability:start
		close(stop)
		// feature:observability:end
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		// feature:observability:start
		close(stop)
		// feature:observability:end
		_ = client.Disconnect(context.Background())
		return nil, nil, err
	}

	cleanup := func() {
		// feature:observability:start
		close(stop)
		// feature:observability:end
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}
	return client, cleanup, nil
}

func NewDatabase(client *mongo.Client, cfg config.MongoConfig) *mongo.Database {
	return client.Database(cfg.Database)
}
