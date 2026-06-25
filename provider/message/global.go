package message

import "sync"

var (
	globalSender Sender
	mu           sync.RWMutex
)

func SetGlobalSender(sender Sender) {
	mu.Lock()
	defer mu.Unlock()
	globalSender = sender
}

func GetGlobalSender() Sender {
	mu.RLock()
	defer mu.RUnlock()
	return globalSender
}
