package e

import (
	"context"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

var sender message.Sender

func SetSender(s message.Sender) {
	sender = s
}

func SendMessage(ctx context.Context, err error) {
	errMsg := GetErrMessage(Err(err))
	env := lib.Env
	serverName := lib.ServerName
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)
	requestJson, _ := ctx.Value(defined.RequestBodyJson).(string)

	zMap := utils.NewZMap[string, string]().
		Set("ServerName", serverName).
		Set("Env", env).
		Set("RequestID", requestID).
		Set("RequestUri", requestUri).
		Set("RequestJson", requestJson).
		Set("Error", errMsg)

	log.WithContext(ctx).Error(errMsg)
	errSender := message.GetPrioritySender(sender)
	if errSender == nil {
		return
	}
	sendErr := errSender.Send(ctx, "Running Error", zMap)
	if sendErr != nil {
		log.WithContext(ctx).Error("err sender:" + sendErr.Error())
	}
}
