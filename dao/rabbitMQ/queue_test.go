package rabbitMQ

import (
	"context"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
)

func TestWorkCh(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 10,
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	pool, err := NewChannelPool(conn, 5)
	assert.NoError(t, err)
	assert.NotNil(t, pool)

	workChConfig := WorkConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewWorkCh(pool, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试生产者
	done := make(chan bool)
	go func() {
		messages := []string{"message1", "message2", "message3"}
		err := workCh.SendMessagesEvery(ctx, messages)
		assert.NoError(t, err)
		done <- true
	}()

	// 测试消费者
	go func() {
		handler := func(msg amqp.Delivery) {
			t.Logf("Received message: %s", msg.Body)
			assert.Contains(t, []string{"message1", "message2", "message3"}, string(msg.Body))
		}
		err := workCh.ConsumeMessagesEvery(ctx, handler)
		assert.NoError(t, err)
	}()
	// 等待生产者发送完成
	<-done
	// 额外等待以确保消息被消费
	time.Sleep(5 * time.Second)
}

func TestWorkCh_SendMessages(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 10,
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	pool, err := NewChannelPool(conn, 5)
	assert.NoError(t, err)
	assert.NotNil(t, pool)

	workChConfig := WorkConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewWorkCh(pool, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messages := []string{"message1", "message2", "message3"}
	failedMessages, err := workCh.SendMessages(ctx, messages)
	assert.NoError(t, err)
	assert.Empty(t, failedMessages)

	workCh.Close()
}