package message

import (
	"context"
	"sync"

	"github.com/Cotary/go-lib/common/utils"
)

var (
	globalSender Sender
	mu           sync.RWMutex
)

func SetGlobalSender(sender Sender) {
	mu.Lock()
	defer mu.Unlock()
	globalSender = sender
}

func GetPrioritySender(sender Sender) Sender {
	if sender == nil {
		mu.RLock()
		defer mu.RUnlock()
		return globalSender
	}
	return sender
}

type Sender interface {
	Send(ctx context.Context, title string, zMap *utils.OrderedMap[string, string]) error
}
