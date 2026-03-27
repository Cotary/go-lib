package handler

import (
	"context"
	"encoding/json"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		//requestID
		xRequestID := uuid.NewString()
		c.Writer.Header().Set(defined.RequestID, xRequestID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestID, xRequestID))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ServerName, appctx.ServerName()))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.ENV, appctx.Env()))

		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestURI, c.Request.RequestURI))
		body, err := utils.GetRequestBody(c)
		if err == nil && json.Valid(body) {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), defined.RequestBodyJson, string(body)))
		}
		c.Next()
	}
}
