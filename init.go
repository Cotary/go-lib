package lib

var ServerName string
var Env string

func Init(serverName, env string) {
	ServerName = serverName
	Env = env
}
