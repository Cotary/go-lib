package rabbitMQ

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	e "github.com/Cotary/go-lib/err"
	"os"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type Config struct {
	DSN        []string `mapstructure:"dsn" yaml:"dsn"`
	CA         string   `mapstructure:"ca" yaml:"ca"`
	CertFile   string   `mapstructure:"certFile" yaml:"certFile"`
	KeyFile    string   `mapstructure:"keyFile" yaml:"keyFile"`
	Heartbeat  int64    `mapstructure:"heartbeat" yaml:"heartbeat"`
	MaxChannel int      `mapstructure:"maxChannel" yaml:"maxChannel"`
}

func (cfg *Config) ensureDefaults() {
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 5
	}
	if cfg.MaxChannel == 0 {
		cfg.MaxChannel = 1000
	}
}

type Connect struct {
	mu        sync.Mutex
	conn      *amqp091.Connection
	cfg       Config
	pool      chan *amqp091.Channel
	closeCh   chan struct{} // 主动 Close 的信号
	closeOnce sync.Once     // 确保 Close() 幂等
}

func NewRabbitMQ(cfg Config) (*Connect, error) {
	cfg.ensureDefaults()
	c := &Connect{
		cfg:     cfg,
		closeCh: make(chan struct{}),
	}
	// 首次建链 + 初始化池
	if err := c.reconnect(); err != nil {
		return nil, fmt.Errorf("initial rabbitmq connect: %w", err)
	}
	// 只 spawn 一次断线监控
	coroutines.SafeGo(coroutines.NewContext("mq healthy check"), func(ctx context.Context) {
		c.watchDisconnect(ctx)
	})
	return c, nil
}

// watchDisconnect 监控底层连接断开，并对网络异常进行重连。
// 收到客户端主动关闭（err == nil 或 closeErrCh 被关闭）时，会优雅退出。
func (c *Connect) watchDisconnect(ctx context.Context) {
	for {
		// 获取当前连接实例
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		// 如果连接尚未建立，等待或退出
		if conn == nil {
			select {
			case <-time.After(1 * time.Second):
				continue
			case <-c.closeCh:
				return
			}
		}

		closeErrCh := make(chan *amqp091.Error, 1)
		conn.NotifyClose(closeErrCh)

		select {
		case <-c.closeCh:
			return

		case err, ok := <-closeErrCh:
			if !ok {
				return
			}
			if err != nil {
				e.SendMessage(ctx, err)
			}
		}

		attempt := 0
		for {
			select {
			case <-c.closeCh:
				return
			default:
			}
			if err := c.reconnect(); err == nil {
				break
			}
			time.Sleep(backoff(attempt))
			attempt++
		}
		// 重连成功之后，回到外层循环，重新注册 NotifyClose
	}
}

// reconnect 只做断链 & 新建 & 链接 & 池重建
func (c *Connect) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	drainPool(c.pool)
	c.pool = nil

	if c.conn != nil && !c.conn.IsClosed() {
		c.conn.Close()
	}
	c.conn = nil

	amqpCfg := amqp091.Config{
		Heartbeat: time.Duration(c.cfg.Heartbeat) * time.Second,
	}
	if c.cfg.CA != "" || (c.cfg.CertFile != "" && c.cfg.KeyFile != "") {
		tlsCfg := &tls.Config{}
		if c.cfg.CA != "" {
			caPEM, err := os.ReadFile(c.cfg.CA)
			if err != nil {
				return fmt.Errorf("read CA: %w", err)
			}
			tlsCfg.RootCAs = x509.NewCertPool()
			tlsCfg.RootCAs.AppendCertsFromPEM(caPEM)
		}
		if c.cfg.CertFile != "" && c.cfg.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.cfg.CertFile, c.cfg.KeyFile)
			if err != nil {
				return fmt.Errorf("load client cert: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		amqpCfg.TLSClientConfig = tlsCfg
	}

	var lastErr error
	for _, dsn := range c.cfg.DSN {
		conn, err := amqp091.DialConfig(dsn, amqpCfg)
		if err == nil {
			c.conn = conn
			break
		}
		lastErr = err
	}
	if c.conn == nil {
		return fmt.Errorf("dial rabbitmq: %w", lastErr)
	}

	c.pool = make(chan *amqp091.Channel, c.cfg.MaxChannel)
	return nil
}

// GetCh 从池里拿 channel，坏的跳过，空时新建
func (c *Connect) GetCh() (*amqp091.Channel, error) {
	c.mu.Lock()
	conn := c.conn
	pool := c.pool
	c.mu.Unlock()
	if conn == nil || conn.IsClosed() {
		return nil, errors.New("rabbitmq connection is closed")
	}

	for {
		select {
		case ch := <-pool:
			if ch != nil && !ch.IsClosed() {
				return ch, nil
			}
		default:
			return conn.Channel()
		}
	}
}

// PutCh 归还 channel，池满或已关闭则关掉
func (c *Connect) PutCh(ch *amqp091.Channel) {
	if ch == nil || ch.IsClosed() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			_ = ch.Close()
		}
	}()
	c.mu.Lock()
	pool := c.pool
	c.mu.Unlock()
	if pool == nil {
		_ = ch.Close()
		return
	}
	select {
	case pool <- ch:
	default:
		_ = ch.Close()
	}
}

func (c *Connect) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.mu.Lock()
		defer c.mu.Unlock()
		drainPool(c.pool)
		c.pool = nil
		if c.conn != nil && !c.conn.IsClosed() {
			c.conn.Close()
			c.conn = nil
		}
	})
}

func drainPool(pool chan *amqp091.Channel) {
	if pool == nil {
		return
	}
	for {
		select {
		case ch := <-pool:
			if ch != nil {
				_ = ch.Close()
			}
		default:
			return
		}
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	const maxBackoff = 60 * time.Second
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}
