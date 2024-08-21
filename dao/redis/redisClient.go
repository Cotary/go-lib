package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/redis/go-redis/v9"
	"strings"
	"time"
)

type Config struct {
	Host       string `yaml:"host"`
	Port       string `yaml:"port"`
	Username   string `yaml:"userName"`
	Auth       string `yaml:"auth"`
	DB         int    `yaml:"db"`
	PoolSize   int    `yaml:"poolSize"`
	Encryption uint8  `yaml:"encryption"`
	Framework  string `yaml:"framework"`
	Prefix     string `yaml:"prefix"`
	Tls        bool   `yaml:"tls"`
	MinVersion uint16 `yaml:"minVersion"`
}

type Client struct {
	redis.UniversalClient
	Config
}

func (t Client) Key(key string) string {
	return fmt.Sprintf("%s%s", t.Config.Prefix, key)
}

func newClient(client redis.UniversalClient, config Config) Client {
	return Client{
		UniversalClient: client,
		Config:          config,
	}
}

func NewRedis(config *Config) Client {
	auth := config.Auth
	if config.Encryption == 1 {
		auth = utils.MD5(auth)
	}
	addr := config.Host + config.Port
	if !strings.Contains(addr, ":") {
		addr = config.Host + ":" + config.Port
	}

	if config.Framework == "cluster" {
		clusterOptions := &redis.ClusterOptions{Addrs: []string{addr}, PoolSize: config.PoolSize}
		if auth != "" {
			clusterOptions.DialTimeout = time.Second * 10
			clusterOptions.Username = config.Username
			clusterOptions.Password = auth
		}
		if config.Tls {
			if config.MinVersion == 0 {
				config.MinVersion = tls.VersionTLS12
			}
			clusterOptions.TLSConfig = &tls.Config{
				MinVersion: config.MinVersion,
			}
		}
		clusterClient := redis.NewClusterClient(clusterOptions)
		if _, err := clusterClient.Ping(context.Background()).Result(); err != nil {
			panic(err)
		}
		return newClient(clusterClient, *config)
	} else {
		options := &redis.Options{Addr: addr, PoolSize: config.PoolSize, DB: config.DB}
		if auth != "" {
			options.Username = config.Username
			options.Password = auth
		}
		if config.Tls {
			if config.MinVersion == 0 {
				config.MinVersion = tls.VersionTLS12
			}
			options.TLSConfig = &tls.Config{
				MinVersion: config.MinVersion,
			}
		}
		client := redis.NewClient(options)
		if _, err := client.Ping(context.TODO()).Result(); err != nil {
			panic(err)
		}
		return newClient(client, *config)
	}
}
