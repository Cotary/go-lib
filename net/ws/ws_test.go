package ws

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
	r.GET("/ws", func(c *gin.Context) {
		s.ServeHTTP(c.Writer, c.Request)
	})

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

// TestServer_Broadcast 验证服务端广播消息能送达所有客户端
func TestServer_Broadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srvCfg := Config{
		OnMessage: func(ctx context.Context, c *Conn, mt int, data []byte) {},
	}
	s := New(srvCfg)

	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		s.ServeHTTP(c.Writer, c.Request)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	u.Scheme = "ws"
	u.Path = "/ws"
	wsURL := u.String()

	const clientCount = 3
	received := make([]chan string, clientCount)

	var clients []*Client
	for i := 0; i < clientCount; i++ {
		idx := i
		received[idx] = make(chan string, 10)

		c := NewClient(wsURL)
		c.retryBase = 20 * time.Millisecond
		c.retryMax = 200 * time.Millisecond

		c.OnMessage(func(ctx context.Context, mt int, data []byte) {
			select {
			case received[idx] <- string(data):
			default:
			}
		})

		connected := make(chan struct{})
		c.OnConnect(func(ctx context.Context) {
			select {
			case connected <- struct{}{}:
			default:
			}
		})

		c.Start()
		defer c.Stop()
		clients = append(clients, c)

		select {
		case <-connected:
		case <-time.After(2 * time.Second):
			t.Fatalf("client %d did not connect", idx)
		}
	}

	// 等待服务端注册所有连接
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, clientCount, s.Count(), "server should have %d connections", clientCount)

	s.BroadcastText("hello-all")

	for i := 0; i < clientCount; i++ {
		select {
		case msg := <-received[i]:
			assert.Equal(t, "hello-all", msg)
		case <-time.After(2 * time.Second):
			t.Errorf("client %d did not receive broadcast", i)
		}
	}
}

// TestClient_ConcurrentSend 验证客户端并发发送不 panic
func TestClient_ConcurrentSend(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var serverMsgCount int32
	srvCfg := Config{
		OnMessage: func(ctx context.Context, c *Conn, mt int, data []byte) {
			atomic.AddInt32(&serverMsgCount, 1)
		},
	}
	s := New(srvCfg)

	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		s.ServeHTTP(c.Writer, c.Request)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	u.Scheme = "ws"
	u.Path = "/ws"
	wsURL := u.String()

	client := NewClient(wsURL)
	client.retryBase = 20 * time.Millisecond
	client.retryMax = 200 * time.Millisecond

	connected := make(chan struct{})
	client.OnConnect(func(ctx context.Context) {
		select {
		case connected <- struct{}{}:
		default:
		}
	})
	client.Start()
	defer client.Stop()

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect")
	}

	const goroutines = 10
	const msgsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < msgsPerGoroutine; i++ {
				_ = client.Send([]byte(fmt.Sprintf("g%d-m%d", id, i)))
			}
		}(g)
	}
	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	count := atomic.LoadInt32(&serverMsgCount)
	t.Logf("server received %d messages", count)
	assert.Greater(t, count, int32(0), "server should have received some messages")
}

// TestClient_Backoff 验证退避时间在预期范围内
func TestClient_Backoff(t *testing.T) {
	client := NewClient("ws://localhost:0")
	client.retryBase = 100 * time.Millisecond
	client.retryMax = 5 * time.Second

	tests := []struct {
		attempt int
		minMs   float64
		maxMs   float64
	}{
		{0, 50, 150},     // base=100ms, jitter 0.5x~1.5x => 50~150ms
		{1, 100, 300},    // base*2=200ms => 100~300ms
		{2, 200, 600},    // base*4=400ms => 200~600ms
		{3, 400, 1200},   // base*8=800ms => 400~1200ms
		{10, 2500, 7500}, // capped at 5s => 2500~7500ms
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			for trial := 0; trial < 50; trial++ {
				d := client.backoff(tt.attempt)
				ms := float64(d) / float64(time.Millisecond)
				assert.GreaterOrEqual(t, ms, tt.minMs,
					"attempt %d trial %d: %v too short", tt.attempt, trial, d)
				assert.LessOrEqual(t, ms, tt.maxMs,
					"attempt %d trial %d: %v too long", tt.attempt, trial, d)
			}
		})
	}
}
