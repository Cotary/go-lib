package lib

import (
	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

func Init(serverName, env string) {
	appctx.Init(serverName, env)
}

func InitLog(logger log.Logger) {
	log.SetGlobalLogger(logger)
}

func InitGlobalSender(sender message.Sender) {
	message.SetGlobalSender(sender)
}
