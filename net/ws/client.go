package ws

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/log"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type Client struct {
	url     string
	headers http.Header
	dialer  *websocket.Dialer

	isRunning   int32
	isConnected int32

	done chan struct{}
	wg   sync.WaitGroup

	pongWait     time.Duration
	pingInterval time.Duration
	writeTimeout time.Duration
	retryBase    time.Duration
	retryMax     time.Duration
	readLimit    int64

	rng *rand.Rand

	mu      sync.Mutex
	writeMu sync.Mutex
	conn    *websocket.Conn

	onMessage    func(ctx context.Context, messageType int, data []byte)
	onConnect    func(ctx context.Context)
	onDisconnect func(ctx context.Context, err error)
}

type Option func(*Client)

func NewClient(url string, opts ...Option) *Client {
	c := &Client{
		url:          url,
		dialer:       websocket.DefaultDialer,
		done:         make(chan struct{}),
		pongWait:     60 * time.Second,
		pingInterval: 20 * time.Second,
		writeTimeout: 5 * time.Second,
		retryBase:    1 * time.Second,
		retryMax:     30 * time.Second,
		readLimit:    0,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func WithHeaders(h http.Header) Option {
	return func(c *Client) {
		c.headers = h.Clone()
	}
}

func WithDialer(d *websocket.Dialer) Option {
	return func(c *Client) { c.dialer = d }
}

func WithRetry(base, max time.Duration) Option {
	return func(c *Client) {
		c.retryBase = base
		c.retryMax = max
	}
}

func WithReadLimit(limit int64) Option {
	return func(c *Client) { c.readLimit = limit }
}

func WithPingInterval(interval time.Duration) Option {
	return func(c *Client) { c.pingInterval = interval }
}

func WithPongWait(wait time.Duration) Option {
	return func(c *Client) { c.pongWait = wait }
}

func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *Client) { c.writeTimeout = timeout }
}

func (c *Client) OnMessage(fn func(context.Context, int, []byte)) { c.onMessage = fn }
func (c *Client) OnConnect(fn func(context.Context))              { c.onConnect = fn }
func (c *Client) OnDisconnect(fn func(context.Context, error))    { c.onDisconnect = fn }

func (c *Client) IsRunning() bool   { return atomic.LoadInt32(&c.isRunning) == 1 }
func (c *Client) IsConnected() bool { return atomic.LoadInt32(&c.isConnected) == 1 }

func (c *Client) Start() {
	if !atomic.CompareAndSwapInt32(&c.isRunning, 0, 1) {
		return
	}

	c.wg.Add(1)
	coroutines.SafeGo(coroutines.NewContext("WS start"), func(ctx context.Context) {
		c.run()
	})
}

func (c *Client) Stop() {
	if !atomic.CompareAndSwapInt32(&c.isRunning, 1, 0) {
		return
	}

	c.mu.Lock()
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.mu.Unlock()

	c.wg.Wait()
	c.closeConn()
}

func (c *Client) Send(data []byte) error {
	if !c.IsConnected() {
		return errors.New("not connected")
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("no connection")
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) run() {
	defer c.wg.Done()
	var attempt int

	for {
		c.mu.Lock()
		done := c.done
		c.mu.Unlock()
		select {
		case <-done:
			return
		default:
		}

		conn, err := c.connect()
		if err != nil {
			wait := c.backoff(attempt)
			attempt++
			log.WithContext(context.Background()).WithFields(map[string]interface{}{"url": c.url}).
				Error(fmt.Sprintf("WS connect failed: %v, retry in %s", err, wait))
			if !c.waitOrDone(wait) {
				return
			}
			continue
		}
		attempt = 0
		atomic.StoreInt32(&c.isConnected, 1)

		if c.onConnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnConnect"), func(ctx context.Context) {
				c.onConnect(ctx)
			})
		}

		ctxConn, cancelConn := context.WithCancel(context.Background())
		errConn := make(chan error, 1)

		var wgConn sync.WaitGroup
		wgConn.Add(1)
		coroutines.SafeGo(coroutines.NewContext("WS readLoop"), func(ctx context.Context) {
			c.readLoop(ctxConn, conn, errConn, &wgConn)
		})

		wgConn.Add(1)
		coroutines.SafeGo(coroutines.NewContext("WS pingLoop"), func(ctx context.Context) {
			c.pingLoop(ctxConn, conn, errConn, &wgConn)
		})

		c.mu.Lock()
		done = c.done
		c.mu.Unlock()
		var cause error
		select {
		case cause = <-errConn:
		case <-done:
			cause = context.Canceled
		}

		cancelConn()
		c.closeSpecificConn(conn)
		wgConn.Wait()

		atomic.StoreInt32(&c.isConnected, 0)
		if c.onDisconnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS onDisconnect"), func(ctx context.Context) {
				c.onDisconnect(ctx, cause)
			})
		}

		c.mu.Lock()
		done = c.done
		c.mu.Unlock()
		select {
		case <-done:
			return
		default:
		}

		if !c.waitOrDone(c.backoff(0)) {
			return
		}
	}
}

func (c *Client) connect() (*websocket.Conn, error) {
	c.mu.Lock()
	hdr := http.Header(nil)
	if c.headers != nil {
		hdr = c.headers.Clone()
	}
	c.mu.Unlock()

	conn, _, err := c.dialer.Dial(c.url, hdr)
	if err != nil {
		return nil, err
	}

	if c.readLimit > 0 {
		conn.SetReadLimit(c.readLimit)
	}

	c.mu.Lock()
	if c.conn != nil && c.conn != conn {
		old := c.conn
		c.mu.Unlock()

		c.writeMu.Lock()
		_ = old.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "switch"),
			time.Now().Add(2*time.Second),
		)
		c.writeMu.Unlock()
		_ = old.Close()

		c.mu.Lock()
	}
	c.conn = conn
	c.mu.Unlock()

	return conn, nil
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, errCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		if c.onMessage != nil {
			data := msg
			coroutines.SafeFunc(coroutines.NewContext("WS onMessage"), func(ctx context.Context) {
				c.onMessage(ctx, mt, data)
			})
		}
	}
}

func (c *Client) pingLoop(ctx context.Context, conn *websocket.Conn, errCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.writeMu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}
}

func (c *Client) closeSpecificConn(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	c.writeMu.Lock()
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
		time.Now().Add(2*time.Second),
	)
	c.writeMu.Unlock()
	_ = conn.Close()

	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	c.mu.Unlock()
}

func (c *Client) closeConn() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		c.writeMu.Lock()
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
			time.Now().Add(2*time.Second),
		)
		c.writeMu.Unlock()
		_ = conn.Close()
	}
}

func (c *Client) waitOrDone(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	c.mu.Lock()
	done := c.done
	c.mu.Unlock()

	select {
	case <-t.C:
		return true
	case <-done:
		return false
	}
}

func (c *Client) backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	baseTime := float64(c.retryBase)
	maxTime := float64(c.retryMax)

	// 指数增长并限幅
	d := baseTime * math.Pow(2, float64(attempt))
	if d > maxTime {
		d = maxTime
	}

	// 抖动 0.5x ~ 1.5x
	jitter := 0.5
	if c.rng != nil {
		jitter += c.rng.Float64()
	}
	d *= jitter

	if d <= 0 {
		d = baseTime
	}
	return time.Duration(d)
}
