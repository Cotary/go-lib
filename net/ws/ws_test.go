package ws

import (
	"context"
	"log"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketClientServerIntegration(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	serverReceived := make(chan string, 1)
	clientReceived := make(chan string, 1)
	connected := make(chan struct{}, 1)

	// 配置并启动服务端
	serverCfg := Config{
		OnMessage: func(ctx context.Context, c *Conn, mt int, data []byte) {
			// 记录服务端收到的消息，并回一条 echo
			txt := string(data)
			select {
			case serverReceived <- txt:
			default:
			}
			_ = c.SendText("echo: " + txt)
		},
		OnConnect: func(ctx context.Context, c *Conn) {
			// 可选：记录连接建立
		},
		OnClose: func(ctx context.Context, c *Conn, err error) {
			// 可选：记录关闭
		},
	}
	s := New(serverCfg)

	router := gin.New()
	router.GET("/ws", s.Handler())

	ts := httptest.NewServer(router)
	defer ts.Close()

	// 构造正确的 ws URL（含路径）
	u, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws"
	wsURL := u.String()

	// 启动客户端
	client := NewClient(wsURL)
	client.OnConnect(func(ctx context.Context) {
		// 通知连接建立
		select {
		case connected <- struct{}{}:
		default:
		}
	})
	client.OnDisconnect(func(ctx context.Context, err error) {
		// 可选：记录断开
	})
	client.OnMessage(func(ctx context.Context, mt int, data []byte) {
		// 客户端接收服务端 echo
		select {
		case clientReceived <- string(data):
		default:
		}
	})

	client.Start()
	defer client.Stop()

	// 等待连接建立（最多 2s）
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect to server in time")
	}

	// 发送一条消息并验证
	msg := []byte("hello websocket")
	err = client.Send(msg)
	assert.NoError(t, err, "client.Send should succeed after connection established")

	// 服务端应收到
	select {
	case got := <-serverReceived:
		assert.Equal(t, string(msg), got)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive client message in time")
	}

	// 客户端应收到 echo
	select {
	case echo := <-clientReceived:
		assert.Equal(t, "echo: "+string(msg), echo)
	case <-time.After(2 * time.Second):
		t.Fatal("client did not receive echo message in time")
	}
}

func TestWebSocket_Reconnect_And_Resend_Sync_Async(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 收集服务端收到的消息（跨连接）
	serverReceived := make(chan string, 32)

	// 服务端：收到 "CLOSE" 主动断开；其他消息正常 echo 并记录
	srvCfg := Config{
		OnMessage: func(ctx context.Context, c *Conn, mt int, data []byte) {
			msg := string(data)
			if msg == "CLOSE" {
				// 模拟服务端主动断开，触发客户端重连
				c.Close(nil)
				return
			}
			select {
			case serverReceived <- msg:
			default:
			}
			_ = c.SendText("echo: " + msg)
		},
	}
	s := New(srvCfg)

	r := gin.New()
	r.GET("/ws", s.Handler())

	ts := httptest.NewServer(r)
	defer ts.Close()

	// 正确构造 ws://host:port/ws
	u, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws"
	wsURL := u.String()

	// 客户端
	client := NewClient(wsURL)
	// 缩短重连退避，避免测试等待过久
	client.retryBase = 20 * time.Millisecond
	client.retryMax = 200 * time.Millisecond

	connected := make(chan struct{}, 4)
	disconnected := make(chan struct{}, 4)
	client.OnMessage(func(ctx context.Context, mt int, data []byte) {
		log.Printf("📩 收到消息: [%d] %s", mt, string(data))
		// 这里不必断言客户端的 echo，重点验证服务端收到/顺序与 Send 返回
	})
	client.OnConnect(func(ctx context.Context) {
		log.Println("🔌 客户端重连成功")
		select {
		case connected <- struct{}{}:
		default:
		}
	})
	client.OnDisconnect(func(ctx context.Context, err error) {
		log.Println("💥 客户端断开连接")
		select {
		case disconnected <- struct{}{}:
		default:
		}
	})

	client.Start()
	defer client.Stop()

	// 等待首次连接
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect initially")
	}

	// 基线：发送一条，确保链路通
	err = client.Send([]byte("hello"))
	assert.NoError(t, err)

	// 验证服务端确实收到
	select {
	case got := <-serverReceived:
		assert.Equal(t, "hello", got)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive baseline message")
	}

	// 触发服务端主动断开
	err = client.Send([]byte("CLOSE"))
	assert.NoError(t, err)

	// 等待客户端感知断开
	select {
	case <-disconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not detect disconnection")
	}

	// 在断线期间入队异步与同步消息
	async1 := "A1"
	async2 := "A2"
	sync1 := "SYNC1"

	// 异步入队（在断线期间，这些应排队等待重连后写出）
	err = client.SendAsync([]byte(async1))
	assert.NoError(t, err)
	err = client.SendAsync([]byte(async2))
	assert.NoError(t, err)

	// 同步消息在断线期间发送：应在重连后写出并返回 nil
	syncDone := make(chan error, 1)
	go func() {
		syncDone <- client.Send([]byte(sync1))
	}()

	// 等待重连
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not reconnect in time")
	}

	// 验证服务端在重连后按顺序收到断线期间排队的消息
	waitMsg := func() string {
		select {
		case m := <-serverReceived:
			return m
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for server message after reconnect")
			return ""
		}
	}

	got1 := waitMsg()
	got2 := waitMsg()
	got3 := waitMsg()

	assert.Equal(t, async1, got1, "first resent message should be A1")
	assert.Equal(t, async2, got2, "second resent message should be A2")
	assert.Equal(t, sync1, got3, "third resent message should be SYNC1 (sync call enqueued after A1/A2)")

	// 同步调用应在消息真正写出后返回 nil
	select {
	case e := <-syncDone:
		assert.NoError(t, e, "sync Send should succeed after reconnect")
	case <-time.After(3 * time.Second):
		t.Fatal("sync Send did not complete after reconnect")
	}
}
