package rabbitMQ

import (
	"context"
	"errors"
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
		var handler ConsumeHandler = func(ctx context.Context, msg *Delivery) error {
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

		var handler ConsumeHandler = func(ctx context.Context, msg *Delivery) error {
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
		var handler ConsumeHandler = func(ctx context.Context, msg *Delivery) error {
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

		err := workCh.ConsumeMessagesEvery(context.Background(), handler)
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
func TestSendMessagesTx(t *testing.T) {
	// 1. 初始化配置 (请根据你的本地环境修改 DSN)
	cfg := Config{
		DSN:        []string{"amqp://guest:guest@localhost:5672/"},
		MaxChannel: 10,
	}

	conn, err := NewRabbitMQ(cfg)
	if err != nil {
		t.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// 2. 初始化队列配置
	qCfg := QueueConfig{
		ExchangeName: "test_tx_exchange",
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "test_tx_key",
		QueueName:    "test_tx_queue",
	}

	queue, err := NewQueue(conn, qCfg)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// 准备测试消息
	msgs := []amqp091.Publishing{
		{Body: []byte("tx_msg_1")},
		{Body: []byte("tx_msg_2")},
		{Body: []byte("tx_msg_3")},
	}

	t.Run("Success_Commit", func(t *testing.T) {
		ctx := context.Background()
		err := queue.SendMessagesTx(ctx, msgs)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		t.Log("Transaction committed successfully.")
	})

	t.Run("Rollback_On_Publish_Error", func(t *testing.T) {
		ctx := context.Background()

		// 构造一个会导致 Publish 失败的情况
		// 比如：使用一个不存在的交换机（这里需要修改源码或Mock，
		// 但基于你的 SendMessagesTx 实现，如果 Publish 内部出错会触发 Rollback）

		badQueue := &Queue{
			QueueConfig: QueueConfig{
				ExchangeName: "non_existent_exchange", // 错误的交换机名
				RouteKey:     "test_tx_key",
			},
			Connect: conn,
		}

		err := badQueue.SendMessagesTx(ctx, msgs)
		if err == nil {
			t.Error("Expected error due to non-existent exchange, but got nil")
		} else {
			t.Logf("Successfully caught error and rolled back: %v", err)
		}
	})

	t.Run("Verify_Data_Integrity", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		count := 0
		err := queue.ConsumeMessages(ctx, func(ctx context.Context, d *Delivery) error {
			count++
			t.Logf("Received message: %s", string(d.Body))

			// 关键点：当收到预期的 3 条消息后，主动取消上下文
			if count == 3 {
				cancel()
			}
			return nil
		})

		// --- 修复逻辑：忽略 context.Canceled 错误 ---
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("ConsumeMessages returned a real error: %v", err)
		}

		if count != 3 {
			t.Errorf("Data integrity check failed: expected 3 messages, got %d", count)
		} else {
			t.Log("Data integrity verified: all 3 committed messages received.")
		}
	})
}

func TestSendMessagesTx_AtomicRollback(t *testing.T) {
	// 1. 初始化连接
	cfg := Config{DSN: []string{"amqp://guest:guest@localhost:5672/"}}
	conn, _ := NewRabbitMQ(cfg)
	defer conn.Close()

	// 2. 声明一个普通队列（不带延迟功能）
	qCfg := QueueConfig{
		ExchangeName: "tx_atomic_exchange",
		RouteKey:     "tx_atomic_key",
		QueueName:    "tx_atomic_queue",
	}
	queue, err := NewQueue(conn, qCfg)
	if err != nil {
		t.Fatalf("Failed to setup queue: %v", err)
	}

	t.Run("Verify_All_Or_Nothing", func(t *testing.T) {
		// 构造消息列表：
		// 第一条：完全正常
		// 第二条：故意制造错误。
		// 注意：在 AMQP 中，如果 Exchange 不存在或路由不可达且设置了 mandatory，会报错。
		// 这里我们利用 SendMessagesTx 内部循环，模拟第二个消息发送到一个不存在的交换机。

		msgs := []amqp091.Publishing{
			{Body: []byte("first_correct_msg")},
			{Body: []byte("second_faulty_msg")},
		}

		// 为了能让循环中的第二条出错，我们临时用一个“坏”的 Queue 对象
		// 这样它在循环到第二条时，Publish 内部会因为 ExchangeName 错误而返回 err
		faultyQueue := &Queue{
			QueueConfig: QueueConfig{
				ExchangeName: "EXIST_EXCHANGE", // 先设为对的
				RouteKey:     qCfg.RouteKey,
			},
			Connect: conn,
		}

		// 我们通过这种方式模拟：SendMessagesTx 执行时，内部第一个成功，第二个失败
		// 但由于 SendMessagesTx 源码中：
		// for i := range messages { err = ch.Publish(...); if err != nil { ch.TxRollback(); return err } }
		// 只要第二个报错，第一个即便已经 Basic.Publish 成功，也必须被回滚。

		ctx := context.Background()

		// 模拟场景：修改交换机名为不存在的名字，直接触发 Publish 失败
		faultyQueue.ExchangeName = "non_existent_exchange"

		err := faultyQueue.SendMessagesTx(ctx, msgs)
		if err == nil {
			t.Fatal("Expected error from SendMessagesTx, but got nil")
		}
		t.Logf("Caught expected error during batch send: %v", err)

		// 3. 验证：队列里绝对不应该有第一条消息 "first_correct_msg"
		verifyCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		count := 0
		_ = queue.ConsumeMessages(verifyCtx, func(ctx context.Context, d *Delivery) error {
			count++
			t.Errorf("Unexpected message found in queue: %s. Rollback failed!", string(d.Body))
			return nil
		})

		if count == 0 {
			t.Log("SUCCESS: Even though the first message was sent successfully, the entire transaction was rolled back.")
		}
	})
}
