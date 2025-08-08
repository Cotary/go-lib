package ws

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// 出站消息：统一管理异步/同步发送与心跳写入
type outMsg struct {
	msgType int
	data    []byte
	result  chan error // nil 表示异步消息或心跳
}

// Client 提供自动重连、异步/同步发送、心跳保活与消息回调的 WS 客户端
type Client struct {
	url    string
	dialer *websocket.Dialer

	// 连接与状态
	mu          sync.RWMutex
	conn        *websocket.Conn
	isRunning   int32
	isConnected int32

	// 出站队列（统一）
	outCh chan outMsg

	// 控制
	done       chan struct{}  // 全局停止
	wg         sync.WaitGroup // 等待 run 退出
	pingTicker *time.Ticker   // 每连接的心跳定时器（由连接周期管理）

	// 配置
	pongWait     time.Duration // 读超时：收到 Pong 后刷新的窗口
	pingInterval time.Duration // 心跳间隔
	writeTimeout time.Duration // 写超时
	outQueueSize int           // 出站队列容量
	retryBase    time.Duration // 重连基础等待
	retryMax     time.Duration // 重连最大等待
	onMessage    func(ctx context.Context, messageType int, data []byte)
	onConnect    func(ctx context.Context)
	onDisconnect func(ctx context.Context, err error)
}

// NewClient 创建客户端，使用合理默认值
func NewClient(url string) *Client {
	return &Client{
		url:          url,
		dialer:       websocket.DefaultDialer,
		outCh:        make(chan outMsg, 1024),
		done:         make(chan struct{}),
		pongWait:     60 * time.Second,
		pingInterval: 20 * time.Second,
		writeTimeout: 5 * time.Second,
		outQueueSize: 1024,
		retryBase:    1 * time.Second,
		retryMax:     30 * time.Second,
	}
}

// 可选回调
func (c *Client) OnMessage(fn func(context.Context, int, []byte)) { c.onMessage = fn }
func (c *Client) OnConnect(fn func(context.Context))              { c.onConnect = fn }
func (c *Client) OnDisconnect(fn func(context.Context, error))    { c.onDisconnect = fn }

func (c *Client) IsRunning() bool   { return atomic.LoadInt32(&c.isRunning) == 1 }
func (c *Client) IsConnected() bool { return atomic.LoadInt32(&c.isConnected) == 1 }

// Start 启动客户端（幂等）
func (c *Client) Start() {
	if !atomic.CompareAndSwapInt32(&c.isRunning, 0, 1) {
		return
	}
	c.wg.Add(1)
	coroutines.SafeGo(coroutines.NewContext("WS start"), func(ctx context.Context) {
		c.run()
	})
}

// Stop 优雅停止（幂等）
func (c *Client) Stop() {
	if !atomic.CompareAndSwapInt32(&c.isRunning, 1, 0) {
		return
	}
	close(c.done)
	c.wg.Wait()
	c.closeConn()
}

// SendAsync 异步发送文本消息：断线时会入队等待重连后发送
func (c *Client) SendAsync(data []byte) error {
	return c.enqueue(outMsg{
		msgType: websocket.TextMessage,
		data:    data,
		// result 为 nil 表示异步
	})
}

// Send 同步发送文本消息：等待写入当前连接的结果（成功/失败）
// 注意：仅确认写入是否成功，不代表对端业务处理成功。
func (c *Client) Send(data []byte) error {
	res := make(chan error, 1)
	if err := c.enqueue(outMsg{
		msgType: websocket.TextMessage,
		data:    data,
		result:  res,
	}); err != nil {
		return err
	}
	select {
	case err := <-res:
		return err
	case <-c.done:
		return context.Canceled
	}
}

// 内部：入队（需要客户端处于运行状态；不要求已连接）
// 队列满返回错误；Stop 后返回 context.Canceled
func (c *Client) enqueue(m outMsg) error {
	if !c.IsRunning() {
		return fmt.Errorf("client is not running")
	}
	select {
	case c.outCh <- m:
		return nil
	case <-c.done:
		return context.Canceled
	default:
		return fmt.Errorf("send queue is full")
	}
}

