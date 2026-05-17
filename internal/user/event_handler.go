package user

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
)

// EventHandler consumes user-related Kafka messages. Initial impl is a
// no-op logger — wire it into business logic when async events ship.
type EventHandler struct {
	logger *slog.Logger
}

func NewEventHandler(logger *slog.Logger) *EventHandler {
	return &EventHandler{logger: logger}
}

func (h *EventHandler) Handle(ctx context.Context, record *kgo.Record) error {
	h.logger.Info("user event received",
		slog.String("topic", record.Topic),
		slog.Int64("offset", record.Offset),
		slog.String("payload", string(record.Value)),
	)
	return nil
}
