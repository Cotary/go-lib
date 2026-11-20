package notify

import (
	"context"
	"log/slog"

	e "github.com/Cotary/go-lib/err"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

func SendMessageWithLevel(ctx context.Context, err error) {
	codeErr := e.AsCodeErr(err)
	if codeErr != nil && codeErr.Level <= e.WarnLevel {
		SendErrMessage(ctx, err)
	}
}

func SendErrMessage(ctx context.Context, err error) {
	errMsg := e.GetErrMessage(e.Err(err), false)
	id := appctx.OrDefault(ctx)
	env := id.Env
	serverName := id.Service
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)
	requestJson, _ := ctx.Value(defined.RequestBodyJson).(string)

	zMap := utils.NewOrderedMap[string, string]().
		Set("ServerName", serverName).
		Set("Env", env).
		Set("RequestID", requestID).
		Set("RequestUri", requestUri).
		Set("RequestJson", requestJson).
		Set("Error", errMsg)

	slog.Default().With(log.WithFields(map[string]any{
		"serverName":  serverName,
		"env":         env,
		"requestID":   requestID,
		"requestUri":  requestUri,
		"requestJson": requestJson,
		"error":       errMsg,
	})...).ErrorContext(ctx, "SendErrMessage Record")
	if sendErr := message.Default().Send(ctx, "Running Error", zMap); sendErr != nil {
		slog.Default().With("action", "SendErrMessage Error").ErrorContext(ctx, sendErr.Error())
	}
}
