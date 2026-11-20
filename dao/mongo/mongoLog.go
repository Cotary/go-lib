package mongo

import (
	"context"
	"log/slog"
	"time"

	"github.com/Cotary/go-lib/log"
	"go.mongodb.org/mongo-driver/v2/event"
)

func newCommandMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   commandStarted,
		Succeeded: commandSucceeded,
		Failed:    commandFailed,
	}
}

func commandStarted(ctx context.Context, e *event.CommandStartedEvent) {
	fields := map[string]any{
		"event":       "mongo_cmd_start",
		"command":     e.CommandName,
		"database":    e.DatabaseName,
		"request_id":  e.RequestID,
		"raw_command": e.Command.String(),
	}
	slog.Default().With(log.WithFields(fields)...).InfoContext(ctx, "Mongo command started")
}

func commandSucceeded(ctx context.Context, e *event.CommandSucceededEvent) {
	fields := map[string]any{
		"event":      "mongo_cmd_ok",
		"command":    e.CommandName,
		"request_id": e.RequestID,
		"cost_ms":    e.Duration.Milliseconds(),
	}
	entry := slog.Default().With(log.WithFields(fields)...)

	if e.Duration > 500*time.Millisecond {
		entry.WarnContext(ctx, "Mongo slow command")
	} else {
		entry.InfoContext(ctx, "Mongo command succeeded")
	}
}

func commandFailed(ctx context.Context, e *event.CommandFailedEvent) {
	fields := map[string]any{
		"event":      "mongo_cmd_fail",
		"command":    e.CommandName,
		"request_id": e.RequestID,
		"cost_ms":    e.Duration.Milliseconds(),
		"error":      e.Failure.Error(),
	}
	slog.Default().With(log.WithFields(fields)...).ErrorContext(ctx, "Mongo command failed")
}
