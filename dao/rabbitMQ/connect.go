package rabbitMQ

import (
	e "github.com/Cotary/go-lib/err"
	"github.com/streadway/amqp"
)

type Config struct {
	DSN string `yaml:"dsn"`
}

type Connect struct {
	conn *amqp.Connection
}

func NewRabbitMQ(config Config) (*Connect, error) {
	//todo tls，加健康检查，连接池等功能
	conn, err := amqp.DialConfig(config.DSN, amqp.Config{})
	if err != nil {
		return nil, e.Err(err)
	}
	return &Connect{
		conn: conn,
	}, nil
}
