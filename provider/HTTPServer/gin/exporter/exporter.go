package exporter

import (
	"context"

	"github.com/Cotary/go-lib/provider/exporter"
	"github.com/gin-gonic/gin"
)

// GinExportContext 是 ExportContext 接口的 gin 实现
type GinExportContext struct {
	c *gin.Context
}

// NewGinExportContext 创建 gin 导出上下文
func NewGinExportContext(c *gin.Context) *GinExportContext {
	return &GinExportContext{c: c}
}

// Context 返回请求的 context.Context
func (g *GinExportContext) Context() context.Context {
	return g.c.Request.Context()
}

// GetHeader 获取请求头
func (g *GinExportContext) GetHeader(key string) string {
	return g.c.Request.Header.Get(key)
}

// SetHeader 设置响应头
func (g *GinExportContext) SetHeader(key, value string) {
	g.c.Header(key, value)
}

// SendFile 发送文件到客户端
func (g *GinExportContext) SendFile(filePath string, contentType string) error {
	g.c.File(filePath)
	return nil
}

// IsDownload 检查请求是否为下载请求
func IsDownload(c *gin.Context) bool {
	return exporter.IsDownloadFromContext(NewGinExportContext(c))
}

// Export 使用 gin.Context 执行导出操作
func Export(c *gin.Context, res interface{}) error {
	exp := exporter.NewExporter()
	return exp.Run(NewGinExportContext(c), res)
}
