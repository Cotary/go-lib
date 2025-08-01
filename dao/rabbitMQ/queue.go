package rabbitMQ

import (
	"context"
	"fmt"
	e "github.com/Cotary/go-lib/err"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rabbitmq/amqp091-go"
	"runtime/debug"
	"time"
)

// QueueConfig 包含交换机和队列配置
type QueueConfig struct {
	ExchangeName string
	ExchangeType string // 添加交换机类型
	RouteKey     string
	QueueName    string
	QueueType    string
}

// Queue 表示工作通道结构体
type Queue struct {
	QueueConfig
	*Connect
}

// NewQueue 创建工作模式通道，使用通道池获取通道
func NewQueue(conn *Connect, config QueueConfig) (*Queue, error) {
	ch, err := conn.GetCh() // 从通道池中获取通道
	if err != nil {
		return nil, e.Err(err)
	}
	defer conn.PutCh(ch) // 确保方法退出时归还通道

	if config.ExchangeType == "" {
		config.ExchangeType = amqp091.ExchangeDirect // 默认为直连交换机
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

	queueArgs := amqp091.Table{}
	if config.QueueType != "" {
		queueArgs = amqp091.Table{"x-queue-type": config.QueueType}
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

	return &Queue{
		QueueConfig: config,
		Connect:     conn,
	}, nil
}

// SendMessages 发送消息到队列并返回未成功的消息列表
func (c *Queue) SendMessages(ctx context.Context, messages []amqp091.Publishing) ([]amqp091.Publishing, error) {
	ch, err := c.GetCh()
	if err != nil {
		return messages, e.Err(err)
	}
	defer func() {
		_ = ch.Close()
	}()

	// 开启确认模式
	err = ch.Confirm(false)
	if err != nil {
		return messages, e.Err(err)
	}

	// 确认通道
	//returns := ch.NotifyReturn(make(chan amqp091.Return, len(messages)))
	confirms := ch.NotifyPublish(make(chan amqp091.Confirmation, len(messages)))

	// 批量发送消息
	for i := range messages {
		//if messages[i].Headers == nil {
		//	messages[i].Headers = amqp091.Table{}
		//}
		//messages[i].Headers["__idx"] = i
		messages[i].DeliveryMode = amqp091.Persistent //消息持久化

		err = ch.Publish(
			c.ExchangeName, // 交换机
			c.RouteKey,     // 路由键（队列名称）
			false,          //mandatory 是否强制发送,配合returns
			false,          //immediate 是否立即发送
			messages[i],
		)
		if err != nil {
			return messages, e.Err(err)
		}
	}

	timeout := 5 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	// 等待确认
	var failed []amqp091.Publishing
	for i := 0; i < len(messages); i++ {
		select {
		//case ret := <-returns:
		//	if rawIdx, ok := ret.Headers["__idx"]; ok {
		//		failed = append(failed, messages[int(utils.AnyToInt(rawIdx))])
		//	} else {
		//		failed = append(failed, returnToPublishing(ret))
		//	}
		case confirm, ok := <-confirms:
			if !ok {
				// confirms 通道被关闭：把剩余都算失败
				for j := i; j < len(messages); j++ {
					failed = append(failed, messages[j])
				}
				return failed, fmt.Errorf("confirm channel closed unexpectedly")
			}
			if !confirm.Ack {
				// ack=false，也算失败
				idx := int(confirm.DeliveryTag - 1)
				failed = append(failed, messages[idx])
			}
		case <-timer.C:
			// 超时：剩余所有未确认的都当失败
			//可以放心这么写，因为 AMQP 的 Publisher‐Confirm 本身就是「严格按发送顺序」来回送 Ack/Nack 的。底层有这么几个保证：
			//1.每次 ch.Publish 之后，ch.NextPublishSeqNo 单调自增，从 1 开始。
			//2.服务器端收到消息后，会生成一个对应的 Confirmation，并且「按序」推回客户端。
			//3.Go 客户端的 NotifyPublish chan 就是把这些按序的 Confirmation 发给你。
			for j := i; j < len(messages); j++ {
				failed = append(failed, messages[j])
			}
			return failed, fmt.Errorf("publish confirm timeout after %s", timeout)
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

// SendMessagesEvery 持续发送消息，直到消息发送成功为止
func (c *Queue) SendMessagesEvery(ctx context.Context, messages []amqp091.Publishing) error {
	pending := messages
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		failed, err := c.SendMessages(ctx, pending)
		if err == nil && len(failed) == 0 {
			return nil
		}
		// 发送失败（包含重新路由），先报警再重试
		e.SendMessage(ctx, err)
		pending = failed // 如果 SendMessages 在 publish 前直接出错，则 failed==orig slice
		time.Sleep(5 * time.Second)
	}
}

var channelClosedErr = errors.New("deliveries channel closed") // 通道关闭的error

// ConsumeMessagesEvery 持续消费消息并处理，确保通道在处理完消息后才归还
func (c *Queue) ConsumeMessagesEvery(ctx context.Context, handler func(*Delivery) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := c.ConsumeMessages(ctx, handler)
			if err != nil {
				if !errors.Is(err, channelClosedErr) {
					e.SendMessage(ctx, err)
				}
				// 等待一段时间后重试，避免无限快速重试
				select {
				case <-time.After(time.Second * 5): // 重试间隔时间
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// ConsumeMessages 消费消息并处理，确保通道在处理完消息后才归还
func (c *Queue) ConsumeMessages(ctx context.Context, handler func(*Delivery) error) error {
	ch, err := c.GetCh()
	if err != nil {
		return e.Err(err)
	}
	tag := uuid.New().String()
	defer func() {
		ch.Cancel(tag, false)
		ch.Close() // 关闭通道,没有确认的会重新入队
	}()

	err = ch.Qos(1, 0, false) // 设置 QoS 1条/次
	if err != nil {
		return e.Err(err)
	}
	deliveries, err := ch.Consume(
		c.QueueName,
		tag,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return e.Err(err)
	}

	for {
		select {
		case <-ctx.Done():
			return e.Err(ctx.Err()) // 上下文取消，退出循环
		case amqpMsg, ok := <-deliveries:
			if !ok {
				return channelClosedErr
			}
			d := NewDelivery(amqpMsg)

			// 用局部 err 保证每条消息都单独判断
			localErr := func() (err error) {
				// 捕获 panic，转成 error 回传，并发送报警
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						err = fmt.Errorf("handler panic: %v\n%s", r, stack)
						e.SendMessage(ctx, err)
					}
				}()
				return handler(d)
			}()

			// 根据 localErr 做 Ack/Nack，不再复用外层 err
			if localErr != nil {
				e.SendMessage(ctx, e.Err(localErr, "mq consume error"))
				d.Nack()
			} else {
				d.Ack()
			}
			if d.Err != nil {
				return d.Err
			}

		}
	}
}

type Delivery struct {
	amqp091.Delivery
	Acked  bool
	Nacked bool
	Err    error
}

func NewDelivery(d amqp091.Delivery) *Delivery {
	return &Delivery{Delivery: d}
}

// Ack 确认消费（multiple=false）
func (d *Delivery) Ack() {
	if d.Acked || d.Nacked {
		return
	}
	err := d.Delivery.Ack(false)
	if err != nil {
		d.Err = err
	}
	d.Acked = true
	return
}

// Nack 重回队列（multiple=false, requeue=true）
func (d *Delivery) Nack() {
	if d.Acked || d.Nacked {
		return
	}
	err := d.Delivery.Nack(false, true)
	if err != nil {
		d.Err = err
	}
	d.Nacked = true
	return
}
