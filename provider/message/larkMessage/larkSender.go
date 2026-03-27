package larkMessage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Cotary/go-lib/cache"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
)

type LarkSender struct {
	RobotPath      string
	Secret         string
	AtList         []string
	robot          *LarkRobot
	cacheInst      cache.Cache[bool]
	language       string
	cacheExpireSec int
}

// Send 发送消息，如果启用了缓存，会检查缓存避免重复发送相同内容
func (s *LarkSender) Send(ctx context.Context, title string, zMap *utils.OrderedMap[string, string]) error {
	// 如果启用了缓存，先检查缓存
	if s.cacheInst != nil {
		cacheKey := s.generateCacheKey(title, zMap)
		_, err := s.cacheInst.Get(ctx, cacheKey)
		if err == nil {
			// 缓存命中，直接返回，不发送消息
			return nil
		}
	}

	// 发送消息
	_, err := s.robot.SendMessage(ctx, s.language, title, zMap, s.AtList)
	if err != nil {
		return e.Err(err)
	}

	// 如果启用了缓存，设置缓存
	if s.cacheInst != nil {
		cacheKey := s.generateCacheKey(title, zMap)
		_ = s.cacheInst.Set(ctx, cacheKey, true)
	}

	return nil
}

// generateCacheKey 生成缓存键，基于消息内容
func (s *LarkSender) generateCacheKey(title string, zMap *utils.OrderedMap[string, string]) string {
	keyData := map[string]interface{}{
		"title":   title,
		"content": zMap,
	}
	keyBytes, _ := json.Marshal(keyData)
	return utils.MD5Sum(string(keyBytes))
}

// NewLarkSender 创建新的 LarkSender，默认启用内存缓存，过期时间为 60 秒
func NewLarkSender(robotPath, secret string, atList []string) *LarkSender {
	sender := &LarkSender{
		RobotPath:      robotPath,
		Secret:         secret,
		AtList:         atList,
		robot:          NewLarkRobot(robotPath, secret),
		language:       "en-US",
		cacheExpireSec: 60, // 默认 60 秒
	}

	// 初始化默认内存缓存
	sender.initCache()

	return sender
}

// initCache 初始化缓存实例
func (s *LarkSender) initCache() {
	if s.cacheExpireSec == 0 {
		s.cacheInst = nil
		return
	}

	c, err := cache.NewMemory[bool](cache.MemoryConfig{
		MaxSize:    10000,
		DefaultTTL: time.Duration(s.cacheExpireSec) * time.Second,
	})
	if err != nil {
		return
	}
	s.cacheInst = c
}

// SetCacheExpire 设置缓存过期时间（秒），如果设置为 0 则关闭缓存功能
func (s *LarkSender) SetCacheExpire(seconds int) {
	s.cacheExpireSec = seconds
	s.initCache()
}
