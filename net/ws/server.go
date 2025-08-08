package ws

import (
	"context"
	"errors"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/log"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
	"sync/atomic"
	"time"
)

const defaultWriteTimeout = 5 * time.Second

type frame struct {
	mt   int
	data []byte
	done chan error // 如果需要同步返回写结果
}

type Conn struct {
	ws         *websocket.Conn
	cfg        Config
	send       chan frame
	closed     int32
	lastActive atomic.Int64
}

func newConn(cfg Config, ws *websocket.Conn) *Conn {
	c := &Conn{
		ws:   ws,
		cfg:  cfg,
		send: make(chan frame, 256),
	}
	c.lastActive.Store(time.Now().UnixNano())
	return c
}

func (c *Conn) run() {
	defer c.Close(nil)

	go c.writePump() // 启动单写协程

	// 读设置
	c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	c.ws.SetPongHandler(func(string) error {
		c.lastActive.Store(time.Now().UnixNano())
		return c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	})

	for {
		mt, msg, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		if c.cfg.OnMessage != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnMessage"), func(ctx context.Context) {
				c.cfg.OnMessage(ctx, c, mt, msg)
			})
		}
	}
}

// 写泵，只允许一个 goroutine 写 socket
func (c *Conn) writePump() {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer func() {
		ticker.Stop()
		_ = c.ws.Close()
	}()

	for {
		select {
		case f, ok := <-c.send:
			if !ok {
				// 通道关闭，发送 Close 帧再退出
				_ = c.ws.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
				_ = c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
				return
			}

			_ = c.ws.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
			err := c.ws.WriteMessage(f.mt, f.data)

			if f.done != nil {
				select {
				case f.done <- err:
				default:
				}
			}
			if err != nil {
				c.Close(err)
				return
			}

		case <-ticker.C:
			// 心跳：也在写泵里完成
			err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			if err != nil {
				c.Close(err)
				return
			}
		}
	}
}

// 关闭连接（幂等）
func (c *Conn) Close(err error) {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	close(c.send) // 通知写泵关闭连接并发送 close 帧
	if c.cfg.OnClose != nil {
		coroutines.SafeFunc(coroutines.NewContext("WS OnClose"), func(ctx context.Context) {
			c.cfg.OnClose(ctx, c, err)
		})
	}
}

// ✅ 同步发送：等待结果确认
func (c *Conn) Send(mt int, data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	cp := append([]byte(nil), data...)
	done := make(chan error, 1)

	select {
	case c.send <- frame{mt: mt, data: cp, done: done}:
		return <-done
	case <-time.After(1 * time.Second):
		return errors.New("send timeout or blocked")
	}
}

func (c *Conn) SendText(text string) error {
	return c.Send(websocket.TextMessage, []byte(text))
}

// ✅ 异步发送（不等待写结果）
func (c *Conn) SendAsync(mt int, data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return errors.New("connection closed")
	}

	cp := append([]byte(nil), data...)

	select {
	case c.send <- frame{mt: mt, data: cp}:
		return nil
	default:
		return errors.New("send buffer full")
	}
}

// ----------------------------------------------------------------------

type Config struct {
	ReadTimeout  time.Duration                                           // Pong 超时
	PingInterval time.Duration                                           // 心跳间隔
	OnMessage    func(ctx context.Context, c *Conn, mt int, data []byte) // 收到消息回调
	OnConnect    func(ctx context.Context, c *Conn)                      // 连接建立回调
	OnClose      func(ctx context.Context, c *Conn, err error)           // 连接关闭回调
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

// Gin handler
func (s *Server) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.WithContext(context.Background()).WithFields(map[string]interface{}{
				"client ip": c.ClientIP(),
			}).Error(fmt.Sprintf("WS upgrade failed: %v", err))
			return
		}
		wsc := newConn(s.cfg, conn)
		coroutines.SafeGo(coroutines.NewContext("WS Server"), func(ctx context.Context) {
			wsc.run()
		})
		log.WithContext(context.Background()).WithFields(map[string]interface{}{
			"client ip": c.ClientIP(),
		}).Info(fmt.Sprintf("WS connection"))
		if s.cfg.OnConnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnConnect"), func(ctx context.Context) {
				s.cfg.OnConnect(ctx, wsc)
			})
		}
	}
}
