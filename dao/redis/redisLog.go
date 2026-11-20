package redis

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/Cotary/go-lib/log"
	"github.com/redis/go-redis/v9"
)

type LogHook struct{}

func (LogHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		start := time.Now()
		conn, err := next(ctx, network, addr)
		fields := map[string]interface{}{
			"event":   "redis_dial",
			"network": network,
			"addr":    addr,
			"cost_ms": time.Since(start).Milliseconds(),
		}
		entry := slog.Default().With(log.WithFields(fields)...)
		if err != nil {
			entry.With("error", err).ErrorContext(ctx, "Redis Dial failed")
		} else {
			entry.InfoContext(ctx, "Redis Dial")
		}
		return conn, err
	}
}

func (LogHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		args := cmd.Args()
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = fmt.Sprint(arg)
		}
		cmdStr := strings.Join(parts, " ")
		err := next(ctx, cmd)

		fields := map[string]interface{}{
			"event":   "redis_cmd",
			"command": cmd.Name(),
			"args":    parts[1:],
			"raw":     cmdStr,
			"cost_ms": time.Since(start).Milliseconds(),
		}
		entry := slog.Default().With(log.WithFields(fields)...)
		if err != nil {
			entry.With("error", err).ErrorContext(ctx, "Redis command failed")
		} else {
			entry.InfoContext(ctx, "Redis command")
		}
		return err
	}
}

func (LogHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()

		cmdList := make([]string, len(cmds))
		for i, cmd := range cmds {
			args := cmd.Args()
			parts := make([]string, len(args))
			for j, arg := range args {
				parts[j] = fmt.Sprint(arg)
			}
			cmdList[i] = strings.Join(parts, " ")
		}
		err := next(ctx, cmds)

		fields := map[string]interface{}{
			"event":     "redis_pipeline",
			"cmd_count": len(cmds),
			"commands":  cmdList,
			"cost_ms":   time.Since(start).Milliseconds(),
		}
		entry := slog.Default().With(log.WithFields(fields)...)
		if err != nil {
			entry.With("error", err).ErrorContext(ctx, "Redis pipeline failed")
		} else {
			entry.InfoContext(ctx, "Redis pipeline")
		}
		return err
	}
}
