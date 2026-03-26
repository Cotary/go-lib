package ws

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

const defaultWriteTimeout = 5 * time.Second

// ------------ Config ------------

type Config struct {
	ReadTimeout  time.Duration
	PingInterval time.Duration
	WriteTimeout time.Duration

	// OnUpgrade 在 WebSocket 升级成功后立即调用（OnConnect 之前）
	// 可用于从 http.Request 中提取信息并存储到 conn.Set()
	OnUpgrade func(conn *Conn, r *http.Request)

	OnMessage func(ctx context.Context, conn *Conn, mt int, data []byte)
	OnConnect func(ctx context.Context, conn *Conn)
	OnClose   func(ctx context.Context, conn *Conn, err error)
}

func (cfg *Config) withDefaults() Config {
	c := *cfg
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 60 * time.Second
	}
	if c.PingInterval == 0 {
		c.PingInterval = 20 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	return c
}

// ------------ Server ------------

type Server struct {
	cfg      Config
	upgrader websocket.Upgrader

	mu    sync.RWMutex
	conns map[*Conn]struct{}
}

func New(cfg Config, upgraders ...websocket.Upgrader) *Server {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true }, // 生产建议按 origin 验证
	}
	if len(upgraders) > 0 {
		upgrader = upgraders[0]
	}

	return &Server{
		cfg:      cfg.withDefaults(),
		upgrader: upgrader,
		conns:    make(map[*Conn]struct{}),
	}
}

// Upgrade 执行 WebSocket 升级并返回新连接
func (s *Server) upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	conn := newConn(s, ws)
	s.addConn(conn)

	if s.cfg.OnUpgrade != nil {
		s.cfg.OnUpgrade(conn, r)
	}
	return conn, nil
}

// ServeHTTP 实现 http.Handler 接口，可直接用于标准库
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrade(w, r)
	if err != nil {
		return
	}
	if s.cfg.OnConnect != nil {
		coroutines.SafeFunc(coroutines.NewContext("WS OnConnect"), func(ctx context.Context) {
			s.cfg.OnConnect(ctx, conn)
		})
	}
	conn.run()
}

// Config 返回服务器配置
func (s *Server) Config() Config {
	return s.cfg
}

func (s *Server) addConn(conn *Conn) {
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeConn(conn *Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

// Count 返回当前活跃连接数
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conns)
}

// Broadcast 向所有连接广播消息
func (s *Server) Broadcast(mt int, data []byte) {
	s.mu.RLock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	for _, c := range conns {
		_ = c.Send(mt, data)
	}
}

// BroadcastText 向所有连接广播文本消息
func (s *Server) BroadcastText(text string) {
	s.Broadcast(websocket.TextMessage, []byte(text))
}

// CloseAll 关闭所有连接
func (s *Server) CloseAll() {
	s.mu.RLock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	for _, c := range conns {
		c.Close(nil)
	}
}

// ForEach 遍历所有连接执行操作
func (s *Server) ForEach(fn func(*Conn)) {
	s.mu.RLock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	for _, c := range conns {
		fn(c)
	}
}

// ------------ Conn ------------

type Conn struct {
	server *Server
	ws     *websocket.Conn
	closed int32

	writeMu   sync.Mutex
	closeOnce sync.Once
	stopCh    chan struct{}

	mu   sync.RWMutex
	data map[string]any
}

func newConn(server *Server, ws *websocket.Conn) *Conn {
	return &Conn{
		server: server,
		ws:     ws,
		stopCh: make(chan struct{}),
		data:   make(map[string]any),
	}
}

// Server 返回所属的服务器实例
func (c *Conn) Server() *Server {
	return c.server
}

// Set 存储自定义数据
func (c *Conn) Set(key string, value any) {
	c.mu.Lock()
	c.data[key] = value
	c.mu.Unlock()
}

// Get 获取自定义数据
func (c *Conn) Get(key string) (any, bool) {
	c.mu.RLock()
	v, ok := c.data[key]
	c.mu.RUnlock()
	return v, ok
}

// MustGet 获取自定义数据，不存在时 panic
func (c *Conn) MustGet(key string) any {
	v, ok := c.Get(key)
	if !ok {
		panic("key not found: " + key)
	}
	return v
}

// GetString 获取字符串类型的数据
func (c *Conn) GetString(key string) string {
	if v, ok := c.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c *Conn) run() {
	cfg := c.server.cfg
	var closeErr error
	defer func() { c.Close(closeErr) }()

	_ = c.ws.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))

	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
	})

	c.ws.SetPingHandler(func(appData string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		if atomic.LoadInt32(&c.closed) == 1 {
			return nil
		}
		deadline := time.Now().Add(cfg.WriteTimeout)
		return c.ws.WriteControl(websocket.PongMessage, []byte(appData), deadline)
	})

	coroutines.SafeGo(coroutines.NewContext("WS pingLoop"), func(ctx context.Context) {
		c.pingLoop()
	})

	for {
		mt, msg, err := c.ws.ReadMessage()
		if err != nil {
			closeErr = err
			break
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
		if cfg.OnMessage != nil {
			cp := append([]byte(nil), msg...)
			coroutines.SafeFunc(coroutines.NewContext("WS OnMessage"), func(ctx context.Context) {
				cfg.OnMessage(ctx, c, mt, cp)
			})
		}
	}
}

func (c *Conn) pingLoop() {
	cfg := c.server.cfg
	ticker := time.NewTicker(cfg.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.writeMu.Lock()
			if atomic.LoadInt32(&c.closed) == 1 {
				c.writeMu.Unlock()
				return
			}
			deadline := time.Now().Add(cfg.WriteTimeout)
			err := c.ws.WriteControl(websocket.PingMessage, nil, deadline)
			c.writeMu.Unlock()

			if err != nil {
				c.Close(err)
				return
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Conn) Close(err error) {
	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)
		close(c.stopCh)

		// 从服务器移除此连接
		c.server.removeConn(c)

		cfg := c.server.cfg

		c.writeMu.Lock()
		_ = c.ws.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
		_ = c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
		_ = c.ws.Close()
		c.writeMu.Unlock()

		if cfg.OnClose != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnClose"), func(ctx context.Context) {
				cfg.OnClose(ctx, c, err)
			})
		}
	})
}

// IsClosed 检查连接是否已关闭
func (c *Conn) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

func (c *Conn) Send(mt int, data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	cp := append([]byte(nil), data...)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	cfg := c.server.cfg
	_ = c.ws.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
	if err := c.ws.WriteMessage(mt, cp); err != nil {
		c.Close(err)
		return err
	}
	return nil
}

func (c *Conn) SendText(text string) error {
	return c.Send(websocket.TextMessage, []byte(text))
}

// SendBinary 发送二进制消息
func (c *Conn) SendBinary(data []byte) error {
	return c.Send(websocket.BinaryMessage, data)
}
