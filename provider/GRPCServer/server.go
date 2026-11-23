package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/Cotary/go-lib/common/appctx"
)

// ============================================================================
// 配置
// ============================================================================

// Config 是 gRPC 服务端配置。
type Config struct {
	// Addr 监听地址，如 ":9090"。
	Addr string

	// MaxRecvMsgSize 单条消息最大接收字节数，0 表示用 grpc 默认值（4MB）。
	MaxRecvMsgSize int

	// MaxSendMsgSize 单条消息最大发送字节数，0 表示用 grpc 默认值。
	MaxSendMsgSize int

	// Keepalive 服务端 keepalive 参数，零值用 grpc 默认值。
	Keepalive keepalive.ServerParameters

	// ShutdownTimeout 优雅关闭的最长等待时间。
	// 超时后强制 Stop。默认 10 秒。
	ShutdownTimeout time.Duration
}

// withDefaults 补齐零值配置。
func (c Config) withDefaults() Config {
	out := c
	if out.MaxRecvMsgSize <= 0 {
		out.MaxRecvMsgSize = 4 * 1024 * 1024
	}
	if out.MaxSendMsgSize <= 0 {
		out.MaxSendMsgSize = 4 * 1024 * 1024
	}
	if out.ShutdownTimeout <= 0 {
		out.ShutdownTimeout = 10 * time.Second
	}
	return out
}

// ============================================================================
// Server
// ============================================================================

// Server 是 gRPC 服务端。
//
// 它负责生命周期（启动、优雅关闭）、健康检查、反射，
// 以及注册内置拦截器。业务方通过 GRPC() 拿到原生 *grpc.Server，
// 用 protoc 生成的桩代码注册自己的 service。
type Server struct {
	cfg       Config
	srv       *grpc.Server
	srvOpts   []grpc.ServerOption
	unary     []grpc.UnaryServerInterceptor
	stream    []grpc.StreamServerInterceptor
	healthSrv *health.Server
	listener  net.Listener
}

// Option 是 Server 的配置项。
type Option func(*Server)

// WithUnaryInterceptor 追加一个 unary 拦截器。
func WithUnaryInterceptor(i grpc.UnaryServerInterceptor) Option {
	return func(s *Server) { s.unary = append(s.unary, i) }
}

// WithUnaryInterceptors 追加多个 unary 拦截器，按注册顺序执行。
func WithUnaryInterceptors(is ...grpc.UnaryServerInterceptor) Option {
	return func(s *Server) { s.unary = append(s.unary, is...) }
}

// WithStreamInterceptor 追加一个 stream 拦截器。
func WithStreamInterceptor(i grpc.StreamServerInterceptor) Option {
	return func(s *Server) { s.stream = append(s.stream, i) }
}

// WithStreamInterceptors 追加多个 stream 拦截器，按注册顺序执行。
func WithStreamInterceptors(is ...grpc.StreamServerInterceptor) Option {
	return func(s *Server) { s.stream = append(s.stream, is...) }
}

// WithCredentials 设置传输凭证（TLS / mTLS）。
func WithCredentials(creds credentials.TransportCredentials) Option {
	return func(s *Server) {
		if creds != nil {
			s.srvOpts = append(s.srvOpts, grpc.Creds(creds))
		}
	}
}

// WithMaxMsgSize 设置消息大小上限。
func WithMaxMsgSize(recv, send int) Option {
	return func(s *Server) {
		if recv > 0 {
			s.srvOpts = append(s.srvOpts, grpc.MaxRecvMsgSize(recv))
		}
		if send > 0 {
			s.srvOpts = append(s.srvOpts, grpc.MaxSendMsgSize(send))
		}
	}
}

// WithKeepalive 设置 keepalive 参数。
func WithKeepalive(params keepalive.ServerParameters) Option {
	return func(s *Server) { s.srvOpts = append(s.srvOpts, grpc.KeepaliveParams(params)) }
}

