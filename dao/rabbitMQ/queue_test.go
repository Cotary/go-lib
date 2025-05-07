package rabbitMQ

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
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

	workChConfig := QueueConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewQueue(pool, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试生产者
	done := make(chan bool)
	go func() {
		messages := []amqp.Publishing{
			{
				Body: []byte("message1"),
			},
			{
				Body: []byte("message2"),
			},
			{
				Body: []byte("message3"),
			},
		}
		err := workCh.SendMessagesEvery(ctx, messages)
		assert.NoError(t, err)
		done <- true
	}()

	// 测试消费者
	failedMessages := make(map[string]int)
	go func() {
		handler := func(msg amqp.Delivery) error {
			t.Logf("Received message: %s", msg.Body)

			// 模拟处理失败并回退消息
			if string(msg.Body) == "message2" && failedMessages[string(msg.Body)] < 1 {
				failedMessages[string(msg.Body)]++
				return fmt.Errorf("simulated error for message: %s", msg.Body)
			}

			assert.Contains(t, []string{"message1", "message2", "message3"}, string(msg.Body))
			return nil
		}
		err := workCh.ConsumeMessagesEvery(ctx, MessagePriority, handler)
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

	workChConfig := QueueConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewQueue(pool, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messages := []amqp.Publishing{
		{
			Body: []byte("message1"),
		},
		{
			Body: []byte("message2"),
		},
		{
			Body: []byte("message2"),
		},
	}
	failedMessages, err := workCh.SendMessages(ctx, messages)
	assert.NoError(t, err)
	assert.Empty(t, failedMessages)

	workCh.Close()
}
