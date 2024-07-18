package e

import (
	"github.com/gin-gonic/gin"
)

func HTTPErrHandler(c *gin.Context, err error) HttpErr {

	var httpErr HttpErr
	switch typeErr := err.(type) {
	case *CodeErr:
		httpErr = NewHttpErr(typeErr, nil)
	case HttpErr:
		httpErr = typeErr
	default:

		httpErr = NewHttpErr(FailedErr, typeErr)
	}
	httpErr.SendErrorMsg(c.Request.Context())
	return httpErr
}
