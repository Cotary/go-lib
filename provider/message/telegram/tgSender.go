package telegram

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"strings"
)

type TGSender struct {
	RobotToken  string
	GroupChatID int64
}

func (s *TGSender) Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error {
	robot, err := NewTelegramRobot(Config{
		Token: s.RobotToken,
		Debug: true,
	})
	if err != nil {
		return e.Err(err)
	}
	msg := fmt.Sprintf("***%s***\n\n", escapeMarkdown(title))
	if zMap != nil {
		zMap.Each(func(p utils.Pair[string, string]) {
			msg = msg + fmt.Sprintf("%s: %s\n", escapeMarkdown(p.Key), escapeMarkdown(p.Value))
		})
	}
	err = robot.SendMessage(s.GroupChatID, msg)
	if err != nil {
		return e.Err(err)
	}
	return nil
}

func NewTelegramSender(token string, chatID int64) *TGSender {
	return &TGSender{
		RobotToken:  token,
		GroupChatID: chatID,
	}
}

// escapeMarkdown 为文本中的 Markdown 特殊字符添加反斜杠转义。
func escapeMarkdown(text string) string {
	// 注意：顺序需要注意，先转义反斜杠再转义其他字符。
	specialCharacters := []string{"\\", "`", "*", "_", "{", "}", "[", "]", "(", ")", "#", "+", "-", ".", "!"}
	for _, ch := range specialCharacters {
		text = strings.ReplaceAll(text, ch, "\\"+ch)
	}
	return text
}
