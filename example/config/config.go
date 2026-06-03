//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package config

import (
	"go-lib/config"
	"go-lib/dao/gormDB"
	"go-lib/dao/rabbitMQ"
	"go-lib/dao/redis"
	log2 "go-lib/log"
)

// Config 全局配置实例，在 init 中加载
var Config *Conf

func init() {
	Config = new(Conf)
	// 使用 go-lib 的 config.Parse 加载配置文件
	// 参数：配置文件路径、文件类型（空字符串自动推断）、目标结构体指针
	if err := config.Parse("./config.yaml", "", Config); err != nil {
		panic(err)
	}
}

// Conf 共享配置结构，聚合 go-lib 提供的各组件配置类型。
// 各服务按需使用其中的字段，yaml 中只填该服务需要的部分。
type Conf struct {
	ServerPort string `mapstructure:"serverPort" yaml:"serverPort"` // 监听端口
	ServerName string `mapstructure:"serverName" yaml:"serverName"` // 服务名称
	ENV        string `mapstructure:"env" yaml:"env"`               // 环境：prod / test

	Logging  *log2.Config       `mapstructure:"log" yaml:"log"`           // 日志配置
	DB       *gormDB.GormConfig `mapstructure:"db" yaml:"db"`             // 数据库配置
	Redis    *redis.Config      `mapstructure:"redis" yaml:"redis"`       // Redis 配置
	RabbitMQ *rabbitMQ.Config   `mapstructure:"rabbitMQ" yaml:"rabbitMQ"` // MQ 配置

	// 业务项目自定义的外部服务配置
}
