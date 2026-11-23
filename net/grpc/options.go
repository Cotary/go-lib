package grpc

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Option 是客户端的配置项。
type Option func(*options)

// options 客户端配置的内部载体。
type options struct {
	dialOptions []grpc.DialOption
	unary       []grpc.UnaryClientInterceptor
	stream      []grpc.StreamClientInterceptor
	timeout     time.Duration
}

// WithInsecure 使用明文连接（仅限内网 / 本地调试）。
func WithInsecure() Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
}

// WithTLS 使用 TLS 连接。
func WithTLS(cfg *tls.Config) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	}
}

// WithCredentials 使用自定义传输凭证。
func WithCredentials(creds credentials.TransportCredentials) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithTransportCredentials(creds))
	}
}

// WithPerRPCCredentials 附加每次调用的凭证（如 token）。
func WithPerRPCCredentials(creds credentials.PerRPCCredentials) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithPerRPCCredentials(creds))
	}
}

// WithUnaryInterceptor 追加一个 unary 客户端拦截器。
func WithUnaryInterceptor(i grpc.UnaryClientInterceptor) Option {
	return func(o *options) {
		o.unary = append(o.unary, i)
	}
}

// WithStreamInterceptor 追加一个 stream 客户端拦截器。
func WithStreamInterceptor(i grpc.StreamClientInterceptor) Option {
	return func(o *options) {
		o.stream = append(o.stream, i)
	}
}

// WithTimeout 给每次调用设置默认超时。
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithKeepalive 设置 keepalive 参数。
func WithKeepalive(params keepalive.ClientParameters) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithKeepaliveParams(params))
	}
}

// WithUserAgent 设置 User-Agent。
func WithUserAgent(ua string) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithUserAgent(ua))
	}
}

// WithDialOption 透传原生 grpc.DialOption。
//
// 用于本库没有单独封装的能力，典型场景：
//
//	// OpenTelemetry（官方推荐的 stats.Handler 方式）
//	grpcx.WithDialOption(grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
//
//	// 自定义负载均衡 / 服务发现 / 连接池
//	grpcx.WithDialOption(grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
//
// 可以直接传 grpc.WithTransportCredentials，但注意它和 WithInsecure / WithTLS
// 是互斥的：同时传会以最后生效的为准，不会报错。
func WithDialOption(opts ...grpc.DialOption) Option {
	return func(o *options) { o.dialOptions = append(o.dialOptions, opts...) }
}

// dialOptionsList 组装最终的 grpc.DialOption 列表。
func (o *options) dialOptionsList() []grpc.DialOption {
	out := make([]grpc.DialOption, 0, len(o.dialOptions)+2)
	out = append(out, o.dialOptions...)
	if len(o.unary) > 0 {
		out = append(out, grpc.WithChainUnaryInterceptor(o.unary...))
	}
	if len(o.stream) > 0 {
		out = append(out, grpc.WithChainStreamInterceptor(o.stream...))
	}
	return out
}

// defaultTimeout 是未指定超时时使用的默认值。
const defaultTimeout = 30 * time.Second

// withTimeout 在 ctx 上补一个默认超时。
func (o *options) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if o.timeout <= 0 {
		return context.WithTimeout(ctx, defaultTimeout)
	}
	return context.WithTimeout(ctx, o.timeout)
}
