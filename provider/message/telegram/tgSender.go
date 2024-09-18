package telegram

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
)

type TGSender struct {
	RobotToken  string
	GroupChatID int64
}

func (s *TGSender) Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error {
	var message []string
	zMap.Each(func(p utils.Pair[string, string]) {
		message = append(message, p.Key+": ", p.Value)
	})
	robot, err := NewTelegramRobot(Config{
		Token: s.RobotToken,
		Debug: true,
	})
	if err != nil {
		return e.Err(err)
	}
	msg := fmt.Sprintf("**%s**", title)
	zMap.Each(func(p utils.Pair[string, string]) {
		msg = msg + fmt.Sprintf("\n%s: %s", p.Key, p.Value)
	})

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
