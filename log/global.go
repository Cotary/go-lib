package log

import (
	"context"
	"sync"

	"github.com/Cotary/go-lib/common/defined"
)

var (
	globalLogger Logger
	mu           sync.RWMutex
	defaultOnce  sync.Once
)

func SetGlobalLogger(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	globalLogger = l
}

func WithContext(ctx context.Context) Logger {
	mu.RLock()
	l := globalLogger
	mu.RUnlock()

	if l == nil {
		defaultOnce.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			if globalLogger == nil {
				globalLogger = NewLogger(&Config{})
			}
		})
		mu.RLock()
		l = globalLogger
		mu.RUnlock()
	}

	fields := make(map[string]any)
	if val := ctx.Value(defined.RequestID); val != nil {
		fields[defined.RequestID] = val
	}
	return l.WithContext(ctx).WithFields(fields)
}
