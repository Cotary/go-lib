package log

import (
	"io"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// RotateConfig 是轮转 writer 的配置，零值使用默认值：
// ./logs/runtime.log、10MB、保留 30 天、stdout + 文件。
type RotateConfig struct {
	Path       string // 日志目录，默认 ./logs/
	FileName   string // 文件名（不含后缀），默认 runtime
	FileSuffix string // 后缀，默认 .log
	MaxSize    int    // 单文件上限 MB，默认 10
	MaxAge     int    // 保留天数，默认 30；-1 表示不限
	MaxBackups int    // 最大备份数，0 不限
	Compress   bool   // 归档是否压缩
}

// RotateWriter 是带轮转的 io.Writer，同时实现 io.Closer。
type RotateWriter struct {
	io.Writer
	closer io.Closer
}

func (w *RotateWriter) Close() error { return w.closer.Close() }

// NewWriter 创建轮转 writer：固定写 stdout + 文件，返回的 RotateWriter 可直接
// 传给 log.JSON / log.Text / log.Raw，退出前调用 Close。
func NewWriter(cfg RotateConfig) RotateWriter {
	c := cfg
	if c.Path == "" {
		c.Path = "./logs/"
	}
	if c.FileName == "" {
		c.FileName = "runtime"
	}
	if c.FileSuffix == "" {
		c.FileSuffix = ".log"
	}
	if c.MaxSize <= 0 {
		c.MaxSize = 10
	}
	if c.MaxAge == 0 {
		c.MaxAge = 30
	} else if c.MaxAge < 0 {
		c.MaxAge = 0
	}
	if c.MaxBackups < 0 {
		c.MaxBackups = 0
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(c.Path, c.FileName+c.FileSuffix),
		MaxSize:    c.MaxSize,
		MaxBackups: c.MaxBackups,
		MaxAge:     c.MaxAge,
		Compress:   c.Compress,
		LocalTime:  true,
	}

	return RotateWriter{
		Writer: io.MultiWriter(os.Stdout, lj),
		closer: lj,
	}
}
