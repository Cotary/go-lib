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
	"github.com/stretchr/testify/require"
)

// newTestQueue 创建一个带随机后缀名的测试队列，测试结束后自动清理
func newTestQueue(t *testing.T, conn *Connect, suffix string, maxDelay ...time.Duration) *Queue {
	t.Helper()
	name := fmt.Sprintf("test_%s_%d", suffix, time.Now().UnixNano())
	cfg := QueueConfig{
		ExchangeName: "ex_" + name,
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "rk_" + name,
		QueueName:    "q_" + name,
		QueueType:    QueueTypeQuorum,
	}
	if len(maxDelay) > 0 {
		cfg.MaxDelay = maxDelay[0]
	}
	q, err := NewQueue(conn, cfg)
	if err != nil {
		t.Fatalf("NewQueue failed: %v", err)
	}

	t.Cleanup(func() {
		ch, err := conn.GetCh()
		if err != nil {
			return
		}
		defer ch.Close()
		_, _ = ch.QueueDelete(cfg.QueueName, false, false, false)
		_ = ch.ExchangeDelete(cfg.ExchangeName, false, false)
	})

	return q
}

func TestSendAndConsume_Basic(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "basic")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages := []amqp091.Publishing{
		{Body: []byte("msg_1")},
		{Body: []byte("msg_2")},
		{Body: []byte("msg_3")},
	}
	failed, err := q.SendMessages(ctx, messages)
	require.NoError(t, err)
	assert.Empty(t, failed)

	received := make(map[string]bool)
	var mu sync.Mutex
	consumeCtx, consumeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer consumeCancel()

	go func() {
		_ = q.ConsumeMessagesEvery(consumeCtx, func(ctx context.Context, d *Delivery) error {
			mu.Lock()
			received[string(d.Body)] = true
			if len(received) == 3 {
				consumeCancel()
			}
			mu.Unlock()
			return nil
		})
	}()

	<-consumeCtx.Done()
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, received["msg_1"], "should receive msg_1")
	assert.True(t, received["msg_2"], "should receive msg_2")
	assert.True(t, received["msg_3"], "should receive msg_3")
}

func TestSendMessagesEvery_RetryOnFailure(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "retry")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	messages := []amqp091.Publishing{
		{Body: []byte("retry_msg_1")},
		{Body: []byte("retry_msg_2")},
	}
	err := q.SendMessagesEvery(ctx, messages)
	assert.NoError(t, err)

	count := 0
	consumeCtx, consumeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer consumeCancel()

	_ = q.ConsumeMessages(consumeCtx, func(ctx context.Context, d *Delivery) error {
		count++
		t.Logf("received: %s", d.Body)
		if count == 2 {
			consumeCancel()
		}
		return nil
	})
	assert.Equal(t, 2, count, "should receive exactly 2 messages")
}

func TestSendMessagesTx_CommitAndRollback(t *testing.T) {
	conn := tryConnect(t)

	t.Run("Commit", func(t *testing.T) {
		q := newTestQueue(t, conn, "tx_commit")
		ctx := context.Background()

		msgs := []amqp091.Publishing{
			{Body: []byte("tx_1")},
			{Body: []byte("tx_2")},
			{Body: []byte("tx_3")},
		}
		err := q.SendMessagesTx(ctx, msgs)
		require.NoError(t, err)

		consumeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		count := 0
		_ = q.ConsumeMessages(consumeCtx, func(ctx context.Context, d *Delivery) error {
			count++
			t.Logf("tx received: %s", d.Body)
			if count == 3 {
				cancel()
			}
			return nil
		})
		assert.Equal(t, 3, count, "should receive all 3 committed messages")
	})

	t.Run("RollbackOnBadExchange", func(t *testing.T) {
		q := newTestQueue(t, conn, "tx_rollback")
		ctx := context.Background()

		badQueue := &Queue{
			QueueConfig: QueueConfig{
				ExchangeName: "non_existent_exchange_" + fmt.Sprint(time.Now().UnixNano()),
				RouteKey:     q.RouteKey,
			},
			Connect: conn,
		}
		msgs := []amqp091.Publishing{{Body: []byte("should_fail")}}
		err := badQueue.SendMessagesTx(ctx, msgs)
		assert.Error(t, err, "should fail on non-existent exchange")
		t.Logf("expected error: %v", err)
	})
}

