package lib

import (
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

var ServerName string
var Env string

func Init(serverName, env string) {
	ServerName = serverName
	Env = env
}

func InitLog(logger log.Logger) {
	log.GlobalLogger = logger
}

func InitGlobalSender(sender message.Sender) {
	message.GlobalSender = sender
}
