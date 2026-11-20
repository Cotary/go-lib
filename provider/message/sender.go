package message

import (
	"context"
	"log/slog"

	"github.com/Cotary/go-lib/common/utils"
)

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.OrderedMap[string, string]) error
}

func SendMsg(ctx context.Context, title string, msg *utils.OrderedMap[string, string]) {
	sendErr := Default().Send(ctx, title, msg)
	if sendErr != nil {
		slog.ErrorContext(ctx, "SendMsg failed",
			"action", "SendMsg",
			"title", title,
			"message", msg.String(),
			"err", sendErr,
		)
	}
}
