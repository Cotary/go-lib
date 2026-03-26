package utils

import "time"

// IsWithinExpire 判断给定秒级时间戳与当前时间的差值是否在 expire 以内
func IsWithinExpire(timestampSec int64, expire time.Duration) bool {
	now := time.Now().UnixMilli()
	diff := now - timestampSec*1000
	if diff < 0 {
		diff = -diff
	}
	return diff <= expire.Milliseconds()
}
