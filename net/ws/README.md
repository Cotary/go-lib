# WebSocket 库

基于 `gorilla/websocket` 的 WebSocket 客户端和服务端封装，支持自动重连、心跳保活、广播等功能。

## 特性

- **客户端**: 自动重连、指数退避抖动、心跳 Ping/Pong、并发安全写入
- **服务端**: 连接管理、广播、自定义数据存取、Gin/标准库兼容

## 客户端

### 基本使用

```go
client := ws.NewClient("ws://localhost:8080/ws")

client.OnConnect(func(ctx context.Context) {
    log.Println("connected")
})

client.OnMessage(func(ctx context.Context, mt int, data []byte) {
    log.Printf("received: %s", data)
})

client.OnDisconnect(func(ctx context.Context, err error) {
    log.Printf("disconnected: %v", err)
})

client.Start()
defer client.Stop()

// 发送消息
err := client.Send([]byte("hello"))
```

### Option 配置

```go
client := ws.NewClient("ws://localhost:8080/ws",
    ws.WithHeaders(http.Header{"Authorization": {"Bearer xxx"}}),
    ws.WithRetry(1*time.Second, 30*time.Second),  // 退避基数 1s，最大 30s
    ws.WithPingInterval(20*time.Second),
    ws.WithPongWait(60*time.Second),
    ws.WithWriteTimeout(5*time.Second),
    ws.WithReadLimit(1024*1024),                   // 最大消息 1MB
    ws.WithDialer(customDialer),
)
```

### 重连机制

客户端内置自动重连，断线后按指数退避策略重连：

- 退避公式: `base * 2^attempt * jitter`
- 抖动范围: `0.5x ~ 1.5x`
- 默认基数 `1s`，上限 `30s`，可通过 `WithRetry` 调整

### 状态查询

```go
client.IsRunning()   // 是否已启动
client.IsConnected() // 是否已连接
```

## 服务端

### 基本使用

```go
srv := ws.New(ws.Config{
    ReadTimeout:  60 * time.Second,
    PingInterval: 20 * time.Second,
    WriteTimeout: 5 * time.Second,
    OnConnect: func(ctx context.Context, conn *ws.Conn) {
        log.Println("new connection")
    },
    OnMessage: func(ctx context.Context, conn *ws.Conn, mt int, data []byte) {
        conn.SendText("echo: " + string(data))
    },
    OnClose: func(ctx context.Context, conn *ws.Conn, err error) {
        log.Printf("connection closed: %v", err)
    },
})
```

### 集成 Gin

```go
r := gin.New()
r.GET("/ws", func(c *gin.Context) {
    srv.ServeHTTP(c.Writer, c.Request)
})
```

### 集成标准库

```go
http.Handle("/ws", srv)
```

### 连接数据存取

通过 `OnUpgrade` 回调从 HTTP 请求中提取信息存入连接：

```go
srv := ws.New(ws.Config{
    OnUpgrade: func(conn *ws.Conn, r *http.Request) {
        conn.Set("user_id", r.Header.Get("X-User-ID"))
    },
    OnMessage: func(ctx context.Context, conn *ws.Conn, mt int, data []byte) {
        userID := conn.GetString("user_id")
        // ...
    },
})
```

### 广播与连接管理

```go
srv.Broadcast(websocket.TextMessage, []byte("hello all"))
srv.BroadcastText("hello all")

srv.Count()     // 当前活跃连接数
srv.CloseAll()  // 关闭所有连接

srv.ForEach(func(conn *ws.Conn) {
    conn.SendText("ping")
})
```

## Config 参数说明

- `ReadTimeout` - 读超时，默认 60s。超过此时间未收到消息（含 Pong）则断开
- `PingInterval` - 服务端主动 Ping 间隔，默认 20s
- `WriteTimeout` - 写超时，默认 5s
- `OnUpgrade` - 升级成功回调，在 OnConnect 之前调用，可从 http.Request 提取信息
- `OnMessage` - 消息回调
- `OnConnect` - 连接建立回调
- `OnClose` - 连接关闭回调

## 注意事项

- 客户端的 `Send` 方法是并发安全的，内部通过 `writeMu` 保护
- 服务端连接的 `Send` / `SendText` / `SendBinary` 也是并发安全的
- `OnMessage` 等回调在独立 goroutine 中通过 `SafeFunc` 执行，内部 panic 不会导致连接崩溃
- 客户端断线重连期间调用 `Send` 会返回 `"not connected"` 错误
