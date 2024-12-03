package coroutines

import (
	"context"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"reflect"
	"runtime/debug"
)

func SafeGo(ctx context.Context, F func(ctx context.Context)) {
	go func() {
		SafeFunc(ctx, F)
	}()
}

func SafeFunc(ctx context.Context, F func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			err := errors.New(utils.AnyToString(r) + "\r\n" + string(debug.Stack()))
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
