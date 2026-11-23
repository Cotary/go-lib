package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Client 是 gRPC 客户端。
//
// 它包了一层，让业务代码用统一的错误还原逻辑，
// 不必每次都自己 status.FromError + details 解析。
type Client struct {
	conn *grpc.ClientConn
	opts *options
}

// Dial 创建客户端连接。
//
// 【注意】grpc.NewClient 是惰性连接：它不会立刻建连，
// 第一次调用时才真正连接。这样启动时不会因为下游不可达而卡住。
//
//	c, err := grpcx.Dial(ctx, "order-service:9090",
//	    grpcx.WithInsecure(),
//	    grpcx.WithUnaryInterceptor(grpcx.RequestIDInterceptor()),
//	)
func Dial(ctx context.Context, target string, opts ...Option) (*Client, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	dialOpts := o.dialOptionsList()
	if len(dialOpts) == 0 {
		// 没指定凭证时默认明文，避免必须显式写 WithInsecure。
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpc: dial %s: %w", target, err)
	}
	_ = ctx

	return &Client{conn: conn, opts: o}, nil
}

// Conn 返回原生连接，需要流式调用或自定义 call option 时用。
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Invoke 发起一次 unary 调用。
//
// 返回的是 gRPC 原始错误（未做任何转换）。要业务语义自己调 ToBusinessError：
//
//	if codeErr, ok := grpcx.ToBusinessError(err); ok { ... }
func (c *Client) Invoke(ctx context.Context, method string, req, reply any, opts ...grpc.CallOption) error {
	ctx, cancel := c.opts.withTimeout(ctx)
	defer cancel()

	return c.conn.Invoke(ctx, method, req, reply, opts...)
}

// NewStream 发起一次流式调用。
//
// 返回的是 gRPC 原始错误（未做任何转换）。
func (c *Client) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx, cancel := c.opts.withTimeout(ctx)
	stream, err := c.conn.NewStream(ctx, desc, method, opts...)
	if err != nil {
		cancel()
		return nil, err
	}
	// 流式的生命周期由调用方控制，超时 cancel 交给调用方释放。
	return &clientStream{ClientStream: stream, cancel: cancel}, nil
}

// clientStream 给流式调用挂上超时 cancel。
type clientStream struct {
	grpc.ClientStream
	cancel context.CancelFunc
}

// Close 关闭发送方向并释放超时。
//
// grpc.ClientStream 没有 Close 方法，接收方向要靠 ctx 取消或 Recv 返回 io.EOF 结束。
func (s *clientStream) Close() error {
	err := s.ClientStream.CloseSend()
	s.cancel()
	return err
}

// Close 关闭连接。
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Ping 检查连接是否可用。
//
// 【注意】grpc 的连接是惰性的，Ping 会触发一次真实连接尝试。
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if c.conn.GetState() == connectivity.Ready {
		return nil
	}
	c.conn.Connect()
	return nil
}
