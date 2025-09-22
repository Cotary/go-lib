package redis

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/log"
	"github.com/redis/go-redis/v9"
	"net"
	"strings"
	"time"
)

type LogHook struct{}

// DialHook：连接建立时触发
func (LogHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		start := time.Now()
		conn, err := next(ctx, network, addr)
		log.WithContext(ctx).WithFields(map[string]interface{}{
			"event":   "redis_dial",
			"network": network,
			"addr":    addr,
			"cost_ms": time.Since(start).Milliseconds(),
			"error":   err,
		}).Info("Redis Dial")

		return conn, err
	}
}

// ProcessHook：单条命令执行时触发
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

		log.WithContext(ctx).WithFields(map[string]interface{}{
			"event":   "redis_cmd",
			"command": cmd.Name(),
			"args":    parts[1:],
			"raw":     cmdStr,
			"cost_ms": time.Since(start).Milliseconds(),
			"error":   err,
		}).Info("Redis command")

		return err
	}
}

// ProcessPipelineHook：Pipeline 批量命令执行时触发
func (LogHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()

		// 收集 pipeline 命令
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
		log.WithContext(ctx).WithFields(map[string]interface{}{
			"event":     "redis_pipeline",
			"cmd_count": len(cmds),
			"commands":  cmdList,
			"cost_ms":   time.Since(start).Milliseconds(),
			"error":     err,
		}).Info("Redis pipeline")

		return err
	}
}
