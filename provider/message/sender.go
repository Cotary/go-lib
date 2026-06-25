package message

import (
	"context"
	"github.com/Cotary/go-lib/log"
	"sync"

	"github.com/Cotary/go-lib/common/utils"
)

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.OrderedMap[string, string]) error
}

func SendMsg(ctx context.Context, title string, msg *utils.OrderedMap[string, string]) {
	sender := GetGlobalSender()
	if sender == nil {
		return
	}
	sendErr := sender.Send(ctx, title, msg)
	if sendErr != nil {
		log.WithContext(ctx).WithFields(map[string]interface{}{
			"action":  "SendMsg",
			"title":   title,
			"message": msg.String(),
		}).Error(sendErr.Error())
	}
}
