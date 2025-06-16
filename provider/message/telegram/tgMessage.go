package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	e "go-lib/err"
)

type Config struct {
	Token string `yaml:"token"`
	Debug bool   `yaml:"debug"`
}

type Robot struct {
	*tgbotapi.BotAPI
	Config
}

func NewTelegramRobot(conf Config) (*Robot, error) {
	bot, err := tgbotapi.NewBotAPI(conf.Token)
	if err != nil {
		return nil, e.Err(err)
	}
	bot.Debug = conf.Debug
	return &Robot{
		bot,
		conf,
	}, nil

}

func (t *Robot) SendMessage(chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	return t.Send(msg)

}

// Send MessageConfig Field:
// BaseChat: 包含基本的聊天信息，如聊天 ID、聊天类型等。
// Text: 要发送的消息文本内容。
// ParseMode: 指定消息文本的解析模式，可以是 Markdown、MarkdownV2 或 HTML，用于格式化消息文本。
// Entities: 一个 MessageEntity 数组，用于指定消息文本中的特殊实体（如链接、粗体文本等）。
// DisableWebPagePreview: 一个布尔值，指示是否禁用消息中的网页预览。
func (t *Robot) Send(msg tgbotapi.MessageConfig) error {
	_, err := t.BotAPI.Send(msg)
	if err != nil {
		return e.Err(err)
	}
	return err

}
