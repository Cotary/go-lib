package log

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/log/handler"
)

const (
	FormatJSON = "json"
	FormatText = "text"
)

// 公开类型转发到 log/handler，调用方仍然只 import log。
type (
	Alert        = handler.Alert
	Alerter      = handler.Alerter
	ContextField = handler.ContextField
)

// AlertKey 是"这条日志要告警"的标记字段名。
const AlertKey = handler.AlertKey

// Config 是日志输出端的配置。Path 为空表示只写 stdout，不落文件。
type Config struct {
	Level      string `mapstructure:"level" yaml:"level"`
	Path       string `mapstructure:"path" yaml:"path"`
	FileName   string `mapstructure:"fileName" yaml:"fileName"`
	FileSuffix string `mapstructure:"fileSuffix" yaml:"fileSuffix"`
	MaxAge     int64  `mapstructure:"maxAge" yaml:"maxAge"`
	MaxBackups int64  `mapstructure:"maxBackups" yaml:"maxBackups"`
	MaxSize    int64  `mapstructure:"maxSize" yaml:"maxSize"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
	ShowFile   bool   `mapstructure:"showFile" yaml:"showFile"`
	Format     string `mapstructure:"format" yaml:"format"`
}

// New 构造日志。返回值是标准库 *slog.Logger，不做任何包装，
// 因此 AddSource 记录的调用点就是业务代码。
//
// 格式化、级别过滤、脱敏、request_id、告警、采样都在 Handler 链里完成；
// Sink 负责输出端和文件句柄，调用方退出前 Close。
func New(cfg Config, opts ...Option) (*slog.Logger, *Sink, error) {
	c, level, err := normalize(cfg)
	if err != nil {
		return nil, nil, err
	}

	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	sink := newSink(c)
	h := handler.New(sink.Writer, handler.Config{
		Level:         level,
		AddSource:     c.ShowFile,
		Console:       c.Format == FormatText,
		Alerter:       o.alerter,
		RedactKeys:    o.redactKeys,
		ContextFields: o.contextFields,
		SampleEvery:   o.sampleEvery,
		SampleBelow:   o.sampleBelow,
		Fields:        o.fields,
	})

	return slog.New(h), sink, nil
}

func normalize(cfg Config) (Config, slog.Level, error) {
	c := cfg

	level, err := parseLevel(c.Level)
	if err != nil {
		return c, 0, err
	}
	c.Level = strings.ToLower(strings.TrimSpace(c.Level))

	c.Format = strings.ToLower(strings.TrimSpace(c.Format))
	if c.Format == "" {
		c.Format = FormatJSON
	}
	if c.Format != FormatJSON && c.Format != FormatText {
		return c, 0, fmt.Errorf("log: unknown format %q", cfg.Format)
	}

	if c.Path != "" {
		if c.FileName == "" {
			c.FileName = "runtime"
		}
		if c.FileSuffix == "" {
			c.FileSuffix = ".logger"
		}
		if c.MaxAge == 0 {
			c.MaxAge = 30
		} else if c.MaxAge < 0 {
			c.MaxAge = 0
		}
		if c.MaxSize == 0 {
			c.MaxSize = 10
		}
		if c.MaxBackups < 0 {
			c.MaxBackups = 0
		}
	}

	return c, level, nil
}

// parseLevel 支持常见别名，便于从配置文件直接映射。
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info", "information", "notice":
		return slog.LevelInfo, nil
	case "debug", "trace", "verbose":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error", "err", "fatal":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("log: unknown level %q", s)
}

// ---------- Option ----------

type options struct {
	alerter       Alerter
	redactKeys    map[string]struct{}
	contextFields []ContextField
	sampleEvery   uint64
	sampleBelow   slog.Level
	fields        []slog.Attr
}

// Option 定制 Logger 行为。
type Option func(*options)

func defaultOptions() options {
	keys := []string{"password", "token", "secret", "authorization"}
	o := options{
		redactKeys:    make(map[string]struct{}, len(keys)),
		contextFields: []ContextField{{Key: defined.RequestID, Name: defined.RequestID}},
	}
	for _, k := range keys {
		o.redactKeys[k] = struct{}{}
	}
	return o
}

// WithFields 把 map 展开成 slog 参数（按 key 排序，输出稳定）。
//
//	logger.With(log.WithFields(map[string]any{"k": "v"})...).Info("x")
func WithFields(fields map[string]any) []any {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]any, 0, len(fields)*2)
	for _, k := range keys {
		out = append(out, k, fields[k])
	}
	return out
}

// WithAlerter 注入告警出口：带 AlertKey 标记的日志会转给它。
func WithAlerter(a Alerter) Option {
	return func(o *options) { o.alerter = a }
}

// WithRedactKeys 覆盖默认的脱敏字段集合（password/token/secret/authorization）。
func WithRedactKeys(keys ...string) Option {
	return func(o *options) {
		set := make(map[string]struct{}, len(keys))
		for _, k := range keys {
			if k = strings.TrimSpace(k); k != "" {
				set[k] = struct{}{}
			}
		}
		o.redactKeys = set
	}
}

// WithContextFields 设置从 ctx 提取并写入日志的字段列表，默认 request_id。
//
// ctx 里取不到、值为空或已经通过 With 显式传过的字段会被跳过。
// 传空列表等于关闭自动注入。
func WithContextFields(fields ...ContextField) Option {
	return func(o *options) {
		o.contextFields = normalizeFields(fields)
	}
}

// WithRequestIDKeys 是 WithContextFields 的简化写法：key 与字段名同名。
func WithRequestIDKeys(keys ...string) Option {
	fields := make([]ContextField, 0, len(keys))
	for _, k := range keys {
		fields = append(fields, ContextField{Key: k, Name: k})
	}
	return WithContextFields(fields...)
}

func normalizeFields(fields []ContextField) []ContextField {
	out := make([]ContextField, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			if s, ok := f.Key.(string); ok {
				name = strings.TrimSpace(s)
			} else if f.Key != nil {
				name = fmt.Sprint(f.Key)
			}
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContextField{Key: f.Key, Name: name})
	}
	return out
}

// WithSampling 对低于 below 级别的日志按 1/every 抽样；every <= 1 表示关闭。
// 带告警标记的记录不参与抽样。
func WithSampling(every uint64, below slog.Level) Option {
	return func(o *options) {
		o.sampleEvery = every
		o.sampleBelow = below
	}
}

// WithFixedFields 给每条日志附加固定字段（如 service/env/version）。
// 按 key 排序，保证输出稳定。
func WithFixedFields(fields map[string]any) Option {
	return func(o *options) {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			o.fields = append(o.fields, slog.Any(k, fields[k]))
		}
	}
}
