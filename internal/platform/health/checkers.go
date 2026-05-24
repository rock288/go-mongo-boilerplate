package health

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoChecker pings the Mongo client. Critical: failure trips readiness.
type MongoChecker struct {
	client *mongo.Client
}

func NewMongoChecker(client *mongo.Client) *MongoChecker {
	return &MongoChecker{client: client}
}

func (m *MongoChecker) Name() string       { return "mongo" }
func (m *MongoChecker) Severity() Severity { return SeverityCritical }
func (m *MongoChecker) Check(ctx context.Context) error {
	return m.client.Ping(ctx, nil)
}
