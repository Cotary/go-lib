package rabbitMQ

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/pkg/errors"
	"github.com/rabbitmq/amqp091-go"
	"time"
)

type QueueConfig struct {
	ExchangeName string
	ExchangeType string
	RouteKey     string
	QueueName    string
	QueueType    string
}

type Queue struct {
	QueueConfig
	conn *Connect
}

func NewQueue(conn *Connect, cfg QueueConfig) (*Queue, error) {
	pool := conn.Pool()
	ch, err := pool.Get()
	if err != nil {
		return nil, e.Err(err)
	}
	defer pool.Put(ch)

	if cfg.ExchangeType == "" {
		cfg.ExchangeType = amqp091.ExchangeDirect
	}
	if cfg.QueueType == "" {
		cfg.QueueType = "quorum"
	}
	// 声明交换机
	if err = ch.ExchangeDeclare(
		cfg.ExchangeName,
		cfg.ExchangeType,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return nil, e.Err(err)
	}
	// 声明队列
	args := amqp091.Table{}
	if cfg.QueueType != "" {
		args = amqp091.Table{"x-queue-type": cfg.QueueType}

	}
	_, err = ch.QueueDeclare(
		cfg.QueueName,
		true,
		false,
		false,
		false,
		args,
	)
	if err != nil {
		return nil, e.Err(err)
	}
	// 绑定
	err = ch.QueueBind(
		cfg.QueueName,
		cfg.RouteKey,
		cfg.ExchangeName,
		false,
		nil,
	)
	if err != nil {
		return nil, e.Err(err)
	}
	return &Queue{
		QueueConfig: cfg,
		conn:        conn,
	}, nil
}

// SendMessages ：批量发布并收集所有 nack/return
func (q *Queue) SendMessages(ctx context.Context, messages []amqp091.Publishing) ([]amqp091.Publishing, error) {
	ch, err := q.conn.Pool().Get()
	if err != nil {
		return messages, e.Err(err)
	}
	defer ch.Close()

	// mandatory=true
	err = ch.Confirm(false)
	if err != nil {
		return messages, e.Err(err)
	}
	returns := ch.NotifyReturn(make(chan amqp091.Return, len(messages)))
	confirms := ch.NotifyPublish(make(chan amqp091.Confirmation, len(messages)))

	// publish loop
	for i := range messages {
		if messages[i].Headers == nil {
			messages[i].Headers = amqp091.Table{}
		}
		messages[i].Headers["__idx"] = i

		messages[i].DeliveryMode = amqp091.Persistent
		err = ch.Publish(
			q.ExchangeName,
			q.RouteKey,
			true,  // mandatory
			false, // immediate
			messages[i],
		)
		if err != nil {
			// 通道层面失败：所有还没发的、已发的直接当失败
			return messages, e.Err(err)
		}
	}

	// 等待并收集
	failed := make([]amqp091.Publishing, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		select {
		case ret := <-returns:
			if rawIdx, ok := ret.Headers["__idx"]; ok {
				failed = append(failed, messages[int(utils.AnyToInt(rawIdx))])
			} else {
				failed = append(failed, returnToPublishing(ret))
			}
		case confirm := <-confirms:
			if !confirm.Ack {
				failed = append(failed, messages[confirm.DeliveryTag-1])
			}
		case <-ctx.Done():
			return messages, e.Err(ctx.Err())
		}
	}
	if len(failed) > 0 {
		return failed, fmt.Errorf("some messages failed to deliver")
	}
	return nil, nil
}

// helper：从 amqp.Return 重建一个 Publishing
func returnToPublishing(ret amqp091.Return) amqp091.Publishing {
	return amqp091.Publishing{
		DeliveryMode:    ret.DeliveryMode,
		ContentType:     ret.ContentType,
		ContentEncoding: ret.ContentEncoding,
		Headers:         ret.Headers,
		CorrelationId:   ret.CorrelationId,
		ReplyTo:         ret.ReplyTo,
		Body:            ret.Body,
		// …其它你关心的字段也可以一并拷贝
	}
}

// SendMessagesEvery：永不丢，直到队列端确认
func (q *Queue) SendMessagesEvery(ctx context.Context, msgs []amqp091.Publishing) error {
	pending := msgs
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		failed, err := q.SendMessages(ctx, pending)
		if err == nil && len(failed) == 0 {
			return nil
		}
		// 发送失败（包含重新路由），先报警再重试
		e.SendMessage(ctx, err)
		pending = failed // 如果 SendMessages 在 publish 前直接出错，则 failed==orig slice
		time.Sleep(5 * time.Second)
	}
}

var (
	// Deliveries 通道关闭时的标记错误
	channelClosedErr = errors.New("deliveries channel closed")
)

const (
	MessagePriorityModel = "MessagePriority"
	ConfirmPriorityModel = "ConfirmPriority"
)

// ConsumeMessagesEvery 持续消费消息，遇到通道关闭或错误时自动重试
func (q *Queue) ConsumeMessagesEvery(
	ctx context.Context,
	model string,
	handler func(amqp091.Delivery) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 在单次调用中消费，出错就 break 跳到重试逻辑
		err := q.ConsumeMessages(ctx, model, handler)
		if err != nil && !errors.Is(err, channelClosedErr) {
			// 非通道关闭错误发报警
			e.SendMessage(ctx, err)
		}

		// 等待再重试
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// consumeOnce 在一个 channel 上拉一次消息，普通错误或通道断开时返回
func (q *Queue) ConsumeMessages(ctx context.Context, model string, handler func(amqp091.Delivery) error) error {
	// 1) 取通道
	ch, err := q.conn.Pool().Get()
	if err != nil {
		return e.Err(err)
	}
	defer q.conn.Pool().Put(ch)

	// 2) 批量拉 Prefetch=1
	err = ch.Qos(1, 0, false)
	if err != nil {
		return e.Err(err)
	}

	// 3) 建立 consume 管道
	deliveries, err := ch.Consume(
		q.QueueName,
		"",
		false, // noAutoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return e.Err(err)
	}

	// 4) 循环读取
	for {
		select {
		case <-ctx.Done():
			return e.Err(ctx.Err())
		case msg, ok := <-deliveries:
			if !ok {
				// 通道被关闭了
				return channelClosedErr
			}
			// 根据模式决定 Ack/Nack 顺序
			if model == MessagePriorityModel {
				err = handler(msg)
				if err != nil {
					if err = msg.Nack(false, true); err != nil {
						return e.Err(err)
					}
				} else {
					if err = msg.Ack(false); err != nil {
						return e.Err(err)
					}
				}
			} else {
				if err = msg.Ack(false); err != nil { // 这里确认之后，就会获取下一条记录
					return e.Err(err)
				}
				err = handler(msg) // 当消息处理超时，这个for会直接完成，然后重新获取通道等情况，同时会重新发送下一条记录，所以会有Redelivered出现
				if err != nil {
					e.SendMessage(ctx, errors.WithMessage(err, fmt.Sprintf("handle error message:%v", string(msg.Body))))
				}
			}
		}
	}
}
