package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	"github.com/redis/go-redis/v9"
)

const defaultTLSVersion = tls.VersionTLS12

type Config struct {
	Host       []string `yaml:"host"`
	Username   string   `yaml:"userName"`
	Auth       string   `yaml:"auth"`
	DB         int      `yaml:"db"`
	PoolSize   int      `yaml:"poolSize"`
	Encryption uint8    `yaml:"encryption"`
	Framework  string   `yaml:"framework"`
	Prefix     string   `yaml:"prefix"`
	Tls        bool     `yaml:"tls"`
	MinVersion uint16   `yaml:"minVersion"`
}

type Client struct {
	redis.UniversalClient
	*redis.Client
	*redis.ClusterClient
	Config
}

func (t Client) Key(key string) string {
	return fmt.Sprintf("%s%s", t.Config.Prefix, key)
}

func (t Client) Close() error {
	return t.UniversalClient.Close()
}

func newClient(client redis.UniversalClient, config Config) Client {
	return Client{
		UniversalClient: client,
		Config:          config,
	}
}

func NewRedis(config *Config) (client Client, err error) {
	auth := config.Auth
	if config.Encryption == 1 {
		auth = utils.MD5(auth)
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
	clusterOptions := &redis.ClusterOptions{
		Addrs:       config.Host,
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
		ClusterClient:   clusterClient,
		Config:          *config,
	}, nil
}

func createStandaloneClient(config *Config, auth string) (Client, error) {
	options := &redis.Options{
		Addr:     config.Host[0],
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
		Client:          client,
		Config:          *config,
	}, nil
}

func setTLSConfig(tlsConfig *tls.Config, minVersion uint16) {
	if minVersion == 0 {
		minVersion = defaultTLSVersion
	}
	tlsConfig.MinVersion = minVersion
}
