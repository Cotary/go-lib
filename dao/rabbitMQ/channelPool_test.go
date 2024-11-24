package rabbitMQ

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestNewChannelPool(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 1, // 设置心跳时间
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	channelPool, err := NewChannelPool(conn, 5)
	assert.NoError(t, err)
	assert.NotNil(t, channelPool)

	// 确保通道池正确初始化
	for i := 0; i < 5; i++ {
		ch := <-channelPool.pool
		assert.NotNil(t, ch)
		channelPool.pool <- ch
	}
}

func TestChannelPoolGetAndPut(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 10, // 设置心跳时间
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	channelPool, err := NewChannelPool(conn, 5)
	assert.NoError(t, err)
	assert.NotNil(t, channelPool)

	// 测试从通道池中获取通道
	ch, err := channelPool.Get()
	assert.NoError(t, err)
	assert.NotNil(t, ch)

	// 测试将通道归还到通道池中
	channelPool.Put(ch)
	assert.Equal(t, 1, len(channelPool.pool))
}

func TestChannelPoolClose(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 1, // 设置心跳时间
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	channelPool, err := NewChannelPool(conn, 5)
	assert.NoError(t, err)
	assert.NotNil(t, channelPool)

	// 确保通道池正确初始化
	for i := 0; i < 5; i++ {
		ch := <-channelPool.pool
		assert.NotNil(t, ch)
		channelPool.pool <- ch
	}
	// 测试从通道池中获取通道
	ch, err := channelPool.Get()
	assert.NoError(t, err)
	assert.NotNil(t, ch)

	// 测试将通道归还到通道池中
	channelPool.Put(ch)
	assert.Equal(t, 1, len(channelPool.pool))

	time.Sleep(5 * time.Second)
	// 关闭通道池
	channelPool.Close()

	// 确保所有通道都被关闭
	for ch := range channelPool.pool {
		assert.Error(t, ch.Close()) // 关闭已经关闭的通道应返回错误
	}
}
