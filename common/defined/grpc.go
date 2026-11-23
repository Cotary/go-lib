package defined

// gRPC metadata key（规范要求全小写）。
//
// 这是「客户端写、服务端读」的线上契约，两侧必须同源：
// 各写一份字面量，改一边就会静默丢数据。
const (
	MDRequestID     = "x-request-id"
	MDToken         = "x-token"
	MDAppid         = "x-appid"
	MDNonce         = "x-nonce"
	MDSign          = "x-sign"
	MDSignTimestamp = "x-signtimestamp"
	MDSignType      = "x-signtype"
	MDAuthorization = "authorization"
	MDCookie        = "cookie"
	MDContentType   = "content-type"
	MDUserAgent     = "user-agent"
)

// google.rpc.ErrorInfo.metadata 的 key：服务端写、客户端读，必须同源。
const (
	MDBizCode = "biz_code"
	MDData    = "data"
)

// BearerScheme 是 authorization 的认证方案前缀（不含空格）。
const BearerScheme = "Bearer"

// UnknownGRPCDomain 是服务名缺失时 ErrorInfo.domain 的兜底值。
//
// 不用库名充数：domain 表示「错误由哪个服务产生」，填 "go-lib"
// 等于谎称存在那个服务。属于线上契约，改动要同步。
const UnknownGRPCDomain = "unknown"

// 日志字段名：是 ELK 查询语句的一部分，写错编译期发现不了，查询会漏数据。
const (
	LogKeyMethod   = "method"
	LogKeyCostMS   = "cost_ms"
	LogKeyTarget   = "target"
	LogKeyServer   = "server"
	LogKeyCode     = "code"
	LogKeyError    = "error"
	LogKeyMetadata = "metadata"
	LogKeyPanic    = "panic"
	LogKeyStack    = "stack"
)

// 日志消息文本，同样是检索入口，两侧必须同源。
const (
	LogMsgGRPCCall           = "gRPC call"
	LogMsgGRPCRequest        = "gRPC request"
	LogMsgGRPCRequestSlow    = "gRPC request slow"
	LogMsgGRPCStream         = "gRPC stream"
	LogMsgGRPCPanicRecovered = "gRPC panic recovered"
)

// 会原样出现在 status.message 里的文案，客户端可能直接展示或按文案断言。
const (
	MsgInternalError     = "internal error"
	MsgRateLimitExceeded = "rate limit exceeded"
	MsgRequestTimeout    = "request timeout"
)
