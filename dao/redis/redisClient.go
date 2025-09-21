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

func NewRedis(config *Config) (client Client, err error) {
	auth := config.Auth
	if config.Encryption == 1 {
		auth = utils.MD5Sum(auth)
	}

	if config.Framework == "cluster" {
		client, err = createClusterClient(config, auth)
	} else {
		client, err = createStandaloneClient(config, auth)
	}

	if err != nil {
		return Client{}, err
	}

	return client, nil
}

func createClusterClient(config *Config, auth string) (Client, error) {
	var addrArr []string
	if len(config.Nodes) == 0 {
		// 如果没配置 Nodes，就用 Host+Port
		addrArr = []string{normalizeAddr(config.Host, config.Port)}
	} else {
		addrArr = normalizeAddrs(config.Nodes)
	}
	clusterOptions := &redis.ClusterOptions{
		Addrs:       addrArr,
		PoolSize:    config.PoolSize,
		DialTimeout: 10 * time.Second,
	}

	if auth != "" {
		clusterOptions.Username = config.Username
		clusterOptions.Password = auth
	}

	if config.Tls {
		setTLSConfig(clusterOptions.TLSConfig, config.MinVersion)
	}

	clusterClient := redis.NewClusterClient(clusterOptions)
	if _, err := clusterClient.Ping(context.Background()).Result(); err != nil {
		return Client{}, err
	}

	return Client{
		UniversalClient: clusterClient,
		Config:          *config,
	}, nil
}

func createStandaloneClient(config *Config, auth string) (Client, error) {
	addr := normalizeAddr(config.Host, config.Port)
	options := &redis.Options{
		Addr:     addr,
		PoolSize: config.PoolSize,
		DB:       config.DB,
	}

	if auth != "" {
		options.Username = config.Username
		options.Password = auth
	}

	if config.Tls {
		setTLSConfig(options.TLSConfig, config.MinVersion)
	}

	client := redis.NewClient(options)
	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return Client{}, err
	}

	return Client{
		UniversalClient: client,
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
