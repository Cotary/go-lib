package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type User struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// ==================== Memory Cache Tests ====================

func TestMemoryCache_BasicOps(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[string](MemoryConfig{MaxSize: 100, DefaultTTL: time.Minute})
	require.NoError(t, err)
	defer c.Close()

	// Set & Get
	require.NoError(t, c.Set(ctx, "k1", "v1"))
	v, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "v1", v)

	// Not found
	_, err = c.Get(ctx, "missing")
	assert.True(t, errors.Is(err, ErrNotFound))

	// Delete
	require.NoError(t, c.Delete(ctx, "k1"))
	_, err = c.Get(ctx, "k1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestMemoryCache_StructType(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[User](MemoryConfig{MaxSize: 100, DefaultTTL: time.Minute})
	require.NoError(t, err)
	defer c.Close()

	user := User{Name: "alice", Age: 30}
	require.NoError(t, c.Set(ctx, "u1", user))

	got, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, user, got)
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[string](MemoryConfig{MaxSize: 100, DefaultTTL: 100 * time.Millisecond})
	require.NoError(t, err)
	defer c.Close()

	require.NoError(t, c.Set(ctx, "k1", "v1"))
	time.Sleep(200 * time.Millisecond)

	_, err = c.Get(ctx, "k1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestMemoryCache_WithTTLOverride(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[string](MemoryConfig{MaxSize: 100, DefaultTTL: time.Minute})
	require.NoError(t, err)
	defer c.Close()

	require.NoError(t, c.Set(ctx, "k1", "v1", WithTTL(100*time.Millisecond)))
	time.Sleep(200 * time.Millisecond)

	_, err = c.Get(ctx, "k1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestMemoryCache_GetOrLoad(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[string](MemoryConfig{MaxSize: 100, DefaultTTL: time.Minute})
	require.NoError(t, err)
	defer c.Close()

	var callCount atomic.Int32
	loader := func(ctx context.Context, key string) (string, error) {
		callCount.Add(1)
		return "loaded:" + key, nil
	}

	v, err := c.GetOrLoad(ctx, "k1", loader)
	require.NoError(t, err)
	assert.Equal(t, "loaded:k1", v)
	assert.Equal(t, int32(1), callCount.Load())

	// Second call should hit cache
	v, err = c.GetOrLoad(ctx, "k1", loader)
	require.NoError(t, err)
	assert.Equal(t, "loaded:k1", v)
	assert.Equal(t, int32(1), callCount.Load())
}

func TestMemoryCache_GetOrLoad_Singleflight(t *testing.T) {
	ctx := context.Background()
	c, err := NewMemory[string](MemoryConfig{MaxSize: 100, DefaultTTL: time.Minute})
	require.NoError(t, err)
	defer c.Close()

	var callCount atomic.Int32
	loader := func(ctx context.Context, key string) (string, error) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "loaded", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad(ctx, "shared", loader)
			assert.NoError(t, err)
			assert.Equal(t, "loaded", v)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), callCount.Load(), "loader should be called only once via singleflight")
}

func TestMemoryCache_InvalidConfig(t *testing.T) {
	_, err := NewMemory[string](MemoryConfig{MaxSize: 0})
	assert.Error(t, err)
}

// ==================== Redis Cache Tests ====================

func newTestRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	return client
}

func TestRedisCache_BasicOps(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewRedis[User](RedisConfig{
		Client:     rdb,
		Prefix:     "test:basic",
		DefaultTTL: time.Minute,
	})
	require.NoError(t, err)
	defer c.Close()
	defer rdb.Del(ctx, "test:basic:u1")

	user := User{Name: "bob", Age: 25}
	require.NoError(t, c.Set(ctx, "u1", user))

	got, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, user, got)

	// Not found
	_, err = c.Get(ctx, "missing")
	assert.True(t, errors.Is(err, ErrNotFound))

	// Delete
	require.NoError(t, c.Delete(ctx, "u1"))
	_, err = c.Get(ctx, "u1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestRedisCache_StringType(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewRedis[string](RedisConfig{
		Client:     rdb,
		Prefix:     "test:str",
		DefaultTTL: time.Minute,
	})
	require.NoError(t, err)
	defer rdb.Del(ctx, "test:str:k1")

	require.NoError(t, c.Set(ctx, "k1", "hello"))

	v, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "hello", v)
}

func TestRedisCache_TTLExpiry(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewRedis[string](RedisConfig{
		Client:     rdb,
		Prefix:     "test:ttl",
		DefaultTTL: 200 * time.Millisecond,
	})
	require.NoError(t, err)

	require.NoError(t, c.Set(ctx, "k1", "v1"))
	time.Sleep(300 * time.Millisecond)

	_, err = c.Get(ctx, "k1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestRedisCache_GetOrLoad_Singleflight(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewRedis[string](RedisConfig{
		Client:     rdb,
		Prefix:     "test:sf",
		DefaultTTL: time.Minute,
	})
	require.NoError(t, err)
	defer rdb.Del(ctx, "test:sf:shared")

	var callCount atomic.Int32
	loader := func(ctx context.Context, key string) (string, error) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "loaded", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad(ctx, "shared", loader)
			assert.NoError(t, err)
			assert.Equal(t, "loaded", v)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), callCount.Load())
}

