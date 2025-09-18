package utils

import "time"

// IsWithinExpire 判断给定时间戳与当前时间的差值是否在 expire 以内
// timestamp 支持秒级或毫秒级
func IsWithinExpire(timestamp int64, expire time.Duration) bool {
	now := time.Now().UnixMilli()
	diff := now - GetMillTime(timestamp)
	if diff < 0 {
		diff = -diff
	}
	return diff <= expire.Milliseconds()
}
