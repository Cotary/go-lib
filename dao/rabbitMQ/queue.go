package rabbitMQ

import (
	"context"
	"fmt"
	e "github.com/Cotary/go-lib/err"
	"github.com/streadway/amqp"
	"time"
)

// WorkConfig 包含交换机和队列配置
type WorkConfig struct {
	ExchangeName string
	ExchangeType string // 添加交换机类型
	RouteKey     string
	QueueName    string
	QueueType    string
}

// WorkCh 表示工作通道结构体
type WorkCh struct {
	WorkConfig
	*ChannelPool
}

// NewWorkCh 创建工作模式通道，使用通道池获取通道
func NewWorkCh(pool *ChannelPool, config WorkConfig) (*WorkCh, error) {
	ch, err := pool.Get() // 从通道池中获取通道
	if err != nil {
		return nil, e.Err(err)
	}
	defer pool.Put(ch) // 确保方法退出时归还通道

	if config.ExchangeType == "" {
		config.ExchangeType = amqp.ExchangeDirect // 默认为直连交换机
	}
	if config.QueueType == "" {
		config.QueueType = "quorum" // 默认为持久队列
	}

	// 声明交换机
	err = ch.ExchangeDeclare(
		config.ExchangeName,
		config.ExchangeType,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, e.Err(err)
	}

	queueArgs := amqp.Table{}
	if config.QueueType != "" {
		queueArgs = amqp.Table{"x-queue-type": config.QueueType}
	}
	_, err = ch.QueueDeclare(
		config.QueueName,
		true,
		false,
		false,
		false,
		queueArgs,
	)
	if err != nil {
		return nil, e.Err(err)
	}

	// 绑定队列到交换机
	err = ch.QueueBind(
		config.QueueName,
		config.RouteKey,
		config.ExchangeName,
		false,
		nil,
	)
	if err != nil {
		return nil, e.Err(err)
	}

	return &WorkCh{
		WorkConfig:  config,
		ChannelPool: pool,
	}, nil
}

// SendMessages 发送消息到队列并返回未成功的消息列表
func (c *WorkCh) SendMessages(ctx context.Context, messages []string) ([]string, error) {
	ch, err := c.ChannelPool.Get() // 从通道池中获取通道
	if err != nil {
		return nil, e.Err(err)
	}
	defer c.ChannelPool.Put(ch) // 确保方法退出时归还通道

	// 开启确认模式
	err = ch.Confirm(false)
	if err != nil {
		return nil, e.Err(err)
	}

	// 确认通道
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, len(messages)))

	// 批量发送消息
	for _, body := range messages {
		err = ch.Publish(
			c.ExchangeName, // 交换机
			c.RouteKey,     // 路由键（队列名称）
			false,          // 是否强制发送
			false,          // 是否立即发送
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
			})
		if err != nil {
			return nil, e.Err(err)
		}
	}

	// 等待确认
	var failedMessages []string
	for i := 0; i < len(messages); i++ {
		select {
		case confirm := <-confirms:
			if !confirm.Ack {
				failedMessages = append(failedMessages, messages[i])
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if len(failedMessages) > 0 {
		return failedMessages, fmt.Errorf("some messages failed to deliver")
	}

	return nil, nil
}

// SendMessagesEvery 持续发送消息，直到消息发送成功为止
func (c *WorkCh) SendMessagesEvery(ctx context.Context, messages []string) error {
	var err error
	failedMessages := messages

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			failedMessages, err = c.SendMessages(ctx, failedMessages)
			if err == nil || len(failedMessages) == 0 {
				return nil
			}

			e.SendMessage(ctx, err)
			// 等待一段时间后重试，避免无限快速重试
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second * 5): // 重试间隔时间
			}
		}
	}
}

// ConsumeMessagesEvery 持续消费消息并处理，确保通道在处理完消息后才归还
func (c *WorkCh) ConsumeMessagesEvery(ctx context.Context, handler func(amqp.Delivery)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := c.ConsumeMessages(handler)
			if err != nil {
				e.SendMessage(ctx, err)
				// 等待一段时间后重试，避免无限快速重试
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second * 5): // 重试间隔时间
				}
			}
		}
	}
}

// ConsumeMessages 消费消息并处理，确保通道在处理完消息后才归还
func (c *WorkCh) ConsumeMessages(handler func(amqp.Delivery)) error {
	ch, err := c.ChannelPool.Get() // 从通道池中获取通道
	if err != nil {
		return e.Err(err)
	}
	defer c.ChannelPool.Put(ch) // 确保在返回之前归还通道

	err = ch.Qos(1, 0, false)
	if err != nil {
		return e.Err(err)
	}
	deliveries, err := ch.Consume(
		c.QueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return e.Err(err)
	}

	for msg := range deliveries {
		handler(msg)
		if err := msg.Ack(false); err != nil {
			return err
		}
	}

	return nil
}
