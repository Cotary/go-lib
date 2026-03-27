# dao/mongo

基于 [go.mongodb.org/mongo-driver/v2](https://pkg.go.dev/go.mongodb.org/mongo-driver/v2) 封装的 MongoDB 客户端，提供连接管理、日志监控和事务支持。

## 两种模式

| 类型 | 说明 | 适用场景 |
|------|------|---------|
| `DB` | 绑定单个 Database，直接暴露 `*mongo.Database` | 单库操作，最常用 |
| `Client` | 持有 `*mongo.Client`，可动态切换 Database | 多库切换、事务 |

## 快速开始

### 配置

```yaml
mongo:
  appName: "my-service"
  uri: "mongodb://user:pass@localhost:27017"
  database: "mydb"
  maxPoolSize: 100     # 连接池最大连接数，默认 100
  minPoolSize: 10      # 连接池最小连接数，默认 10
  maxConnIdleMs: 1800000 # 空闲连接最大存活时间(ms)，默认 30 分钟
  connectTimeout: 10000  # 连接超时(ms)，默认 10 秒
  enableLog: true        # 是否记录命令日志，默认 false
```

### Config 字段说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `AppName` | string | 否 | - | 应用名，用于 MongoDB 服务端日志标识 |
| `URI` | string | 是 | - | MongoDB 连接串，如 `mongodb://host:port` |
| `Database` | string | 是 | - | 默认数据库名 |
| `MaxPoolSize` | uint64 | 否 | 100 | 连接池最大连接数 |
| `MinPoolSize` | uint64 | 否 | 10 | 连接池最小连接数 |
| `MaxConnIdleMs` | int64 | 否 | 1800000 | 空闲连接最大存活时间（毫秒） |
| `ConnectTimeout` | int64 | 否 | 10000 | 连接超时（毫秒） |
| `EnableLog` | bool | 否 | false | 是否记录命令日志 |

### DB 模式（推荐单库使用）

```go
cfg := &mongo.Config{
    URI:      "mongodb://localhost:27017",
    Database: "mydb",
}

// 返回 error
db, err := mongo.NewDB(cfg)
if err != nil {
    log.Fatal(err)
}
defer db.Close(context.Background())

// 直接使用 *mongo.Database 的所有方法
coll := db.Collection("users")

// 插入
_, err = coll.InsertOne(ctx, bson.D{{"name", "Alice"}, {"age", 30}})

// 查询
var result bson.M
err = coll.FindOne(ctx, bson.D{{"name", "Alice"}}).Decode(&result)

// 更新
_, err = coll.UpdateOne(ctx, bson.D{{"name", "Alice"}}, bson.D{{"$set", bson.D{{"age", 31}}}})

// 删除
_, err = coll.DeleteOne(ctx, bson.D{{"name", "Alice"}})
```

init 阶段可使用 panic 版本：

```go
db := mongo.MustNewDB(cfg)
```

### Client 模式（多库 / 事务）

```go
cfg := &mongo.Config{
    URI:      "mongodb://localhost:27017",
    Database: "mydb",
}

client, err := mongo.NewClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer client.Close(context.Background())

// 使用默认 Database
db := client.Database()

// 切换到其他 Database
otherDB := client.Database("other_db")
```

### 事务

事务通过 `Client.Transaction` 方法使用，自动处理提交和回滚：

```go
result, err := client.Transaction(ctx, func(ctx context.Context) (any, error) {
    coll := client.Database().Collection("orders")
    
    _, err := coll.InsertOne(ctx, bson.D{{"item", "phone"}, {"qty", 1}})
    if err != nil {
        return nil, err // 自动回滚
    }

    _, err = coll.UpdateOne(ctx,
        bson.D{{"item", "phone"}},
        bson.D{{"$inc", bson.D{{"stock", -1}}}},
    )
    if err != nil {
        return nil, err // 自动回滚
    }

    return "order created", nil // 自动提交
})
```

> 注意：事务要求 MongoDB 副本集或分片集群，单节点不支持事务。

## 日志监控

内置 `CommandMonitor`，自动记录所有 MongoDB 命令的执行情况：

- **命令开始**：记录命令名、数据库、原始命令内容
- **命令成功**：记录耗时，超过 500ms 以 Warn 级别输出
- **命令失败**：以 Error 级别记录错误信息和耗时

日志通过项目的 `log.WithContext(ctx)` 输出，自动携带 RequestID 等链路信息。

## 运行测试

```bash
# 纯函数单元测试（无需 MongoDB）
go test -v -run TestUnit ./dao/mongo/

# 集成测试（需要 MongoDB 实例）
MONGO_TEST_URI=mongodb://localhost:27017 go test -v -run TestIntegration ./dao/mongo/

# 事务测试（需要副本集）
MONGO_TEST_URI=mongodb://localhost:27017/?replicaSet=rs0 go test -v -run TestIntegrationTransaction ./dao/mongo/
```
