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

func SafeGo(ctx context.Context, F func()) {
	go func() {
		SafeFunc(ctx, F)
	}()
}

func SafeFunc(ctx context.Context, F func()) {
	defer func() {
		if r := recover(); r != nil {
			err := errors.New(utils.Json(r) + "\r\n" + string(debug.Stack()))
			e.SendMessage(ctx, err)
		}
	}()
	F()
}

func NewContext(contextType string) context.Context {
	requestID := contextType + uuid.NewString()
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
