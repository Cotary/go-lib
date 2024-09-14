package message

import (
	"context"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
)

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error
}

type Message struct {
	Ctx     context.Context
	Title   string
	Content *utils.ZMap[string, string]
}

type AsyncSender struct {
	sender  Sender
	message chan Message
}

func NewAsyncSender(sender Sender, bufferSize int) *AsyncSender {
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

func (a *AsyncSender) Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) {
	a.message <- Message{ctx, title, zMap}
}
func (a *AsyncSender) consumeZMap() {
	for message := range a.message {
		if a.sender != nil {
			err := a.sender.Send(message.Ctx, message.Title, message.Content)
			if err != nil && log.DefaultLogger != nil {
				log.WithContext(message.Ctx).WithFields(map[string]interface{}{
					"title":   message.Title,
					"message": message,
				}).Error(err)
			}
		}
	}
}
