package rabbitMQ

import (
	e "github.com/Cotary/go-lib/err"
	"github.com/streadway/amqp"
)

type WorkConfig struct {
	ExchangeName string
	ExchangeType string
	RouteKey     string
	QueueName    string
	QueueType    string
}

type WorkCh struct {
	WorkConfig
	Ch *amqp.Channel
}

// NewWorkCh 工作模式常用配置
// todo 配置延迟队列， 消息发送到延迟队列，要过期了，就死信到正常队列
func NewWorkCh(conn *Connect, config WorkConfig) (*WorkCh, error) {
	ch, err := conn.conn.Channel()
	if err != nil {
		return nil, e.Err(err)
	}

	// 声明交换机
	err = ch.ExchangeDeclare(
		config.ExchangeName, // 交换机名称
		config.ExchangeType, // 交换机类型
		true,                // 持久性
		false,               // 自动删除
		false,               // 内部
		false,               // 无等待
		nil,                 // 其他参数
	)
	if err != nil {
		return nil, e.Err(err)
	}

	queueArgs := amqp.Table{}
	if config.QueueType != "" {
		queueArgs = amqp.Table{"x-queue-type": config.QueueType}
	}
	_, err = ch.QueueDeclare(
		config.QueueName, // 队列名称
		true,             // 持久性
		false,            // 专用性
		false,            // 自动删除
		false,            // 无等待
		queueArgs,        // 其他参数
	)
	if err != nil {
		return nil, e.Err(err)
	}

	// 绑定队列到交换机
	err = ch.QueueBind(
		config.QueueName,    // 队列名称
		config.RouteKey,     // 路由键
		config.ExchangeName, // 交换机名称
		false,
		nil,
	)
	if err != nil {
		return nil, e.Err(err)
	}

	return &WorkCh{
		WorkConfig: config,
		Ch:         ch,
	}, nil
}

func (c *WorkCh) ConsumeCh() (<-chan amqp.Delivery, error) {
	err := c.Ch.Qos(1, 0, false)
	if err != nil {
		return nil, e.Err(err)
	}
	return c.Ch.Consume(
		c.QueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
}
