package message

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
)

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.ZMap[string, string]) error
}
