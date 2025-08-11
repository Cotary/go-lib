package ws

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 假定本包内已有：type Config struct{ OnMessage func(ctx context.Context, c *Conn, mt int, data []byte) }
// 以及：New(cfg Config) *Server, (*Server).Handler() gin.HandlerFunc
// 和：Conn.SendText(string) error, Conn.Close(error) 等。
// 同时已有：NewClient(url string, opts ...Option) *Client

func TestWebSocket_Reconnect_And_Continuous_Send_With_Close_Every_5(t *testing.T) {
	gin.SetMode(gin.TestMode)

	serverReceived := make(chan string, 1024)
	var serverCloseCount int32

	// 服务端：收到 "close" 立即断开；其他消息记录并 echo
	srvCfg := Config{
		OnMessage: func(ctx context.Context, c *Conn, mt int, data []byte) {
			msg := string(data)
			if strings.EqualFold(msg, "close") {
				atomic.AddInt32(&serverCloseCount, 1)
				c.Close(nil)
				return
			}
			select {
			case serverReceived <- msg:
			default:
				// 丢弃以避免堵塞
			}
			_ = c.SendText("echo: " + msg)
		},
	}
	s := New(srvCfg)

	r := gin.New()
	r.GET("/ws", s.Handler())

	ts := httptest.NewServer(r)
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws"
	wsURL := u.String()

	// 客户端
	client := NewClient(wsURL)
	// 加快重连速度，测试更快完成
	client.retryBase = 20 * time.Millisecond
	client.retryMax = 200 * time.Millisecond

	connected := make(chan struct{}, 16)
	disconnected := make(chan struct{}, 16)
	var connectCount int32
	var disconnectCount int32

	client.OnMessage(func(ctx context.Context, mt int, data []byte) {
		// 这里仅打印观察效果
		t.Logf("📩 client recv: [%d] %s", mt, string(data))
	})
	client.OnConnect(func(ctx context.Context) {
		atomic.AddInt32(&connectCount, 1)
		select {
		case connected <- struct{}{}:
		default:
		}
		t.Logf("🔌 connected (%d)", atomic.LoadInt32(&connectCount))
	})
	client.OnDisconnect(func(ctx context.Context, err error) {
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt32(&disconnectCount, 1)
		select {
		case disconnected <- struct{}{}:
		default:
		}
		t.Logf("💥 disconnected (%d), err=%v", atomic.LoadInt32(&disconnectCount), err)
	})

	client.Start()
	defer client.Stop()

	// 等待首次连接
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect initially")
	}

	// 启动持续发送：每隔 5 条发送一个 "close"
	runDur := 3 * time.Second
	sendTick := 30 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), runDur)
	defer cancel()

	go func() {
		ticker := time.NewTicker(sendTick)
		defer ticker.Stop()
		i := 1
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var msg string
				if i%5 == 0 {
					msg = "close"
				} else {
					msg = fmt.Sprintf("msg-%03d", i)
				}
				err := client.Send([]byte(msg))
				if err != nil {
					// 断线期间会报错，等待重连后继续；这里只打印观察效果
					t.Logf("➡️ send %q err: %v", msg, err)
				} else {
					t.Logf("➡️ send %q ok", msg)
				}
				i++
			}
		}
	}()

	// 采样读取服务端接收的部分消息，用于观察效果
	collected := make([]string, 0, 64)
CollectLoop:
	for {
		select {
		case m := <-serverReceived:
			collected = append(collected, m)
			if len(collected) >= 20 {
				// 采样到一定数量就不再阻塞主流程
				break CollectLoop
			}
		case <-ctx.Done():
			break CollectLoop
		}
	}

	// 等待发送循环结束
	<-ctx.Done()

	// 基础断言：应该发生了多次断开/重连，以及服务端确实因为 "close" 断过连接
	cc := atomic.LoadInt32(&connectCount)
	dc := atomic.LoadInt32(&disconnectCount)
	sc := atomic.LoadInt32(&serverCloseCount)

	t.Logf("summary: connects=%d, disconnects=%d, serverCloseCount=%d, sampleReceived=%d",
		cc, dc, sc, len(collected))
	for i, m := range collected {
		t.Logf("server-recv[%02d]=%s", i, m)
	}

	// 至少发生过 1 次断开与 1 次重连（通常会多次）
	assert.GreaterOrEqual(t, dc, int32(1), "should have at least one disconnection")
	assert.GreaterOrEqual(t, cc, int32(2), "should have reconnected at least once")
	// 服务端至少处理过一次 'close'
	assert.GreaterOrEqual(t, sc, int32(1), "server should have closed at least once due to 'close'")
	// 服务端应当收到不少普通消息（close 不会入列）
	assert.GreaterOrEqual(t, len(collected), 5, "server should have received multiple normal messages")
}
