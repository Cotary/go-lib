package middleware

import (
	"bytes"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/gin-gonic/gin"
	"time"
)

type BodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w BodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}
func (w BodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func RequestLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		ctx := c.Request.Context()
		//log
		requestBody, _ := utils.GetRequestBody(c)
		bodyLogWriter := &BodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = bodyLogWriter
		start := time.Now()

		c.Next()

		//log
		end := time.Now()
		responseBody := bodyLogWriter.body.String()
		logField := map[string]interface{}{
			"url":             c.Request.URL.String(),
			"start_timestamp": start.Format(time.DateTime),
			"end_timestamp":   end.Format(time.DateTime),
			"server_name":     c.Request.Host,
			"remote_addr":     c.ClientIP(),
			"proto":           c.Request.Proto,
			"request_method":  c.Request.Method,
			"response_time":   end.Sub(start).Milliseconds(), // 毫秒

			"status": c.Writer.Status(),
			"header": c.Request.Header,

			"request_id":    c.Writer.Header().Get(defined.RequestID),
			"request_body":  string(requestBody),
			"response_body": responseBody,
		}
		log.WithContext(ctx).WithFields(logField).Info("request log")
	}
}
