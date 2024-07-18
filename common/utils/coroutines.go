package utils

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go-lib"
	"go-lib/common/defined"
	e "go-lib/err"
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
			err := errors.New(Json(r) + "\r\n" + string(debug.Stack()))
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
