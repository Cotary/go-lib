package coroutines

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"reflect"
	"runtime/debug"
	"sync"
)

func SafeGo(ctx context.Context, F func(ctx context.Context)) {
	go func() {
		SafeFunc(ctx, F)
	}()
}

func SafeFunc(ctx context.Context, F func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("Recover: %v \n Stack: %s", r, string(debug.Stack()))
			err := errors.New(msg)
			e.SendMessage(ctx, err)
		}
	}()
	F(ctx)
}

// Retry 自己维护重试sleep时间
func Retry(ctx context.Context, F func(ctx context.Context) error, count ...int) {
	maxRetries := -1
	if len(count) > 0 {
		maxRetries = count[0]
	}

	retries := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := F(ctx)
			if err != nil {
				retries++
				if maxRetries >= 0 && retries > maxRetries {
					e.SendMessage(ctx, errors.WithMessage(err, "Retry Error: Exceeded max retries"))
					return
				}
				e.SendMessage(ctx, errors.WithMessage(err, "Retry Error"))
			} else {
				return
			}
		}
	}
}

func NewContext(contextType string) context.Context {
	requestID := contextType + "-" + uuid.NewString()
	ctx := context.Background()
	ctx = context.WithValue(ctx, defined.ServerName, lib.ServerName)
	ctx = context.WithValue(ctx, defined.ENV, lib.Env)
	ctx = context.WithValue(ctx, defined.RequestID, requestID)
	return ctx
}

func GetStructName(i interface{}) string {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Ptr {
		return "*" + t.Elem().Name()
	}
	return t.Name()
}

// SafeCloseChan 使用泛型安全地关闭任意类型的通道
func SafeCloseChan[T any](ch chan T) {
	select {
	case _, open := <-ch:
		if !open {
			return
		}
		close(ch)
	default:
		close(ch)
	}
}

// ConcurrentProcessor 控制并发数
func ConcurrentProcessor[T any](ctx context.Context, limit int, items []T, processItem func(ctx context.Context, item T)) {
	semaphore := make(chan T, limit)
	var wg sync.WaitGroup

	// 启动并发处理协程
	for i := 0; i < limit; i++ {
		wg.Add(1)
		SafeGo(ctx, func(ctx context.Context) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-semaphore:
					if !ok {
						return
					}
					processItem(ctx, item)
				}
			}
		})
	}

	// 将数据发送到通道
	for _, item := range items {
		select {
		case <-ctx.Done():
			break
		case semaphore <- item:
		}
	}

	// 关闭通道并等待所有协程完成
	close(semaphore)
	wg.Wait()
}

// ConcurrentProcessorChan 控制并发数
func ConcurrentProcessorChan[T any](ctx context.Context, limit int, semaphore chan T, processItem func(ctx context.Context, item T)) {
	var wg sync.WaitGroup
	// 启动并发处理协程
	for i := 0; i < limit; i++ {
		wg.Add(1)
		SafeGo(ctx, func(ctx context.Context) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-semaphore:
					if !ok {
						return
					}
					processItem(ctx, item)
				}
			}
		})
	}
	wg.Wait()
}
