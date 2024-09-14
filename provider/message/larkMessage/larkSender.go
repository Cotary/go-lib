package larkMessage

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
)

type LarkSender struct {
	RobotPath string
	Secret    string
	AtList    []string
}

func (s *LarkSender) Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error {
	var message []string
	zMap.Each(func(p utils.Pair[string, string]) {
		message = append(message, p.Key+": ", p.Value)
	})
	larkRobot := NewLarkRobot(s.RobotPath, s.Secret)
	_, err := larkRobot.SendMessage("en-US", title, message, s.AtList, false)
	if err != nil {
		return e.Err(err)
	}
	return nil
}

func NewLarkSender(robotPath, secret string, atList []string) *LarkSender {
	return &LarkSender{
		RobotPath: robotPath,
		Secret:    secret,
		AtList:    atList,
	}
}
