//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package dao

import (
	"myproject/config"

	"go-lib/dao/rabbitMQ"
	"go-lib/dao/redis"
)

// 全局数据源实例
var (
	Redis redis.Client
	MQ    *rabbitMQ.Connect
)

// InitRedis 初始化 Redis 连接
func InitRedis() {
	var err error
	Redis, err = redis.NewRedis(config.Config.Redis)
	if err != nil {
		panic(err)
	}
}

// InitMQ 初始化 RabbitMQ 连接
func InitMQ() {
	var err error
	MQ, err = rabbitMQ.NewRabbitMQ(*config.Config.RabbitMQ)
	if err != nil {
		panic(err)
	}
}
