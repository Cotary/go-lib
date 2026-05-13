package asyncSender

import (
	"context"

	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

type Message struct {
	Title   string
	Content *utils.OrderedMap[string, string]
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

// Send 异步发送消息，上下文信息应在调用方构造 zMap 时提取写入，此处不依赖 ctx 传递追踪数据
func (a *AsyncSender) Send(_ context.Context, title string, zMap *utils.OrderedMap[string, string]) error {
	a.message <- Message{Title: title, Content: zMap}
	return nil
}

func (a *AsyncSender) consumeZMap() {
	ctx := coroutines.NewContext("messageSender")
	coroutines.ConcurrentProcessorChan(ctx, 10, a.message, func(ctx context.Context, msg Message) {
		if a.sender != nil {
			sendCtx := coroutines.NewContext("asyncSend")
			err := a.sender.Send(sendCtx, msg.Title, msg.Content)
			if err != nil {
				log.WithContext(sendCtx).WithFields(map[string]interface{}{
					"title":   msg.Title,
					"message": msg.Content,
				}).Error(err.Error())
			}
		}
	})
}