func TestConsumeMessages_HandlerPanicRecovery(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "panic")

	ctx := context.Background()
	msgs := []amqp091.Publishing{{Body: []byte("panic_msg")}}
	_, err := q.SendMessages(ctx, msgs)
	require.NoError(t, err)

	consumeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	handled := false
	_ = q.ConsumeMessagesEvery(consumeCtx, func(ctx context.Context, d *Delivery) error {
		if !handled {
			handled = true
			panic("simulated panic")
		}
		t.Logf("recovered, message redelivered: %s", d.Body)
		cancel()
		return nil
	})

	assert.True(t, handled, "handler should have been called at least once")
}

func TestConsumeMessages_CtxCancel(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "ctx_cancel")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.ConsumeMessages(ctx, func(ctx context.Context, d *Delivery) error {
		return nil
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestConsumeMessages_Prefetch(t *testing.T) {
	conn := tryConnect(t)

	name := fmt.Sprintf("test_prefetch_%d", time.Now().UnixNano())
	cfg := QueueConfig{
		ExchangeName: "ex_" + name,
		ExchangeType: amqp091.ExchangeDirect,
		RouteKey:     "rk_" + name,
		QueueName:    "q_" + name,
		QueueType:    QueueTypeQuorum,
		Prefetch:     5,
	}
	q, err := NewQueue(conn, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ch, chErr := conn.GetCh()
		if chErr != nil {
			return
		}
		defer ch.Close()
		_, _ = ch.QueueDelete(cfg.QueueName, false, false, false)
		_ = ch.ExchangeDelete(cfg.ExchangeName, false, false)
	})

	ctx := context.Background()
	msgs := make([]amqp091.Publishing, 10)
	for i := range msgs {
		msgs[i] = amqp091.Publishing{Body: []byte(fmt.Sprintf("prefetch_%d", i))}
	}
	_, err = q.SendMessages(ctx, msgs)
	require.NoError(t, err)

	consumeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	count := 0
	_ = q.ConsumeMessages(consumeCtx, func(ctx context.Context, d *Delivery) error {
		count++
		t.Logf("prefetch received: %s", d.Body)
		if count == 10 {
			cancel()
		}
		return nil
	})
	assert.Equal(t, 10, count, "should receive all 10 messages with prefetch=5")
}

func TestDelayQueue_RetryLater(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "delay", 30*time.Second)

	ctx := context.Background()
	msgs := []amqp091.Publishing{{Body: []byte("delay_msg")}}
	_, err := q.SendMessages(ctx, msgs)
	require.NoError(t, err)

	consumeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	var firstTime, secondTime time.Time
	var callCount int
	var mu sync.Mutex

	_ = q.ConsumeMessagesEvery(consumeCtx, func(ctx context.Context, d *Delivery) error {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		t.Logf("[%s] received (attempt %d, retry_count=%d): %s",
			time.Now().Format("15:04:05"), callCount, d.GetRetryNum(), d.Body)

		if callCount == 1 {
			firstTime = time.Now()
			return fmt.Errorf("simulated first failure")
		}
		secondTime = time.Now()
		cancel()
		return nil
	})

	mu.Lock()
	defer mu.Unlock()
	if callCount >= 2 {
		delay := secondTime.Sub(firstTime)
		t.Logf("delay between first failure and retry: %v", delay)
		assert.GreaterOrEqual(t, delay.Seconds(), 1.0, "retry should have delay")
	}
	assert.GreaterOrEqual(t, callCount, 1, "should consume at least once")
}

func TestDelayQueue_RetryCountIncrement(t *testing.T) {
	conn := tryConnect(t)
	q := newTestQueue(t, conn, "retry_count", 5*time.Second)

	ctx := context.Background()
	msgs := []amqp091.Publishing{{Body: []byte("count_msg")}}
	_, err := q.SendMessages(ctx, msgs)
	require.NoError(t, err)

	consumeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var retryNums []int64
	var mu sync.Mutex

	_ = q.ConsumeMessagesEvery(consumeCtx, func(ctx context.Context, d *Delivery) error {
		mu.Lock()
		retryNums = append(retryNums, d.GetRetryNum())
		count := len(retryNums)
		mu.Unlock()

		t.Logf("attempt %d, retry_count=%d", count, d.GetRetryNum())

		if count >= 3 {
			cancel()
			return nil
		}
		return fmt.Errorf("fail to trigger retry")
	})

	mu.Lock()
	defer mu.Unlock()
	if len(retryNums) >= 3 {
		assert.Equal(t, int64(0), retryNums[0], "first attempt should have retry_count=0")
		assert.Equal(t, int64(1), retryNums[1], "second attempt should have retry_count=1")
		assert.Equal(t, int64(2), retryNums[2], "third attempt should have retry_count=2")
	}
}
