package mongo

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"time"
)

type Config struct {
	AppName     string `mapstructure:"appName" yaml:"appName"`
	Database    string `mapstructure:"database" yaml:"database"`
	Dns         string `mapstructure:"dns" yaml:"dns"`
	IdleTimeout int64  `mapstructure:"idleTimeout" yaml:"idleTimeout"`
	MaxOpens    uint64 `mapstructure:"maxOpens" yaml:"maxOpens"`
	MinOpens    uint64 `mapstructure:"minOpens" yaml:"minOpens"`
}

type Drive struct {
	*mongo.Database
}

type Client struct {
	c *Config
	*mongo.Client
}

func NewDrive(c *Config) *Drive {
	ctx := context.Background()
	db := new(Drive)
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(c.Dns).
		SetAppName(c.AppName).
		SetMaxConnIdleTime(time.Millisecond*time.Duration(c.IdleTimeout)).
		SetMaxPoolSize(c.MaxOpens).
		SetMinPoolSize(c.MinOpens))
	if err != nil {
		panic(err)
	}
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		panic(err)
	}
	db.Database = client.Database(c.Database)
	return db
}

func NewClient(c *Config) *Client {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(c.Dns).
		SetAppName(c.AppName).
		SetMaxConnIdleTime(time.Millisecond*time.Duration(c.IdleTimeout)).
		SetMaxPoolSize(c.MaxOpens).
		SetMinPoolSize(c.MinOpens))
	if err != nil {
		panic(err)
	}
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		panic(err)
	}
	mc := &Client{
		c:      c,
		Client: client,
	}
	return mc
}

func (this *Client) GetConn() (mongo.Session, *mongo.Database) {
	session, err := this.StartSession()
	if err != nil {
		panic(err)
	}

	return session, session.Client().Database(this.c.Database)
}

func (this *Client) CloseConn(session mongo.Session, c ...context.Context) {
	var ctx context.Context
	if len(c) > 0 {
		ctx = c[0]
	} else {
		ctx = context.Background()
	}
	session.EndSession(ctx)
}
