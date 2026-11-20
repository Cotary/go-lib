package log

import (
	"io"
	"os"
	"path/filepath"

	"github.com/natefinch/lumberjack"
)

// Sink 是日志的输出端：stdout + 可选的文件滚动。
//
// 它不属于 Logger —— Logger 只负责结构化记录，Sink 只负责"字节写到哪里"
// 和文件句柄的生命周期。需要原样输出（比如 GORM 的 SQL 文本）的组件
// 直接持有 Sink，而不是让 Logger 长出一个 Raw 方法。
type Sink struct {
	io.Writer
	closer io.Closer
}

// Close 关闭底层文件 writer，应在程序退出前调用。
// 没有文件输出（Path 为空）时是空操作，可重复调用。
func (s *Sink) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

func newSink(c Config) *Sink {
	writers := []io.Writer{os.Stdout}
	var closer io.Closer

	if c.Path != "" {
		lj := &lumberjack.Logger{
			Filename:   filepath.Join(c.Path, c.FileName+c.FileSuffix),
			MaxSize:    int(c.MaxSize),
			MaxBackups: int(c.MaxBackups),
			MaxAge:     int(c.MaxAge),
			Compress:   c.Compress,
			LocalTime:  true,
		}
		writers = append(writers, lj)
		closer = lj
	}

	return &Sink{Writer: io.MultiWriter(writers...), closer: closer}
}
