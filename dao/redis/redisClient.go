package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	"github.com/redis/go-redis/v9"
)

const defaultTLSVersion = tls.VersionTLS12

type Config struct {
	Host       string   `mapstructure:"host" yaml:"host"`   // 单机模式主机
	Port       string   `mapstructure:"port" yaml:"port"`   // 单机模式端口
	Nodes      []string `mapstructure:"nodes" yaml:"nodes"` // 集群模式节点列表（host:port）
	Username   string   `mapstructure:"userName" yaml:"username"`
	Auth       string   `mapstructure:"auth" yaml:"auth"`
	DB         int      `mapstructure:"db" yaml:"db"`
	PoolSize   int      `mapstructure:"poolSize" yaml:"pool_size"`
	Encryption uint8    `mapstructure:"encryption" yaml:"encryption"`
	Framework  string   `mapstructure:"framework" yaml:"framework"` // "standalone" / "cluster"，不填默认单机
	Prefix     string   `mapstructure:"prefix" yaml:"prefix"`
	Tls        bool     `mapstructure:"tls" yaml:"tls"`
	MinVersion uint16   `mapstructure:"minVersion" yaml:"minVersion"`
}

type Client struct {
	redis.UniversalClient
	Config
}

func (t Client) Key(key string) string {
	return fmt.Sprintf("%s%s", t.Config.Prefix, key)
}

func (t Client) Close() error {
	return t.UniversalClient.Close()
}

func NewRedis(config *Config) (Client, error) {
	auth := config.Auth
	if config.Encryption == 1 {
		auth = utils.MD5Sum(auth)
	}

	var addrs []string
	// 根据模式选择地址
	if strings.EqualFold(config.Framework, "cluster") {
		// 集群模式
		if len(config.Nodes) == 0 {
			// 如果没配置 Nodes，就用 Host+Port
			addrs = []string{normalizeAddr(config.Host, config.Port)}
		} else {
			addrs = normalizeAddrs(config.Nodes)
		}
	} else {
		// 单机模式
		addrs = []string{normalizeAddr(config.Host, config.Port)}
	}

	opts := &redis.UniversalOptions{
		Addrs:        addrs,
		DB:           config.DB, // 集群模式会忽略
		PoolSize:     config.PoolSize,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	if auth != "" {
		opts.Username = config.Username
		opts.Password = auth
	}

	if config.Tls {
		opts.TLSConfig = &tls.Config{}
		setTLSConfig(opts.TLSConfig, config.MinVersion)
	}

	// 集群模式额外优化
	if strings.EqualFold(config.Framework, "cluster") {
		opts.RouteRandomly = true
	}

	uc := redis.NewUniversalClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := uc.Ping(ctx).Err(); err != nil {
		return Client{}, err
	}

	return Client{
		UniversalClient: uc,
		Config:          *config,
	}, nil
}

func setTLSConfig(tlsConfig *tls.Config, minVersion uint16) {
	if minVersion == 0 {
		minVersion = defaultTLSVersion
	}
	tlsConfig.MinVersion = minVersion
}

func normalizeAddr(host, port string) string {
	if strings.Contains(host, ":") {
		return host
	}
	return fmt.Sprintf("%s:%s", host, port)
}

func normalizeAddrs(nodes []string) []string {
	addrs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		n = strings.TrimSpace(n)
		if n != "" {
			addrs = append(addrs, n)
		}
	}
	return addrs
}
