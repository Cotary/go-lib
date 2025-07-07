package ws

import (
	"context"
	"github.com/Cotary/go-lib/common/coroutines"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client 封装 WebSocket 客户端
type Client struct {
	url           string
	dialer        *websocket.Dialer
	conn          *websocket.Conn
	mu            sync.RWMutex
	writeMu       sync.Mutex
	sendQueue     chan []byte
	pongWait      time.Duration
	pingInterval  time.Duration
	reconnectWait time.Duration
	onMessage     func(messageType int, data []byte)
	done          chan struct{}
	wg            sync.WaitGroup
}

// NewWSClient 构造
func NewWSClient(url string) *Client {
	return &Client{
		url:           url,
		dialer:        websocket.DefaultDialer,
		sendQueue:     make(chan []byte, 1024),
		pongWait:      60 * time.Second,
		pingInterval:  20 * time.Second,
		reconnectWait: 5 * time.Second,
		done:          make(chan struct{}),
	}
}

// RegisterOnMessage 注册消息回调
func (c *Client) RegisterOnMessage(fn func(int, []byte)) {
	c.onMessage = fn
}

// Send 发送消息（断线时排队）
func (c *Client) Send(data []byte) {
	select {
	case c.sendQueue <- data:
	case <-c.done:
	}
}

// Start 启动
func (c *Client) Start() {
	c.wg.Add(1)
	go c.run()
}

// Stop 停止
func (c *Client) Stop() {
	close(c.done)
	c.wg.Wait()
}

// run 管理生命周期
func (c *Client) run() {
	defer c.wg.Done()
	for {
		// 停止信号优先
		select {
		case <-c.done:
			return
		default:
		}

		if err := c.connect(); err != nil {
			log.Printf("连接失败: %v, %s后重试...", err, c.reconnectWait)
			if !c.waitOrDone(c.reconnectWait) {
				return
			}
			continue
		}

		// per-connection 重连信号
		reconnect := make(chan struct{}, 1)
		var once sync.Once
		triggerReconnect := func() {
			once.Do(func() { reconnect <- struct{}{} })
		}

		// 启动读写心跳
		var wgConn sync.WaitGroup
		wgConn.Add(3)
		go c.readLoop(&wgConn, reconnect, triggerReconnect)
		go c.writeLoop(&wgConn, reconnect, triggerReconnect)
		go c.pingLoop(&wgConn, reconnect, triggerReconnect)

		// 等待重连或停止
		select {
		case <-c.done:
		case <-reconnect:
		}

		c.closeConn()
		wgConn.Wait()
		log.Println("连接断开，重连中...")

		if !c.waitOrDone(c.reconnectWait) {
			return
		}
	}
}

// connect 建立连接并设置 Pong 处理
func (c *Client) connect() error {
	conn, _, err := c.dialer.Dial(c.url, nil)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(c.pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(c.pongWait))
		return nil
	})

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	log.Println("WebSocket 已连接")
	return nil
}

// readLoop 持续读
func (c *Client) readLoop(wg *sync.WaitGroup, reconnect chan struct{}, trigger func()) {
	defer wg.Done()
	for {
		select {
		case <-c.done:
			return
		case <-reconnect:
			return
		default:
		}
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			trigger()
			return
		}
		if c.onMessage != nil {
			coroutines.SafeFunc(coroutines.NewContext("readLoop"), func(ctx context.Context) {
				c.onMessage(mt, msg)
			})
		}
	}
}

// writeLoop 持续写
func (c *Client) writeLoop(wg *sync.WaitGroup, reconnect chan struct{}, trigger func()) {
	defer wg.Done()
	for {
		select {
		case <-c.done:
			return
		case <-reconnect:
			return
		case data := <-c.sendQueue:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()
			if conn == nil {
				// 断线时重排队
				go func(d []byte) { c.sendQueue <- d }(data)
				trigger()
				return
			}
			if err := c.safeWrite(websocket.TextMessage, data, 5*time.Second); err != nil {
				log.Println("写入失败:", err)
				go func(d []byte) { c.sendQueue <- d }(data)
				trigger()
				return
			}
		}
	}
}

// pingLoop 定时心跳
func (c *Client) pingLoop(wg *sync.WaitGroup, reconnect chan struct{}, trigger func()) {
	defer wg.Done()
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-reconnect:
			return
		case <-ticker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()
			if conn == nil {
				continue
			}
			if err := c.safeWrite(websocket.TextMessage, nil, 5*time.Second); err != nil {
				log.Println("Ping 发送失败:", err)
				trigger()
				return
			}
		}
	}
}

func (c *Client) safeWrite(msgType int, data []byte, deadline time.Duration) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(deadline))
	return c.conn.WriteMessage(msgType, data)
}

// waitOrDone 等待 d 时长或停止信号
func (c *Client) waitOrDone(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-c.done:
		return false
	}
}

// closeConn 安全关闭
func (c *Client) closeConn() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
}
