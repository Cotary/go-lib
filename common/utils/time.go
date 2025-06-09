package utils

import "time"

func CheckTimeExpire(timestamp int64, expire time.Duration) bool {
	now := time.Now().UnixMilli()
	diff := now - GetMillTime(timestamp)
	if diff < 0 {
		diff = -diff
	}
	return diff <= expire.Milliseconds()
}
