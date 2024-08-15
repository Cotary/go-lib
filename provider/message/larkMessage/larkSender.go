package larkMessage

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
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
	res, err := larkRobot.SendMessage("en-US", title, message, s.AtList, false)
	if err != nil {
		logger := log.WithContext(ctx).
			WithField("type", "larkRobotMsgError")
		if res != nil {
			logger.WithField("response", res.String())
		}
		logger.Error(err)
		return err
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
