package rabbitMQ

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	e "github.com/Cotary/go-lib/err"
	"github.com/streadway/amqp"
)

type Config struct {
	DSN        []string `yaml:"dsn"`
	CA         string   `yaml:"caPath"`
	ClientUser string   `yaml:"clientUserPath"`
	ClientKey  string   `yaml:"clientKeyPath"`
	Heartbeat  int64    `yaml:"heartbeat"` // heartbeat 心跳检查 秒
}

type Connect struct {
	Conn *amqp.Connection
	Config
	closeCh chan struct{}
}

func handleConfig(config *Config) {
	if config.Heartbeat == 0 {
		config.Heartbeat = 30
	}
}

func NewRabbitMQ(config Config) (*Connect, error) {
	handleConfig(&config)
	mq := &Connect{
		Conn:    nil,
		Config:  config,
		closeCh: make(chan struct{}),
	}
	err := mq.connect()
	if err != nil {
		return nil, e.Err(err)
	}

	ctx := coroutines.NewContext("RabbitMQ Health")
	coroutines.SafeGo(ctx, func(ctx context.Context) {
		mq.checkHealth(ctx)
	})
	return mq, nil
}

func (c *Connect) checkHealth(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(c.Config.Heartbeat) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			//fmt.Println("Health check...")
			if c.Conn.IsClosed() {
				err := c.connect()
				if err != nil {
					e.SendMessage(ctx, err)
				}
			}
		case <-c.closeCh:
			//fmt.Println("Stopping health check...")
			return
		}
	}
}

func (c *Connect) connect() error {
	var err error
	var tlsConfig tls.Config
	if c.Config.CA != "" {
		caCert, err := os.ReadFile(c.Config.CA)
		if err != nil {
			return errors.New(fmt.Sprintf("read CA file err:%v", err))
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		tlsConfig.RootCAs.AppendCertsFromPEM(caCert)
	}
	if c.Config.ClientUser != "" && c.Config.ClientKey != "" {
		clientCert, err := tls.LoadX509KeyPair(c.Config.ClientUser, c.Config.ClientKey)
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to load client certificate: %v", err))
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	amqpConfig := amqp.Config{
		TLSClientConfig: &tlsConfig,
		Heartbeat:       time.Duration(c.Config.Heartbeat) * time.Second,
	}

	for _, dsn := range c.DSN {
		conn, connErr := amqp.DialConfig(dsn, amqpConfig)
		if connErr == nil {
			c.Conn = conn
			break
		} else {
			err = connErr
		}
	}
	return err
}

func (c *Connect) Close() {
	close(c.closeCh)
	if c.Conn != nil && !c.Conn.IsClosed() {
		c.Conn.Close()
	}
	fmt.Println("RabbitMQ connection closed")
}
