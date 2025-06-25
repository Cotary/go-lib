package rabbitMQ

import (
	"sync"

	e "github.com/Cotary/go-lib/err"
	"github.com/pkg/errors"
	"github.com/rabbitmq/amqp091-go"
)

type ChannelPool struct {
	conn    *Connect
	pool    chan *amqp091.Channel
	maxSize int
	mu      sync.Mutex
}

func NewChannelPool(conn *Connect, maxSize int) (*ChannelPool, error) {
	pool := &ChannelPool{
		conn:    conn,
		pool:    make(chan *amqp091.Channel, maxSize),
		maxSize: maxSize,
	}
	return pool.reset()
}

// reset 关闭旧 channel 并重建池
func (p *ChannelPool) reset() (*ChannelPool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// close existing
	if p.pool != nil {
		close(p.pool)
		for ch := range p.pool {
			ch.Close()
		}
	}
	// new buffered channel
	buf := make(chan *amqp091.Channel, p.maxSize)
	for i := 0; i < p.maxSize; i++ {
		ch, err := p.conn.Conn.Channel()
		if err != nil {
			// 失败则全部清理
			for c := range buf {
				c.Close()
			}
			return nil, e.Err(err)
		}
		buf <- ch
	}
	p.pool = buf
	return p, nil
}

func (p *ChannelPool) Get() (*amqp091.Channel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn.Conn.IsClosed() {
		return nil, errors.New("connection is closed")
	}
	select {
	case ch := <-p.pool:
		return ch, nil
	default:
		return p.conn.Conn.Channel()
	}
}

func (p *ChannelPool) Put(ch *amqp091.Channel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ch == nil {
		return
	}
	if p.pool == nil {
		ch.Close()
		return
	}
	select {
	case p.pool <- ch:
	default:
		ch.Close()
	}
}

func (p *ChannelPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool != nil {
		close(p.pool)
		for ch := range p.pool {
			ch.Close()
		}
		p.pool = nil
	}
}
