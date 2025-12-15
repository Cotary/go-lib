package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Cotary/go-lib/cache"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/eko/gocache/lib/v4/store"
	gocache_store "github.com/eko/gocache/store/go_cache/v4"
	gocache "github.com/patrickmn/go-cache"
)

type TGSender struct {
	RobotToken     string
	GroupChatID    int64
	robot          *Robot
	cacheInst      cache.Cache[bool]
	cacheStore     store.StoreInterface
	cacheExpireSec int
}

// Send 发送消息，如果启用了缓存，会检查缓存避免重复发送相同内容
func (s *TGSender) Send(ctx context.Context, title string, zMap *utils.OrderedMap[string, string]) error {
	// 如果启用了缓存，先检查缓存
	if s.cacheInst != nil {
		cacheKey := s.generateCacheKey(title, zMap)
		_, err := s.cacheInst.Get(ctx, cacheKey)
		if err == nil {
			// 缓存命中，直接返回，不发送消息
			return nil
		}
	}

	// 构建消息内容
	msg := s.buildMessage(title, zMap)

	// 发送消息
	err := s.robot.SendMessage(s.GroupChatID, msg)
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

// buildMessage 构建消息内容，使用 strings.Builder 优化性能
func (s *TGSender) buildMessage(title string, zMap *utils.OrderedMap[string, string]) string {
	var builder strings.Builder
	builder.WriteString("***")
	builder.WriteString(utils.EscapeMarkdownV2(title))
	builder.WriteString("***\n\n")

	if zMap != nil {
		zMap.Each(func(p utils.Pair[string, string]) {
			builder.WriteString(utils.EscapeMarkdownV2(p.Key))
			builder.WriteString(": ")
			builder.WriteString(utils.EscapeMarkdownV2(p.Value))
			builder.WriteString("\n")
		})
	}

	return builder.String()
}

// generateCacheKey 生成缓存键，基于消息内容
func (s *TGSender) generateCacheKey(title string, zMap *utils.OrderedMap[string, string]) string {
	// 构建缓存键：title + zMap 的 JSON 序列化
	keyData := map[string]interface{}{
		"title":   title,
		"chat_id": s.GroupChatID,
	}
	if zMap != nil {
		content := make(map[string]string)
		zMap.Each(func(p utils.Pair[string, string]) {
			content[p.Key] = p.Value
		})
		keyData["content"] = content
	}
	keyBytes, _ := json.Marshal(keyData)
	return utils.MD5Sum(string(keyBytes))
}

// NewTelegramSender 创建新的 TGSender，默认启用内存缓存，过期时间为 60 秒
func NewTelegramSender(token string, chatID int64) (*TGSender, error) {
	robot, err := NewTelegramRobot(Config{
		Token: token,
		Debug: true,
	})
	if err != nil {
		return nil, e.Err(err)
	}

	sender := &TGSender{
		RobotToken:     token,
		GroupChatID:    chatID,
		robot:          robot,
		cacheExpireSec: 60, // 默认 60 秒
	}

	// 初始化默认内存缓存
	sender.initCache()

	return sender, nil
}

// initCache 初始化缓存实例
func (s *TGSender) initCache() {
	// 如果过期时间为 0，关闭缓存功能
	if s.cacheExpireSec == 0 {
		s.cacheInst = nil
		return
	}

	// 确定缓存存储，如果为 nil 则使用默认的内存缓存
	storeInstance := s.cacheStore
	if storeInstance == nil {
		gocacheClient := gocache.New(5*time.Minute, 10*time.Minute)
		storeInstance = gocache_store.NewGoCache(gocacheClient)
	}

	// 初始化缓存
	s.cacheInst = cache.StoreInstance[bool](
		cache.Config[bool]{
			Prefix: "telegram_message",
			Expire: time.Duration(s.cacheExpireSec) * time.Second,
		},
		storeInstance,
	)
}

// SetCacheExpire 设置缓存过期时间（秒），如果设置为 0 则关闭缓存功能
func (s *TGSender) SetCacheExpire(seconds int) {
	s.cacheExpireSec = seconds
	s.initCache()
}
