package rabbitMQ

import (
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"sync"
)

type ChannelPool struct {
	conn    *Connect
	pool    chan *amqp.Channel
	mu      sync.Mutex
	maxSize int
}

// NewChannelPool 创建新的通道池
func NewChannelPool(conn *Connect, maxSize int) (*ChannelPool, error) {
	pool := make(chan *amqp.Channel, maxSize)
	for i := 0; i < maxSize; i++ {
		ch, err := conn.Conn.Channel()
		if err != nil {
			return nil, err
		}
		pool <- ch
	}
	return &ChannelPool{
		conn:    conn,
		pool:    pool,
		maxSize: maxSize,
	}, nil
}

// Get 从通道池中获取一个通道
func (p *ChannelPool) Get() (*amqp.Channel, error) {
	if p.conn.Conn.IsClosed() {
		return nil, errors.New("connection is closed")
	}
	select {
	case ch := <-p.pool:
		if err := ch.Flow(true); err != nil { //这里报错了，这个channel 就自动关闭了
			return p.Get()
		}
		return ch, nil
	default:
		return p.conn.Conn.Channel()
	}
}

// Put 将通道归还到通道池中
func (p *ChannelPool) Put(ch *amqp.Channel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pool) < p.maxSize {
		p.pool <- ch
	} else {
		ch.Close()
	}
}

// Close 关闭通道池中的所有通道
func (p *ChannelPool) Close() {
	defer p.conn.Close()
	close(p.pool)
	for ch := range p.pool {
		ch.Close()
	}
}
