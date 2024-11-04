package message

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
)

var GlobalSender Sender

func GetPrioritySender(sender Sender) Sender {
	if sender == nil {
		return GlobalSender
	}
	return sender
}

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error
}
