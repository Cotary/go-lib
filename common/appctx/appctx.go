package appctx

import "sync"

var (
	serverName string
	env        string
	mu         sync.RWMutex
)

func Init(name, e string) {
	mu.Lock()
	defer mu.Unlock()
	serverName = name
	env = e
}

func ServerName() string {
	mu.RLock()
	defer mu.RUnlock()
	return serverName
}

func Env() string {
	mu.RLock()
	defer mu.RUnlock()
	return env
}
