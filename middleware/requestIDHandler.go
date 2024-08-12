package middleware

import (
	"context"
	"encoding/json"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		//requestID
		xRequestID := uuid.NewString()
		c.Writer.Header().Set(defined.RequestID, xRequestID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestID, xRequestID))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ServerName, lib.ServerName))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ENV, lib.Env))

		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestURI, c.Request.RequestURI))
		body, err := utils.GetRequestBody(c)
		if err == nil && json.Valid(body) {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestBodyJson, string(body)))
		}
		c.Next()
	}
}
