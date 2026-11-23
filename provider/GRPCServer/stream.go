package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// serverStreamWithContext 给 ServerStream 换一个 ctx。
//
// stream 拦截器里拿到的 ss.Context() 是只读的，
// 想把请求 ID 之类的东西传下去必须包一层。
type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

// Context 返回新的 ctx。
func (s *serverStreamWithContext) Context() context.Context {
	return s.ctx
}

// metadataWith 构造只有一个键值对的 metadata。
func metadataWith(key, value string) metadata.MD {
	return metadata.Pairs(key, value)
}

// filterMetadata 按白名单筛出可记录的 metadata。
//
// 敏感 key 无条件剔除：白名单只是「允许」，
// 不是「强制记录」。
func filterMetadata(ctx context.Context, keys []string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	out := make(map[string]string, len(keys))
	for _, key := range keys {
		lower := strings.ToLower(key)
		if sensitiveMetadataKeys[lower] {
			continue
		}
		if v := firstValue(md, lower); v != "" {
			out[lower] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
