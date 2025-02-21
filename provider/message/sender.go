package message

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
)

var globalSender Sender

func SetGlobalSender(sender Sender) {
	globalSender = sender
}
func GetPrioritySender(sender Sender) Sender {
	if sender == nil {
		return globalSender
	}
	return sender
}

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error
}
