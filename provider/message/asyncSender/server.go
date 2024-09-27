package asyncSender

import (
	"context"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

type Message struct {
	Ctx     context.Context
	Title   string
	Content *utils.ZMap[string, string]
}

type AsyncSender struct {
	sender  message.Sender
	message chan Message
}

func NewAsyncSender(sender message.Sender, bufferSize int) *AsyncSender {
	asyncSender := &AsyncSender{
		sender:  sender,
		message: make(chan Message, bufferSize),
	}
	newCtx := coroutines.NewContext("messageSender")
	coroutines.SafeGo(newCtx, func(ctx context.Context) {
		asyncSender.consumeZMap()
	})
	return asyncSender
}

func (a *AsyncSender) Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error {
	a.message <- Message{ctx, title, zMap}
	return nil
}
func (a *AsyncSender) consumeZMap() {
	for msg := range a.message {
		if a.sender != nil {
			err := a.sender.Send(msg.Ctx, msg.Title, msg.Content)
			if err != nil {
				log.WithContext(msg.Ctx).WithFields(map[string]interface{}{
					"title":   msg.Title,
					"message": msg.Content,
				}).Error(err.Error())
			}
		}
	}
}
