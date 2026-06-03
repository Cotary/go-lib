//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package defined

// ===== Redis Key 前缀 =====
// 建议格式：业务模块:用途:

const (
	RedisKeyOrderLock    = "order:lock:"   // 订单操作分布式锁
	RedisKeyUserToken    = "user:token:"   // 用户登录 Token
	RedisKeyRateLimit    = "rate:limit:"   // 接口限流计数
	RedisKeyCurrencyRate = "currency:rate" // 汇率缓存
)

// ===== 业务枚举 =====

const (
	CurrencyUSD = "USD"
	CurrencyCNY = "CNY"
	CurrencyEUR = "EUR"
)

// ===== HTTP Header =====

const (
	HeaderAppID = "X-App-ID"
	HeaderToken = "X-Token"
	HeaderLang  = "X-Language"
)
