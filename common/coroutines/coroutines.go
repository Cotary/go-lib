package coroutines

import (
	"context"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/notify"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func SafeGo(ctx context.Context, F func(ctx context.Context)) {
	go func() {
		SafeFunc(ctx, F)
	}()
}

// SafeFunc 在当前 goroutine 内执行 F，并在 defer 中 recover()。
//
// 关于 panic 的协程语义：panic 是协程级事件，只能在抛出它的那个
// goroutine 的 defer 中 recover；别的 goroutine 拦不到。所以每个
// 新启动的 goroutine 都必须经过 SafeFunc / SafeGo 包装，少包一个
// 就漏一个。注意：这里只能拦 L2 panic，runtime 抛出的 L3 fatal
// error（concurrent map writes 等）会跳过 defer，由 crash.go 的
// InitCrashReporter 落盘 + 下次启动补报负责。
//
// 这里区分 r 的类型：原生 error 用 WithStack 保留底层信息以兼容
// e.GetErrMessage 的渲染；非 error 值用 Errorf 转字符串。最终用
// WithMessage 把 debug.Stack() 拼上去，避免 recover 处吃掉 panic
// 现场（pkg/errors 的 stack 只到 recover 这一帧，不能反推真实抛点）。
func SafeFunc(ctx context.Context, F func(ctx context.Context)) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		var err error
		if e, ok := r.(error); ok {
			err = errors.WithStack(e)
		} else {
			err = errors.Errorf("panic: %v", r)
		}
		err = errors.WithMessage(err, "stack:\n"+string(debug.Stack()))
		notify.SendErrMessage(ctx, err)
	}()
	F(ctx)
}

// Retry 自己维护重试sleep时间
func Retry(ctx context.Context, F func(ctx context.Context) error, count ...int) error {
	maxRetries := -1
	if len(count) > 0 {
		maxRetries = count[0]
	}

	retries := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := F(ctx)
			if err != nil {
				retries++
				if maxRetries >= 0 && retries > maxRetries {
					notify.SendErrMessage(ctx, errors.WithMessage(err, "Retry Error: Exceeded max retries"))
					return err
				}
				notify.SendErrMessage(ctx, errors.WithMessage(err, "Retry Error"))
			} else {
				return nil
			}
		}
	}
}

func NewContext(contextType string) context.Context {
	requestID := contextType + "-" + uuid.NewString()
	ctx := context.Background()
	ctx = context.WithValue(ctx, defined.ServerName, appctx.ServerName())
	ctx = context.WithValue(ctx, defined.ENV, appctx.Env())
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

// Deprecated: SafeCloseChan 存在丢消息风险（会读走一条数据后才关闭），
// 请使用 common/utils.SafeCloser 替代。
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
			goto done
		case semaphore <- item:
		}
	}
done:

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
