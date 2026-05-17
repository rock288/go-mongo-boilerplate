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
func NewMongoClient(cfg config.MongoConfig, obs config.ObservabilityConfig) (*mongo.Client, func(), error) {
	stop := make(chan struct{})
	clientOpts := options.Client().ApplyURI(cfg.URI)
	if obs.Enabled {
		clientOpts = clientOpts.SetMonitor(NewCommandMonitor(stop))
	}

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		close(stop)
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		close(stop)
		_ = client.Disconnect(context.Background())
		return nil, nil, err
	}

	cleanup := func() {
		close(stop)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}
	return client, cleanup, nil
}

func NewDatabase(client *mongo.Client, cfg config.MongoConfig) *mongo.Database {
	return client.Database(cfg.Database)
}
