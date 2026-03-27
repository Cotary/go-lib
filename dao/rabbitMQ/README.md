# rabbitMQ

封装 `amqp091-go`，提供连接池、自动重连、Publisher Confirm / 事务发送、延迟重试消费等能力。

## 架构

```
┌─────────────────────────────────────────────────────┐
│  NewRabbitMQ(Config)                                │
│  ┌───────────────────────────────────────────────┐  │
│  │  Connect                                      │  │
│  │  - *amqp091.Connection (自动重连)              │  │
│  │  - chan *amqp091.Channel (池化，懒创建)         │  │
│  │  - watchDisconnect (后台监控，指数退避重连)      │  │
│  └───────────────────────────────────────────────┘  │
│         │                                           │
│  NewQueue(conn, QueueConfig)                        │
│  ┌───────────────────────────────────────────────┐  │
│  │  Queue                                        │  │
│  │  - SendMessages      (Publisher Confirm)      │  │
│  │  - SendMessagesTx    (AMQP 事务)              │  │
│  │  - SendMessagesEvery (持续重试直到成功)         │  │
│  │  - ConsumeMessages   (手动 ACK)               │  │
│  │  - ConsumeMessagesEvery (持续消费 + 自动重连)  │  │
│  │  - RetryLater        (延迟重投递)              │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

## 快速开始

```go
// 1. 建立连接
conn, err := rabbitMQ.NewRabbitMQ(rabbitMQ.Config{
    DSN:        []string{"amqp://guest:guest@localhost:5672/"},
    MaxChannel: 50,
})
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// 2. 声明队列
q, err := rabbitMQ.NewQueue(conn, rabbitMQ.QueueConfig{
    ExchangeName: "my_exchange",
    RouteKey:     "my_route",
    QueueName:    "my_queue",
})
if err != nil {
    log.Fatal(err)
}

// 3. 发送消息
ctx := context.Background()
failed, err := q.SendMessages(ctx, []amqp091.Publishing{
    {Body: []byte(`{"event":"order_created","id":123}`)},
})

// 4. 消费消息
err = q.ConsumeMessagesEvery(ctx, func(ctx context.Context, d *rabbitMQ.Delivery) error {
    log.Printf("received: %s", d.Body)
    return nil // 返回 nil 自动 Ack；返回 error 触发 RetryLater/Nack
})
```

## 配置说明

### Config（连接配置）

| 字段         | 类型       | 默认值 | 说明                                              |
|-------------|-----------|--------|--------------------------------------------------|
| DSN         | []string  | -      | AMQP 连接地址列表，按顺序尝试                        |
| CA          | string    | ""     | CA 证书文件路径（TLS）                              |
| CertFile    | string    | ""     | 客户端证书文件路径（TLS）                            |
| KeyFile     | string    | ""     | 客户端私钥文件路径（TLS）                            |
| Heartbeat   | int64     | 5      | 心跳间隔（秒）                                      |
| MaxChannel  | int       | 1000   | Channel 池容量上限（懒创建，按需分配）                |

### QueueConfig（队列配置）

| 字段          | 类型           | 默认值    | 说明                                            |
|--------------|---------------|----------|------------------------------------------------|
| ExchangeName | string        | -        | 交换机名称                                       |
| ExchangeType | string        | "direct" | 交换机类型（direct/fanout/topic/headers）          |
| RouteKey     | string        | -        | 路由键                                           |
| QueueName    | string        | -        | 队列名称                                         |
| QueueType    | string        | "quorum" | 队列类型（classic/quorum）                        |
| Prefetch     | int           | 1        | 消费者预取数量，高吞吐场景可调大                      |
| MaxDelay     | time.Duration | 0        | 延迟队列最大延迟时间，>0 启用延迟交换机                |

## 使用模式

### Publisher Confirm 发送

```go
// 批量发送，返回发送失败的消息列表
failed, err := q.SendMessages(ctx, messages)
if err != nil {
    // 全局错误（连接断开等）
}
if len(failed) > 0 {
    // 部分消息 nack
}
```

### 持续重试发送

```go
// 自动重试直到全部成功或 ctx 取消，失败时自动报警
err := q.SendMessagesEvery(ctx, messages)

// 使用事务模式（原子性，性能较低）
err := q.SendMessagesEvery(ctx, messages, true)
```

### 事务发送

```go
// 原子性：全部成功或全部回滚
err := q.SendMessagesTx(ctx, messages)
```

### 消费消息

```go
// 单次消费（channel 断开时返回）
err := q.ConsumeMessages(ctx, handler)

// 持续消费（自动重连 + 指数退避）
err := q.ConsumeMessagesEvery(ctx, handler)
```

handler 返回值决定消息处理方式：
- `return nil` → 自动 Ack
- `return error` → 报警 + RetryLater（有 MaxDelay）或 Nack（无 MaxDelay）
- panic → 自动 recover + 报警 + RetryLater/Nack

### 延迟队列与重试

需要 RabbitMQ 安装 [rabbitmq_delayed_message_exchange](https://github.com/rabbitmq/rabbitmq-delayed-message-exchange) 插件。

```go
q, _ := rabbitMQ.NewQueue(conn, rabbitMQ.QueueConfig{
    ExchangeName: "delay_exchange",
    RouteKey:     "delay_route",
    QueueName:    "delay_queue",
    MaxDelay:     30 * time.Second, // 启用延迟交换机
})
```

消费失败时自动通过 `x-delay` header 延迟重投递，`x-retry-count` 递增追踪重试次数。
可通过 `d.GetRetryNum()` 在 handler 中获取当前重试次数。

### 手动控制 Delivery

```go
handler := func(ctx context.Context, d *rabbitMQ.Delivery) error {
    retryNum := d.GetRetryNum()
    if retryNum > 10 {
        // 超过阈值，手动 Ack 丢弃或转入 DLQ
        d.Ack()
        return nil
    }
    // 业务处理
    if err := process(d.Body); err != nil {
        d.RetryLater(5 * time.Second) // 手动指定延迟
        return nil                     // 已手动处理，返回 nil 避免框架重复操作
    }
    return nil
}
```

## 连接管理机制

- **多 DSN 容灾**：按顺序尝试连接，首个成功即使用
- **自动重连**：后台 goroutine 监听 `NotifyClose`，断线后指数退避重连（1s → 2s → 4s → ... → 60s）
- **Channel 池**：懒创建，用时从池取或新建，用完归还；Confirm/Tx 模式的 channel 不归还（避免状态污染）
- **优雅关闭**：`Close()` 幂等，先通知后台退出，再清理池和连接

## 注意事项

- 队列默认使用 **quorum** 类型，在 RabbitMQ 集群中提供更好的数据安全性
- 延迟队列依赖 `rabbitmq_delayed_message_exchange` 插件，未安装时 `NewQueue` 会报错
- TLS 配置仅在提供 CA 或证书时启用
- 所有异步错误（断线、发送失败、消费 panic 等）通过 `e.SendMessage` 上报

## 测试

测试依赖本地 RabbitMQ 实例。如不可用，测试会自动 skip。

```bash
# 启动 RabbitMQ（Docker）
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:management

# 延迟队列测试还需要安装插件
docker exec rabbitmq rabbitmq-plugins enable rabbitmq_delayed_message_exchange

# 运行测试
go test ./dao/rabbitMQ/... -v
```