// WithServerOption 透传原生 grpc.ServerOption。
//
// 用于本库没有单独封装的能力，典型场景：
//
//	// OpenTelemetry（官方推荐的 stats.Handler 方式）
//	grpcserver.WithServerOption(grpc.StatsHandler(otelgrpc.NewServerHandler()))
//
//	// 自定义编码器 / 压缩 / 拦截器链
//	grpcserver.WithServerOption(grpc.ForceServerCodec(codec))
//
// 它和 WithUnaryInterceptor 等选项可以混用：拦截器链在最后统一注册，
// 所以不会和这里传入的 grpc.ChainUnaryInterceptor 冲突——除非你自己也传了
// 一个 ChainUnaryInterceptor，那两条链都会生效，顺序由 grpc 内部决定。
func WithServerOption(opts ...grpc.ServerOption) Option {
	return func(s *Server) { s.srvOpts = append(s.srvOpts, opts...) }
}

// New 创建 gRPC 服务端。
//
// 【典型用法】
//
//	srv := grpcserver.New(grpcserver.Config{Addr: ":9090"},
//	    grpcserver.WithUnaryInterceptor(grpcserver.RecoveryUnaryInterceptor()),
//	    grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor()),
//	)
//	pb.RegisterOrderServiceServer(srv.GRPC(), impl)
func New(cfg Config, opts ...Option) *Server {
	c := cfg.withDefaults()

	s := &Server{cfg: c}

	s.srvOpts = []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(c.MaxSendMsgSize),
	}

	for _, opt := range opts {
		opt(s)
	}

	// 拦截器放在最后注册，保证它们在最外层。
	if len(s.unary) > 0 {
		s.srvOpts = append(s.srvOpts, grpc.ChainUnaryInterceptor(s.unary...))
	}
	if len(s.stream) > 0 {
		s.srvOpts = append(s.srvOpts, grpc.ChainStreamInterceptor(s.stream...))
	}

	s.srv = grpc.NewServer(s.srvOpts...)
	return s
}

// GRPC 返回原生 *grpc.Server，用于注册 protoc 生成的 service。
func (s *Server) GRPC() *grpc.Server {
	return s.srv
}

// RegisterHealthService 注册 gRPC 标准健康检查服务。
//
// 【为什么用标准协议】K8s 的 gRPC 探针直接调用 grpc.health.v1.Health/Check，
// 不用标准协议就得自己写探针脚本。服务名默认取 appctx.ServerName()。
func (s *Server) RegisterHealthService() {
	if s.healthSrv != nil {
		return
	}
	s.healthSrv = health.NewServer()
	grpc_health_v1.RegisterHealthServer(s.srv, s.healthSrv)
	s.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	if name := appctx.Default().Service; name != "" {
		s.healthSrv.SetServingStatus(name, grpc_health_v1.HealthCheckResponse_SERVING)
	}
}

// SetHealthStatus 更新某个服务的健康状态。
func (s *Server) SetHealthStatus(service string, serving bool) {
	if s.healthSrv == nil {
		return
	}
	st := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if serving {
		st = grpc_health_v1.HealthCheckResponse_SERVING
	}
	s.healthSrv.SetServingStatus(service, st)
}

// RegisterReflection 注册服务端反射。
//
// 【价值】开启后可以用 grpcurl 直接调用，不用写客户端代码就能自检：
//
//	grpcurl -plaintext localhost:9090 list
//	grpcurl -plaintext -d '{"id":"1"}' localhost:9090 order.v1.OrderService/GetOrder
func (s *Server) RegisterReflection() {
	reflection.Register(s.srv)
}

// Listen 监听端口但不开始服务。用于测试或自定义启动流程。
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	return nil
}

// Addr 返回实际监听地址。Listen 或 Serve 之后有效。
func (s *Server) Addr() string {
	if s.listener == nil {
		return s.cfg.Addr
	}
	return s.listener.Addr().String()
}

// Serve 开始服务，阻塞直到 Shutdown 被调用。
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}

	// ctx 取消时优雅关闭。
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		_ = s.Shutdown(shutdownCtx)
	}()

	err := s.srv.Serve(s.listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

// Shutdown 优雅关闭。
//
// 先等在途请求处理完（GracefulStop），超时后强制 Stop。
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		s.srv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.srv.Stop()
		return ctx.Err()
	}
}

// ListenAndServe 是最简启动方式：监听并阻塞服务。
func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.Listen(); err != nil {
		return fmt.Errorf("grpcserver: listen %s: %w", s.cfg.Addr, err)
	}
	return s.Serve(ctx)
}
