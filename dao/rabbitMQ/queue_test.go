package rabbitMQ

import (
	"context"
	"fmt"
	"sync"
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

	workChConfig := QueueConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewQueue(conn, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试生产者
	done := make(chan bool)
	go func() {
		messages := []amqp091.Publishing{
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
		handler := func(msg *Delivery) error {
			t.Logf("Received message: %s", msg.Body)

			// 模拟处理失败并回退消息
			if string(msg.Body) == "message2" && failedMessages[string(msg.Body)] < 1 {
				failedMessages[string(msg.Body)]++
				return fmt.Errorf("simulated error for message: %s", msg.Body)
			}

			assert.Contains(t, []string{"message1", "message2", "message3"}, string(msg.Body))
			return nil
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

	workChConfig := QueueConfig{
		ExchangeName: "test_exchange",
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "test_route",
		QueueName:    "test_queue",
		QueueType:    "quorum",
	}

	workCh, err := NewQueue(conn, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	fmt.Println("can disconnect")
	time.Sleep(5 * time.Second)

	go func() {

		handler := func(msg *Delivery) error {
			fmt.Println("receive message", string(msg.Body))
			return nil
		}
		err := workCh.ConsumeMessagesEvery(context.Background(), handler)
		assert.NoError(t, err)
	}()

	messages := []amqp091.Publishing{
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
	err = workCh.SendMessagesEvery(context.Background(), messages)
	fmt.Println(err)

	time.Sleep(10 * time.Second)

}
func TestWorkCh_DelayQueue(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 10,
	}
	conn, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	workChConfig := QueueConfig{
		ExchangeName: "test_exchange_delay",
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "test_route_delay",
		QueueName:    "test_queue_delay",
		QueueType:    "quorum",
		MaxDelay:     10 * time.Second, // 队列最大延迟
	}

	workCh, err := NewQueue(conn, workChConfig)
	assert.NoError(t, err)
	assert.NotNil(t, workCh)

	fmt.Println("等待 RabbitMQ 连接稳定...")
	time.Sleep(2 * time.Second)

	// 用于标记是否是第一次消费
	var firstConsume sync.Map

	go func() {
		handler := func(msg *Delivery) error {
			body := string(msg.Body)
			fmt.Printf("[%s] 收到消息: %s\n", time.Now().Format("15:04:05"), body)

			// 第一次消费该消息时，模拟失败并延迟 5 秒重试
			if _, loaded := firstConsume.LoadOrStore(body, true); !loaded {
				return fmt.Errorf("模拟处理失败")
			}

			// 第二次消费成功
			fmt.Printf("[%s] 第二次消费成功: %s\n", time.Now().Format("15:04:05"), body)
			//msg.Ack()
			return nil
		}

		err := workCh.ConsumeMessagesEvery(context.Background(), handler, 3*time.Second)
		assert.NoError(t, err)
	}()

	// 发送一条测试消息
	messages := []amqp091.Publishing{
		{Body: []byte("delay_message_1")},
	}
	err = workCh.SendMessagesEvery(context.Background(), messages)
	assert.NoError(t, err)

	// 等待足够时间观察延迟效果
	time.Sleep(20 * time.Second)
}
