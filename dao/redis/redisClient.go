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

const (
	defaultTLSVersion   = tls.VersionTLS12
	defaultReadTimeout  = 3 * time.Second
	defaultWriteTimeout = 3 * time.Second
	defaultPoolSize     = 20
)

type Config struct {
	Host         string   `mapstructure:"host" yaml:"host"`
	Port         string   `mapstructure:"port" yaml:"port"`
	Nodes        []string `mapstructure:"nodes" yaml:"nodes"`
	Username     string   `mapstructure:"userName" yaml:"userName"`
	Auth         string   `mapstructure:"auth" yaml:"auth"`
	DB           int      `mapstructure:"db" yaml:"db"`
	PoolSize     int      `mapstructure:"poolSize" yaml:"poolSize"`         // 连接池大小，0 时使用默认值 20
	Encryption   uint8    `mapstructure:"encryption" yaml:"encryption"`     // 1=MD5 密码
	Framework    string   `mapstructure:"framework" yaml:"framework"`       // "standalone"(默认) / "cluster"
	Prefix       string   `mapstructure:"prefix" yaml:"prefix"`             // key 前缀
	Tls          bool     `mapstructure:"tls" yaml:"tls"`                   // 是否启用 TLS
	MinVersion   uint16   `mapstructure:"minVersion" yaml:"minVersion"`     // TLS 最低版本，默认 TLS1.2
	ReadTimeout  int64    `mapstructure:"readTimeout" yaml:"readTimeout"`   // 读超时(ms)，默认 3000
	WriteTimeout int64    `mapstructure:"writeTimeout" yaml:"writeTimeout"` // 写超时(ms)，默认 3000
	EnableLog    bool     `mapstructure:"enableLog" yaml:"enableLog"`       // 是否记录命令日志，默认 false
}

type Client struct {
	redis.UniversalClient
	config Config
}

func (t Client) Key(key string) string {
	return fmt.Sprintf("%s%s", t.config.Prefix, key)
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

	if config.EnableLog {
		client.AddHook(LogHook{})
	}

	return client, nil
}

func createClusterClient(config *Config, auth string) (Client, error) {
	var addrArr []string
	if len(config.Nodes) == 0 {
		addrArr = []string{normalizeAddr(config.Host, config.Port)}
	} else {
		addrArr = normalizeAddrs(config.Nodes)
	}
	clusterOptions := &redis.ClusterOptions{
		Addrs:        addrArr,
		PoolSize:     poolSize(config.PoolSize),
		DialTimeout:  10 * time.Second,
		ReadTimeout:  duration(config.ReadTimeout, defaultReadTimeout),
		WriteTimeout: duration(config.WriteTimeout, defaultWriteTimeout),
	}

	if auth != "" {
		clusterOptions.Username = config.Username
		clusterOptions.Password = auth
	}

	if config.Tls {
		clusterOptions.TLSConfig = newTLSConfig(config.MinVersion)
	}

	clusterClient := redis.NewClusterClient(clusterOptions)
	if _, err := clusterClient.Ping(context.Background()).Result(); err != nil {
		return Client{}, err
	}

	return Client{
		UniversalClient: clusterClient,
		config:          *config,
	}, nil
}

func createStandaloneClient(config *Config, auth string) (Client, error) {
	addr := normalizeAddr(config.Host, config.Port)
	options := &redis.Options{
		Addr:         addr,
		PoolSize:     poolSize(config.PoolSize),
		DB:           config.DB,
		ReadTimeout:  duration(config.ReadTimeout, defaultReadTimeout),
		WriteTimeout: duration(config.WriteTimeout, defaultWriteTimeout),
	}

	if auth != "" {
		options.Username = config.Username
		options.Password = auth
	}

	if config.Tls {
		options.TLSConfig = newTLSConfig(config.MinVersion)
	}

	client := redis.NewClient(options)
	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return Client{}, err
	}

	return Client{
		UniversalClient: client,
		config:          *config,
	}, nil
}

func newTLSConfig(minVersion uint16) *tls.Config {
	if minVersion == 0 {
		minVersion = defaultTLSVersion
	}
	return &tls.Config{
		MinVersion: minVersion,
	}
}

func poolSize(size int) int {
	if size <= 0 {
		return defaultPoolSize
	}
	return size
}

func duration(ms int64, defaultVal time.Duration) time.Duration {
	if ms <= 0 {
		return defaultVal
	}
	return time.Duration(ms) * time.Millisecond
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
