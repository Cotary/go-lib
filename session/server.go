package session

import (
	"encoding/json"
	"errors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	e "go-lib/err"
)

var Nil = errors.New("session is nil")

type Sess struct {
	sessions.Session
}

func GetSession(c *gin.Context) Sess {
	return Sess{
		Session: sessions.Default(c),
	}
}

func (s Sess) SetStr(key interface{}, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return e.Err(err)
	}
	s.Set(key, string(data))
	err = s.Save()
	if err != nil {
		return e.Err(err)
	}
	return nil
}

func (s Sess) GetStr(key interface{}, data interface{}) error {
	sessionData := s.Get(key)
	if sessionData == nil {
		return Nil
	}
	dataStr, ok := sessionData.(string)
	if !ok {
		return e.Err(errors.New("data is not string"))
	}
	err := json.Unmarshal([]byte(dataStr), data)
	if err != nil {
		return e.Err(err)
	}
	return nil
}
