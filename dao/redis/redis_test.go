package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 纯函数单元测试（不依赖 Redis 实例）
// ---------------------------------------------------------------------------

func TestNormalizeAddr(t *testing.T) {
	tests := []struct {
		host string
		port string
		want string
	}{
		{"127.0.0.1", "6379", "127.0.0.1:6379"},
		{"127.0.0.1:6380", "6379", "127.0.0.1:6380"},
		{"::1", "6379", "::1"},
		{"redis.example.com", "6380", "redis.example.com:6380"},
		{"", "6379", ":6379"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.host, tt.port), func(t *testing.T) {
			got := normalizeAddr(tt.host, tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeAddrs(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		want  []string
	}{
		{"normal", []string{"a:1", "b:2"}, []string{"a:1", "b:2"}},
		{"with_spaces", []string{" a:1 ", "  b:2"}, []string{"a:1", "b:2"}},
		{"with_empty", []string{"a:1", "", "  ", "b:2"}, []string{"a:1", "b:2"}},
		{"all_empty", []string{"", "  "}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAddrs(tt.nodes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKey(t *testing.T) {
	tests := []struct {
		prefix string
		key    string
		want   string
	}{
		{"app:", "user:1", "app:user:1"},
		{"", "user:1", "user:1"},
		{"prefix_", "key", "prefix_key"},
	}
	for _, tt := range tests {
		t.Run(tt.prefix+tt.key, func(t *testing.T) {
			c := Client{config: Config{Prefix: tt.prefix}}
			assert.Equal(t, tt.want, c.Key(tt.key))
		})
	}
}

func TestDbErr(t *testing.T) {
	t.Run("redis.Nil returns nil", func(t *testing.T) {
		assert.Nil(t, DbErr(redis.Nil))
	})

	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, DbErr(nil))
	})

	t.Run("real error passthrough", func(t *testing.T) {
		realErr := errors.New("connection refused")
		assert.Equal(t, realErr, DbErr(realErr))
	})

	t.Run("wrapped redis.Nil returns nil", func(t *testing.T) {
		wrapped := fmt.Errorf("get failed: %w", redis.Nil)
		assert.Nil(t, DbErr(wrapped))
	})
}

func TestNewTLSConfig(t *testing.T) {
	t.Run("default version", func(t *testing.T) {
		cfg := newTLSConfig(0)
		require.NotNil(t, cfg)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	})

	t.Run("custom version", func(t *testing.T) {
		cfg := newTLSConfig(tls.VersionTLS13)
		require.NotNil(t, cfg)
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	})
}

func TestPoolSize(t *testing.T) {
	assert.Equal(t, defaultPoolSize, poolSize(0))
	assert.Equal(t, defaultPoolSize, poolSize(-1))
	assert.Equal(t, 50, poolSize(50))
}

func TestDuration(t *testing.T) {
	assert.Equal(t, defaultReadTimeout, duration(0, defaultReadTimeout))
	assert.Equal(t, defaultWriteTimeout, duration(-1, defaultWriteTimeout))
	assert.Equal(t, 5*time.Second, duration(5000, defaultReadTimeout))
	assert.Equal(t, 100*time.Millisecond, duration(100, defaultReadTimeout))
}

// ---------------------------------------------------------------------------
// 集成测试（需要真实 Redis，通过环境变量 REDIS_TEST_ADDR 控制）
//
// 运行方式：
//   REDIS_TEST_ADDR=127.0.0.1:6379 go test -v -run TestIntegration ./dao/redis/
//
// 如需密码：
//   REDIS_TEST_AUTH=yourpassword
//
// 如需测试集群：
//   REDIS_TEST_CLUSTER=10.0.0.1:6379,10.0.0.2:6379,10.0.0.3:6379
// ---------------------------------------------------------------------------

func skipIfNoRedis(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("skipping: set REDIS_TEST_ADDR to run integration tests")
	}
	return addr
}

func newTestClient(t *testing.T) Client {
	t.Helper()
	addr := skipIfNoRedis(t)
	host, port := addr, ""
	config := &Config{
		Host:   host,
		Auth:   os.Getenv("REDIS_TEST_AUTH"),
		DB:     15, // 使用 DB15 避免影响业务数据
		Prefix: "test_golib:",
	}
	if idx := len(host) - 1; idx > 0 {
		for i := len(host) - 1; i >= 0; i-- {
			if host[i] == ':' {
				config.Host = host[:i]
				port = host[i+1:]
				break
			}
		}
	}
	if port != "" {
		config.Port = port
	} else {
		config.Port = "6379"
	}

	client, err := NewRedis(config)
	require.NoError(t, err)

	t.Cleanup(func() {
		client.FlushDB(context.Background())
		client.Close()
	})
	return client
}

func TestIntegrationNewRedis(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	pong, err := client.Ping(ctx).Result()
	require.NoError(t, err)
	assert.Equal(t, "PONG", pong)
}

func TestIntegrationSetGet(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	key := client.Key("hello")
	err := client.Set(ctx, key, "world", time.Minute).Err()
	require.NoError(t, err)

	val, err := client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, "world", val)

	_, err = client.Get(ctx, client.Key("nonexistent")).Result()
	assert.ErrorIs(t, err, redis.Nil)
	assert.Nil(t, DbErr(err))
}

func TestIntegrationScanKeys(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		client.Set(ctx, client.Key(fmt.Sprintf("scan:%d", i)), i, time.Minute)
	}
	for i := 0; i < 10; i++ {
		client.Set(ctx, client.Key(fmt.Sprintf("other:%d", i)), i, time.Minute)
	}

	keys, err := client.ScanKeys(ctx, client.Key("scan:*"))
	require.NoError(t, err)
	assert.Len(t, keys, 50)
}

func TestIntegrationScanKeysWithTimeout(t *testing.T) {
	client := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 20; i++ {
		client.Set(ctx, client.Key(fmt.Sprintf("timeout:%d", i)), i, time.Minute)
	}

	keys, err := client.ScanKeys(ctx, client.Key("timeout:*"))
	require.NoError(t, err)
	assert.Len(t, keys, 20)
}

func TestIntegrationScanKeysCancelledContext(t *testing.T) {
	client := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ScanKeys(ctx, client.Key("any:*"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestIntegrationPipeline(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	pipe := client.Pipeline()
	pipe.Set(ctx, client.Key("pipe:1"), "a", time.Minute)
	pipe.Set(ctx, client.Key("pipe:2"), "b", time.Minute)
	pipe.Set(ctx, client.Key("pipe:3"), "c", time.Minute)
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	val, err := client.Get(ctx, client.Key("pipe:2")).Result()
	require.NoError(t, err)
	assert.Equal(t, "b", val)
}

func TestIntegrationNewRedisInvalidAddr(t *testing.T) {
	config := &Config{
		Host: "127.0.0.1",
		Port: "1", // 不可能连上的端口
		DB:   0,
	}
	_, err := NewRedis(config)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// 集群集成测试
// ---------------------------------------------------------------------------

func TestIntegrationClusterScanKeys(t *testing.T) {
	nodes := os.Getenv("REDIS_TEST_CLUSTER")
	if nodes == "" {
		t.Skip("skipping: set REDIS_TEST_CLUSTER to run cluster tests")
	}

	var nodeList []string
	for _, n := range splitNodes(nodes) {
		if n != "" {
			nodeList = append(nodeList, n)
		}
	}

	config := &Config{
		Framework: "cluster",
		Nodes:     nodeList,
		Auth:      os.Getenv("REDIS_TEST_AUTH"),
		Prefix:    "test_cluster:",
	}

	client, err := NewRedis(config)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		client.Set(ctx, client.Key(fmt.Sprintf("cscan:%d", i)), i, time.Minute)
	}

	keys, err := client.ScanKeys(ctx, client.Key("cscan:*"))
	require.NoError(t, err)
	assert.Len(t, keys, 100)
}

func splitNodes(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
