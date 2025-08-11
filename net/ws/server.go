package ws

import (
	"context"
	"github.com/pkg/errors"

	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/log"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const defaultWriteTimeout = 5 * time.Second

type Conn struct {
	ws     *websocket.Conn
	cfg    Config
	closed int32

	writeMu   sync.Mutex
	closeOnce sync.Once
	stopCh    chan struct{} // 关闭时关闭此通道，停止 ping 协程
}

func newConn(cfg Config, ws *websocket.Conn) *Conn {
	return &Conn{
		ws:     ws,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

func (c *Conn) run() {
	// 确保 OnClose 能拿到真实错误原因
	var closeErr error
	defer c.Close(closeErr)

	// 初始读超时
	_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))

	// 收到 Pong 刷新读超时
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	})

	// 自定义 PingHandler：刷新读超时 + 在写锁内安全回 Pong，避免并发写
	c.ws.SetPingHandler(func(appData string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		if atomic.LoadInt32(&c.closed) == 1 {
			return nil
		}
		deadline := time.Now().Add(defaultWriteTimeout)
		return c.ws.WriteControl(websocket.PongMessage, []byte(appData), deadline)
	})

	// 定时 ping（与写操作共用一把锁，保证单写者）
	coroutines.SafeGo(coroutines.NewContext("WS pingLoop"), func(ctx context.Context) {
		c.pingLoop()
	})

	// 读循环
	for {
		mt, msg, err := c.ws.ReadMessage()
		if err != nil {
			closeErr = err
			break
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
		if c.cfg.OnMessage != nil {
			// 复制数据，避免异步处理时潜在复用问题
			cp := append([]byte(nil), msg...)
			coroutines.SafeFunc(coroutines.NewContext("WS OnMessage"), func(ctx context.Context) {
				c.cfg.OnMessage(ctx, c, mt, cp)
			})
		}
	}
}

func (c *Conn) pingLoop() {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.writeMu.Lock()
			if atomic.LoadInt32(&c.closed) == 1 {
				c.writeMu.Unlock()
				return
			}
			deadline := time.Now().Add(defaultWriteTimeout)
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

		// 尽量发送 Close 帧（持锁，保证与其他写串行）
		c.writeMu.Lock()
		_ = c.ws.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
		_ = c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
		_ = c.ws.Close()
		c.writeMu.Unlock()

		if c.cfg.OnClose != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnClose"), func(ctx context.Context) {
				c.cfg.OnClose(ctx, c, err)
			})
		}
	})
}

func (c *Conn) Send(mt int, data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	// 复制数据，避免外部修改
	cp := append([]byte(nil), data...)

	// 串行写 + 写超时
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	_ = c.ws.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	if err := c.ws.WriteMessage(mt, cp); err != nil {
		c.Close(err)
		return err
	}
	return nil
}

func (c *Conn) SendText(text string) error {
	return c.Send(websocket.TextMessage, []byte(text))
}

type Config struct {
	ReadTimeout  time.Duration
	PingInterval time.Duration
	OnMessage    func(ctx context.Context, conn *Conn, mt int, data []byte)
	OnConnect    func(ctx context.Context, conn *Conn, c *gin.Context)
	OnClose      func(ctx context.Context, conn *Conn, err error)
}

type Server struct {
	cfg      Config
	upgrader websocket.Upgrader
}

func New(cfg Config, upgraders ...websocket.Upgrader) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 60 * time.Second
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 20 * time.Second
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true }, // 生产建议按 origin 验证
	}
	if len(upgraders) > 0 {
		upgrader = upgraders[0]
	}
	return &Server{
		cfg:      cfg,
		upgrader: upgrader,
	}
}

// Handler 如果需要鉴权等，用 Gin 中间件在路由层统一处理
func (s *Server) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		cCopy := c.Copy()
		conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.WithContext(context.Background()).WithFields(map[string]interface{}{
				"client ip": c.ClientIP(),
			}).Error(fmt.Sprintf("WS upgrade failed: %v", err))
			return
		}
		wsc := newConn(s.cfg, conn)
		log.WithContext(context.Background()).WithFields(map[string]interface{}{
			"client ip": c.ClientIP(),
		}).Info("WS connection")

		// OnConnect 可能为 nil；异步使用 gin.Context 必须使用副本
		if s.cfg.OnConnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnConnect"), func(ctx context.Context) {
				s.cfg.OnConnect(ctx, wsc, cCopy)
			})
		}

		// 读写在后台运行
		coroutines.SafeGo(coroutines.NewContext("WS Server"), func(ctx context.Context) {
			wsc.run()
		})
	}
}
