### net/ws - 轻量 WebSocket 客户端与服务端封装

基于 `gorilla/websocket` 与 `gin` 的 WebSocket 组件，提供服务端升级处理与客户端的自动重连、心跳保活、同步/异步发送、优雅停止等能力。

#### 主要特性

- **服务端**：
  - **自动心跳**：定时发送 Ping，Pong 刷新读超时；`ReadTimeout` 超时自动断开
  - **事件回调**：`OnConnect`、`OnMessage`、`OnClose`
  - **简易发送**：`Conn.SendText`、`Conn.Send`
- **客户端**：
  - **自动重连**：指数退避 + 抖动（1s 起，最大 30s，默认值）
  - **心跳保活**：定时发送 Ping，收到 Pong 刷新读窗口
  - **统一出站队列**：支持同步 `Send` 与异步 `SendAsync`；断线期间自动排队，重连后按顺序写出
  - **优雅停止**：`Start/Stop` 幂等，`IsRunning/IsConnected` 查询

依赖：`github.com/gorilla/websocket`、`github.com/gin-gonic/gin`

#### 安装与导入

```go
import (
    ws "github.com/Cotary/go-lib/net/ws"
)
```

### 快速开始

#### 服务端（Gin）

```go
package main

import (
    "github.com/gin-gonic/gin"
    ws "github.com/Cotary/go-lib/net/ws"
)

func main() {
    s := ws.New(ws.Config{
        OnMessage: func(c *ws.Conn, mt int, data []byte) {
            // 简单 echo
            _ = c.SendText("echo: " + string(data))
        },
        OnConnect: func(c *ws.Conn) {},
        OnClose:   func(c *ws.Conn, err error) {},
    })

    r := gin.Default()
    r.GET("/ws", s.Handler())
    _ = r.Run(":8080")
}
```

#### 客户端

```go
client := ws.NewClient("ws://127.0.0.1:8080/ws")

client.OnConnect(func() { /* ... */ })
client.OnDisconnect(func(err error) { /* ... */ })
client.OnMessage(func(mt int, data []byte) {
    // 收到服务端消息
})

client.Start()
defer client.Stop()

// 同步发送：等待写入当前连接结果
if err := client.Send([]byte("hello websocket")); err != nil {
    // 可能因队列满/停止/当前连接写失败等返回错误
}

// 异步发送：快速入队，不等待写入完成
_ = client.SendAsync([]byte("fire-and-forget"))
```

### API 概览

#### 服务端

- `type Config struct`：
  - `ReadTimeout time.Duration`：Pong 超时（默认 60s）
  - `PingInterval time.Duration`：心跳间隔（默认 20s）
  - `OnMessage func(c *Conn, mt int, data []byte)`：收到消息回调
  - `OnConnect func(c *Conn)`：新连接建立回调（可选）
  - `OnClose func(c *Conn, err error)`：连接关闭回调（可选）
- `func New(cfg Config) *Server`：创建服务端
- `func (s *Server) Handler() gin.HandlerFunc`：Gin 路由处理器（负责升级）
- `type Conn`：代表单个连接
  - `func (c *Conn) SendText(text string) error`
  - `func (c *Conn) Send(mt int, data []byte) error`：可指定 `websocket.TextMessage`/`BinaryMessage` 等
  - `func (c *Conn) Close(err error)`：优雅关闭并触发 `OnClose`

说明：`Upgrader.CheckOrigin` 默认放行所有来源，如需限制跨域来源可自行在源码中调整。

#### 客户端

- `func NewClient(url string) *Client`
- 生命周期：
  - `Start()` / `Stop()`：幂等、线程安全
  - `IsRunning()` / `IsConnected()`：运行与连接状态查询
- 发送：
  - `Send(data []byte) error`：同步发送（确认写入是否成功），断线时会在队列中等待重连后写出
  - `SendAsync(data []byte) error`：异步发送（快速入队）
- 回调：
  - `OnMessage(func(mt int, data []byte))`
  - `OnConnect(func())`
  - `OnDisconnect(func(err error))`

默认参数：

- 读超时（Pong 窗口）`60s`，心跳间隔 `20s`，写超时 `5s`
- 自动重连：指数退避（基础 `1s`，最大 `30s`，带抖动）
- 出站队列容量：`1024`（当前不可外部配置）

注意：客户端当前提供的便捷方法为“文本帧”发送（`Send/SendAsync`），如需二进制可在本包内扩展或修改。

### 使用建议与注意事项

- **路径与协议**：客户端 URL 必须包含路径并使用 `ws://` 或 `wss://`；例如 `ws://host:port/ws`。
- **队列与背压**：当出站队列（默认 1024）已满时，`Send/SendAsync` 会返回错误，请在上层做重试或限流。
- **断线期间发送**：两者都会入队；重连成功后按入队顺序写出。同步 `Send` 会在对应消息真正写出后返回。
- **心跳超时**：若长时间无 Pong，读侧会报错并触发重连；服务器与客户端都内置 Ping/Pong 逻辑。
- **关闭语义**：`Stop()` 会关闭后台协程并关闭连接；`Start/Stop` 可多次调用且安全。

### 端到端示例（集成测试同款）

你可以参考包内测试快速验证：

```bash
go test ./net/ws -run WebSocket -v
```

测试覆盖：
- 客户端/服务端联通、echo 验证
- 服务端主动断开后，客户端自动重连
- 断线期间异步与同步消息的排队与顺序写出

### 常见问题（FAQ）

- **连接不上/404**：确认路由是否注册了正确的 WebSocket 路径，例如 `router.GET("/ws", s.Handler())`。
- **跨域/来源限制**：默认 `CheckOrigin` 放开所有来源，生产环境建议按需收紧。
- **二进制消息**：服务端 `Conn.Send` 可指定帧类型；客户端当前便捷方法只封装了文本帧，可按需扩展。
- **调试日志**：内部会打印关键错误日志（连接失败、读写错误、回调 panic 保护等）。

### 许可证

遵循项目根目录的许可证声明。