func TestRedisCache_CustomCodec(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewRedis[User](RedisConfig{
		Client:     rdb,
		Prefix:     "test:codec",
		DefaultTTL: time.Minute,
		Codec:      StdJsonCodec,
	})
	require.NoError(t, err)
	defer rdb.Del(ctx, "test:codec:u1")

	user := User{Name: "charlie", Age: 40}
	require.NoError(t, c.Set(ctx, "u1", user))

	got, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, user, got)
}

func TestRedisCache_InvalidConfig(t *testing.T) {
	_, err := NewRedis[string](RedisConfig{Client: nil, DefaultTTL: time.Minute})
	assert.Error(t, err)

	rdb := newTestRedisClient(t)
	_, err = NewRedis[string](RedisConfig{Client: rdb, DefaultTTL: 0})
	assert.Error(t, err)
}

// ==================== Two-Level Cache Tests ====================

func TestTwoLevelCache_BasicOps(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewTwoLevel[User](TwoLevelConfig{
		Local:  MemoryConfig{MaxSize: 100, DefaultTTL: 30 * time.Second},
		Remote: RedisConfig{Client: rdb, Prefix: "test:tl", DefaultTTL: time.Minute},
	})
	require.NoError(t, err)
	defer c.Close()
	defer rdb.Del(ctx, "test:tl:u1")

	user := User{Name: "dave", Age: 35}
	require.NoError(t, c.Set(ctx, "u1", user))

	// Get should hit L1
	got, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, user, got)

	// Not found
	_, err = c.Get(ctx, "missing")
	assert.True(t, errors.Is(err, ErrNotFound))

	// Delete removes from both levels
	require.NoError(t, c.Delete(ctx, "u1"))
	_, err = c.Get(ctx, "u1")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestTwoLevelCache_L2Promotion(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewTwoLevel[string](TwoLevelConfig{
		Local:  MemoryConfig{MaxSize: 100, DefaultTTL: 30 * time.Second},
		Remote: RedisConfig{Client: rdb, Prefix: "test:promo", DefaultTTL: time.Minute},
	})
	require.NoError(t, err)
	defer c.Close()
	defer rdb.Del(ctx, "test:promo:k1")

	// Write directly to Redis (simulating another process wrote it)
	data, _ := JsonCodec.Marshal("from-redis")
	rdb.Set(ctx, "test:promo:k1", data, time.Minute)

	// Get should find in L2 and promote to L1
	v, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "from-redis", v)

	// Second Get should hit L1 now (we can verify by deleting from Redis)
	rdb.Del(ctx, "test:promo:k1")
	v, err = c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "from-redis", v)
}

func TestTwoLevelCache_GetOrLoad(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewTwoLevel[string](TwoLevelConfig{
		Local:  MemoryConfig{MaxSize: 100, DefaultTTL: 30 * time.Second},
		Remote: RedisConfig{Client: rdb, Prefix: "test:tl-load", DefaultTTL: time.Minute},
	})
	require.NoError(t, err)
	defer c.Close()
	defer rdb.Del(ctx, "test:tl-load:k1")

	var callCount atomic.Int32
	loader := func(ctx context.Context, key string) (string, error) {
		callCount.Add(1)
		return "origin:" + key, nil
	}

	v, err := c.GetOrLoad(ctx, "k1", loader)
	require.NoError(t, err)
	assert.Equal(t, "origin:k1", v)
	assert.Equal(t, int32(1), callCount.Load())

	// Subsequent calls should hit L1, no more loader calls
	v, err = c.GetOrLoad(ctx, "k1", loader)
	require.NoError(t, err)
	assert.Equal(t, "origin:k1", v)
	assert.Equal(t, int32(1), callCount.Load())
}

func TestTwoLevelCache_GetOrLoad_Singleflight(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewTwoLevel[string](TwoLevelConfig{
		Local:  MemoryConfig{MaxSize: 100, DefaultTTL: 30 * time.Second},
		Remote: RedisConfig{Client: rdb, Prefix: "test:tl-sf", DefaultTTL: time.Minute},
	})
	require.NoError(t, err)
	defer c.Close()
	defer rdb.Del(ctx, "test:tl-sf:shared")

	var callCount atomic.Int32
	loader := func(ctx context.Context, key string) (string, error) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "loaded", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad(ctx, "shared", loader)
			assert.NoError(t, err)
			assert.Equal(t, "loaded", v)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), callCount.Load())
}

func TestTwoLevelCache_GetOrLoad_LoaderError(t *testing.T) {
	rdb := newTestRedisClient(t)
	ctx := context.Background()

	c, err := NewTwoLevel[string](TwoLevelConfig{
		Local:  MemoryConfig{MaxSize: 100, DefaultTTL: 30 * time.Second},
		Remote: RedisConfig{Client: rdb, Prefix: "test:tl-err", DefaultTTL: time.Minute},
	})
	require.NoError(t, err)
	defer c.Close()

	loaderErr := fmt.Errorf("db connection failed")
	loader := func(ctx context.Context, key string) (string, error) {
		return "", loaderErr
	}

	_, err = c.GetOrLoad(ctx, "k1", loader)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db connection failed")
}
