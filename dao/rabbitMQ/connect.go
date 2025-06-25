package rabbitMQ

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	e "github.com/Cotary/go-lib/err"
	"github.com/rabbitmq/amqp091-go"
)

type Config struct {
	DSN        []string `yaml:"dsn"`
	CA         string   `yaml:"caPath"`
	ClientUser string   `yaml:"clientUserPath"`
	ClientKey  string   `yaml:"clientKeyPath"`
	Heartbeat  int64    `yaml:"heartbeat"`
	MaxChannel int      `yaml:"maxChannel"`
}

type Connect struct {
	mu       sync.Mutex
	Conn     *amqp091.Connection
	cfg      Config
	closeCh  chan struct{}
	chanPool *ChannelPool
}

func handleConfig(cfg *Config) {
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 30
	}
	if cfg.MaxChannel == 0 {
		cfg.MaxChannel = 2000
	}
}

func NewRabbitMQ(cfg Config) (*Connect, error) {
	handleConfig(&cfg)

	m := &Connect{
		cfg:     cfg,
		closeCh: make(chan struct{}),
	}
	if err := m.reconnect(); err != nil {
		return nil, e.Err(err)
	}

	// health check + auto‐rebuild pool
	ctx := coroutines.NewContext("rabbitmq-health")
	coroutines.SafeGo(ctx, func(ctx context.Context) {
		m.checkHealth(ctx)
	})
	return m, nil
}

func (m *Connect) checkHealth(ctx context.Context) {
	coroutines.SafeGo(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(m.cfg.Heartbeat) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.mu.Lock()
				closed := m.Conn.IsClosed()
				m.mu.Unlock()
				if closed {
					if err := m.reconnect(); err != nil {
						e.SendMessage(ctx, err)
					}
				}
			case <-m.closeCh:
				return
			}
		}
	})
}

func (m *Connect) reconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// close old
	if m.chanPool != nil {
		m.chanPool.Close()
		m.chanPool = nil
	}
	if m.Conn != nil && !m.Conn.IsClosed() {
		m.Conn.Close()
	}

	// prepare TLS
	var tlsCfg tls.Config
	if m.cfg.CA != "" {
		caPEM, err := os.ReadFile(m.cfg.CA)
		if err != nil {
			return fmt.Errorf("read CA file: %w", err)
		}
		tlsCfg.RootCAs = x509.NewCertPool()
		tlsCfg.RootCAs.AppendCertsFromPEM(caPEM)
	}
	if m.cfg.ClientUser != "" && m.cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(m.cfg.ClientUser, m.cfg.ClientKey)
		if err != nil {
			return fmt.Errorf("load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	cfg := amqp091.Config{
		TLSClientConfig: &tlsCfg,
		Heartbeat:       time.Duration(m.cfg.Heartbeat) * time.Second,
	}
	var lastErr error
	for _, dsn := range m.cfg.DSN {
		conn, err := amqp091.DialConfig(dsn, cfg)
		if err == nil {
			m.Conn = conn
			lastErr = nil
			break
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}

	// build new channel pool
	pool, err := NewChannelPool(m, m.cfg.MaxChannel)
	if err != nil {
		return err
	}
	m.chanPool = pool
	return nil
}

func (m *Connect) Pool() *ChannelPool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chanPool
}

func (m *Connect) Close() {
	close(m.closeCh)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.chanPool != nil {
		m.chanPool.Close()
	}
	if m.Conn != nil && !m.Conn.IsClosed() {
		m.Conn.Close()
	}
}
