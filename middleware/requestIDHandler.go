package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go-lib"
	"go-lib/common/defined"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		//requestID
		xRequestID := uuid.NewString()
		c.Writer.Header().Set(defined.RequestID, xRequestID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestID, xRequestID))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ServerName, lib.ServerName))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ENV, lib.Env))

		c.Next()
	}
}
