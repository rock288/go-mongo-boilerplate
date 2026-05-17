package user_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rock288/go-mongo-boilerplate/internal/user"
)

func TestUserEventHandler_Handle_Success_LogsAndReturnsNil(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := user.NewEventHandler(logger)

	record := &kgo.Record{
		Topic:  "user.events",
		Offset: 42,
		Value:  []byte(`{"id":"abc"}`),
	}

	require.NoError(t, h.Handle(context.Background(), record))

	out := buf.String()
	assert.Contains(t, out, "user event received")
	assert.Contains(t, out, "user.events")
	assert.Contains(t, out, "42")
}

func TestUserEventHandler_Handle_EmptyValue_NoError(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	h := user.NewEventHandler(logger)

	require.NoError(t, h.Handle(context.Background(), &kgo.Record{Topic: "x"}))
}

func TestUserEventHandler_Handle_PayloadInLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := user.NewEventHandler(logger)

	payload := `{"event":"user.created","id":"1"}`
	require.NoError(t, h.Handle(context.Background(), &kgo.Record{
		Topic: "user.events", Value: []byte(payload),
	}))

	assert.True(t, strings.Contains(buf.String(), "user.created"), "payload should be logged")
}
