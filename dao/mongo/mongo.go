package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

const (
	defaultMaxPoolSize     = 100
	defaultMinPoolSize     = 10
	defaultMaxConnIdleTime = 30 * time.Minute
	defaultConnectTimeout  = 10 * time.Second
)

type Config struct {
	AppName        string `mapstructure:"appName" yaml:"appName"`
	Database       string `mapstructure:"database" yaml:"database"`
	URI            string `mapstructure:"uri" yaml:"uri"`
	MaxConnIdleMs  int64  `mapstructure:"maxConnIdleMs" yaml:"maxConnIdleMs"`
	MaxPoolSize    uint64 `mapstructure:"maxPoolSize" yaml:"maxPoolSize"`
	MinPoolSize    uint64 `mapstructure:"minPoolSize" yaml:"minPoolSize"`
	ConnectTimeout int64  `mapstructure:"connectTimeout" yaml:"connectTimeout"`
	EnableLog      bool   `mapstructure:"enableLog" yaml:"enableLog"`
}

type DB struct {
	*mongo.Database
	client *mongo.Client
}

type Client struct {
	config *Config
	*mongo.Client
}

func buildClientOpts(c *Config) *options.ClientOptions {
	opts := options.Client().ApplyURI(c.URI)

	if c.AppName != "" {
		opts.SetAppName(c.AppName)
	}

	idleTime := defaultMaxConnIdleTime
	if c.MaxConnIdleMs > 0 {
		idleTime = time.Duration(c.MaxConnIdleMs) * time.Millisecond
	}
	opts.SetMaxConnIdleTime(idleTime)

	var maxPool uint64 = defaultMaxPoolSize
	if c.MaxPoolSize > 0 {
		maxPool = c.MaxPoolSize
	}
	opts.SetMaxPoolSize(maxPool)

	var minPool uint64 = defaultMinPoolSize
	if c.MinPoolSize > 0 {
		minPool = c.MinPoolSize
	}
	opts.SetMinPoolSize(minPool)

	connectTimeout := defaultConnectTimeout
	if c.ConnectTimeout > 0 {
		connectTimeout = time.Duration(c.ConnectTimeout) * time.Millisecond
	}
	opts.SetConnectTimeout(connectTimeout)

	if c.EnableLog {
		opts.SetMonitor(newCommandMonitor())
	}

	return opts
}

func connect(c *Config) (*mongo.Client, error) {
	opts := buildClientOpts(c)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	if err = client.Ping(context.Background(), readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	return client, nil
}

func NewDB(c *Config) (*DB, error) {
	client, err := connect(c)
	if err != nil {
		return nil, err
	}
	return &DB{
		Database: client.Database(c.Database),
		client:   client,
	}, nil
}

func MustNewDB(c *Config) *DB {
	db, err := NewDB(c)
	if err != nil {
		panic(err)
	}
	return db
}

func (d *DB) Close(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}

func NewClient(c *Config) (*Client, error) {
	client, err := connect(c)
	if err != nil {
		return nil, err
	}
	return &Client{
		config: c,
		Client: client,
	}, nil
}

func MustNewClient(c *Config) *Client {
	mc, err := NewClient(c)
	if err != nil {
		panic(err)
	}
	return mc
}

func (c *Client) Database(name ...string) *mongo.Database {
	dbName := c.config.Database
	if len(name) > 0 && name[0] != "" {
		dbName = name[0]
	}
	return c.Client.Database(dbName)
}

func (c *Client) Close(ctx context.Context) error {
	return c.Client.Disconnect(ctx)
}

// Transaction 在一个会话事务中执行 fn。
// 事务自动提交；fn 返回 error 时自动回滚。
func (c *Client) Transaction(ctx context.Context, fn func(ctx context.Context) (any, error)) (any, error) {
	session, err := c.StartSession()
	if err != nil {
		return nil, fmt.Errorf("mongo start session: %w", err)
	}
	defer session.EndSession(ctx)

	result, err := session.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("mongo transaction: %w", err)
	}
	return result, nil
}