// 主循环：连接 -> 连接内循环（读/写/心跳）-> 断开 -> 指数退避重连
func (c *Client) run() {
	defer c.wg.Done()
	rand.Seed(time.Now().UnixNano())

	var attempt int
	for {
		select {
		case <-c.done:
			return
		default:
		}

		conn, err := c.connect()
		if err != nil {
			wait := c.backoff(attempt)
			attempt++
			log.WithContext(context.Background()).WithFields(map[string]interface{}{
				"url": c.url,
			}).Error(fmt.Sprintf("WS connect failed: %v, retry in %s", err, wait))
			if !c.waitOrDone(wait) {
				return
			}
			continue
		}
		attempt = 0 // reset on success

		atomic.StoreInt32(&c.isConnected, 1)
		if c.onConnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS OnConnect"), func(ctx context.Context) {
				c.onConnect(ctx)
			})
		}

		// 启动 per-connection 读、写、心跳
		connClosed := make(chan struct{})
		var wgConn sync.WaitGroup
		wgConn.Add(2)
		c.pingTicker = time.NewTicker(c.pingInterval)

		coroutines.SafeGo(coroutines.NewContext("WS readLoop"), func(ctx context.Context) {
			c.readLoop(conn, connClosed, &wgConn)
		})
		coroutines.SafeGo(coroutines.NewContext("WS writeLoop"), func(ctx context.Context) {
			c.writeLoop(conn, connClosed, &wgConn)
		})

		// 等待连接结束或全局停止
		select {
		case <-connClosed:
		case <-c.done:
		}

		// 清理连接资源
		if c.pingTicker != nil {
			c.pingTicker.Stop()
			c.pingTicker = nil
		}
		c.closeSpecificConn(conn)
		wgConn.Wait()

		atomic.StoreInt32(&c.isConnected, 0)
		if c.onDisconnect != nil {
			coroutines.SafeFunc(coroutines.NewContext("WS onDisconnect"), func(ctx context.Context) {
				c.onDisconnect(ctx, err)
			})
		}

		// 若全局停止则退出
		select {
		case <-c.done:
			return
		default:
		}

		// 小等待后重连（避免忙重连），也可立即 backoff
		wait := c.backoff(0)
		if !c.waitOrDone(wait) {
			return
		}
	}
}

// 建立连接并配置 Pong/超时
func (c *Client) connect() (*websocket.Conn, error) {
	conn, _, err := c.dialer.Dial(c.url, nil)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(c.pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(c.pongWait))
	})

	c.mu.Lock()
	// 关闭旧连接（理论上不应存在，但保险处理）
	if c.conn != nil && c.conn != conn {
		_ = c.conn.Close()
	}
	c.conn = conn
	c.mu.Unlock()

	return conn, nil
}

// 读循环：读取消息、触发回调；出错即关闭 connClosed
func (c *Client) readLoop(conn *websocket.Conn, connClosed chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		// 若全局停止，直接退出
		select {
		case <-c.done:
			return
		default:
		}
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			log.WithContext(context.Background()).WithFields(map[string]interface{}{
				"url": c.url,
			}).Error(fmt.Sprintf("WS read error: %v", err))
			closeOnce(connClosed)
			return
		}
		if c.onMessage != nil {
			mtCopy := mt
			dataCopy := append([]byte(nil), msg...) // 避免复用底层缓冲
			coroutines.SafeFunc(coroutines.NewContext("WS onMessage"), func(ctx context.Context) {
				c.onMessage(ctx, mtCopy, dataCopy)
			})
		}
	}
}

// 写循环：统一从 outCh 读取异步/同步/心跳消息并写入当前连接
func (c *Client) writeLoop(conn *websocket.Conn, connClosed chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-c.done:
			return
		case <-connClosed:
			// 读侧已判定连接失效，写侧应立即退出，交由 run() 做清理与重连
			return
		case <-c.pingTicker.C:
			// 心跳写入：队列可能满时直接尝试即时写，减少阻塞
			if err := c.writeOne(conn, websocket.PingMessage, nil); err != nil {
				log.WithContext(context.Background()).WithFields(map[string]interface{}{
					"url": c.url,
				}).Error(fmt.Sprintf("WS ping error: %v", err))
				closeOnce(connClosed)
				return
			}
		case m := <-c.outCh:
			if err := c.writeOne(conn, m.msgType, m.data); err != nil {
				// 将错误回传给同步发送者（如果有）
				if m.result != nil {
					select {
					case m.result <- err:
					default:
					}
				}
				log.WithContext(context.Background()).WithFields(map[string]interface{}{
					"url": c.url,
				}).Error(fmt.Sprintf("WS write error: %v", err))

				closeOnce(connClosed)
				return
			}
			// 成功写入，告知同步发送者
			if m.result != nil {
				select {
				case m.result <- nil:
				default:
				}
			}
		}
	}
}

// 写入单帧（带超时）
func (c *Client) writeOne(conn *websocket.Conn, msgType int, data []byte) error {
	_ = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	return conn.WriteMessage(msgType, data)
}

// 关闭特定连接（避免受 c.conn 切换影响）
func (c *Client) closeSpecificConn(conn *websocket.Conn) {
	if conn != nil {
		_ = conn.Close()
	}
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	c.mu.Unlock()
}

// 关闭当前连接（Stop 阶段调用）
func (c *Client) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// 等待一段时间或收到停止信号
func (c *Client) waitOrDone(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-c.done:
		return false
	}
}

// 指数退避 + 抖动
func (c *Client) backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := c.retryBase * (1 << attempt)
	if d > c.retryMax {
		d = c.retryMax
	}
	// 抖动：0.5x ~ 1.5x
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(d) * jitter)
}

// 安全关闭（只关一次）
func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
		// already closed
	default:
		close(ch)
	}
}
