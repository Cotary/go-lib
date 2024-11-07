package rabbitMQ

import (
	e "github.com/Cotary/go-lib/err"
	"github.com/streadway/amqp"
)

type WorkConfig struct {
	ExchangeName string
	RouteKey     string
	QueueName    string
	QueueType    string
}

type WorkCh struct {
	WorkConfig
	Ch *amqp.Channel
}

// NewWorkCh 工作模式常用配置
func NewWorkCh(conn *Connect, config WorkConfig) (*WorkCh, error) {
	ch, err := conn.Conn.Channel()
	if err != nil {
		return nil, e.Err(err)
	}

	// 声明交换机
	err = ch.ExchangeDeclare(
		config.ExchangeName,
		amqp.ExchangeDirect,
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
		WorkConfig: config,
		Ch:         ch,
	}, nil
}

func (c *WorkCh) ConsumeCh() (<-chan amqp.Delivery, error) {
	err := c.Ch.Qos(1, 0, false)
	if err != nil {
		return nil, err
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
