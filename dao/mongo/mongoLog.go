package mongo

import (
	"context"
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
	log.WithContext(ctx).WithFields(fields).Info("Mongo command started")
}

func commandSucceeded(ctx context.Context, e *event.CommandSucceededEvent) {
	fields := map[string]any{
		"event":      "mongo_cmd_ok",
		"command":    e.CommandName,
		"request_id": e.RequestID,
		"cost_ms":    e.Duration.Milliseconds(),
	}
	entry := log.WithContext(ctx).WithFields(fields)

	if e.Duration > 500*time.Millisecond {
		entry.Warn("Mongo slow command")
	} else {
		entry.Info("Mongo command succeeded")
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
	log.WithContext(ctx).WithFields(fields).Error("Mongo command failed")
}
