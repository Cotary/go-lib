package community

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Cotary/go-lib/common/defined"
)

// TimeRange 通用时间范围请求体，可直接作为 gin 请求结构体绑定 query/form/json 参数。
//
// 字段语义由 TimeRangeType 决定，未传 TimeRangeType 时按 10 位秒级 Unix 时间戳解释。
type TimeRange struct {
	StartTime     int64         `form:"start_time" json:"start_time"`
	EndTime       int64         `form:"end_time" json:"end_time"`
	TimeRangeType TimeRangeType `form:"time_range_type" json:"time_range_type" binding:"omitempty,oneof=timestamp timestamp_ms year_month date"`
}

// TimeRangeType 时间范围的值类型，决定如何解读 StartTime/EndTime 的 int64 值。
// 底层为 string，gin 的 ShouldBind 可直接从 URL query/form 参数绑定。
type TimeRangeType string

const (
	// TimeRangeTimestamp 10位秒级 Unix 时间戳，不传 TimeRangeType 时的默认值
	TimeRangeTimestamp TimeRangeType = "timestamp"
	// TimeRangeTimestampMs 13位毫秒级 Unix 时间戳
	TimeRangeTimestampMs TimeRangeType = "timestamp_ms"
	// TimeRangeYearMonth yyyyMM 格式，如 202604 表示 2026年4月
	TimeRangeYearMonth TimeRangeType = "year_month"
	// TimeRangeDate yyyyMMdd 格式，如 20260414 表示 2026年4月14日
	TimeRangeDate TimeRangeType = "date"
)

// Parse 将 StartTime/EndTime 按 TimeRangeType 解析为 time.Time 对。
//
// tz 可选传入时区，支持两种形式：
//   - string: IANA 时区名，如 "Asia/Shanghai"
//   - *time.Location: 直接传已有的 Location 变量
//
// 不传时默认使用系统本地时区（time.Local）。
//
// 闭区间语义（start == end 时不会返回空区间）：
//   - year_month: start 取该月1日 00:00:00，end 取该月最后一天 23:59:59.999999999
//   - date: start 取该天 00:00:00，end 取该天 23:59:59.999999999
//   - timestamp/timestamp_ms: 直接转换，不做区间扩展
//
// 时区对不同类型的影响：
//   - timestamp/timestamp_ms 是绝对时刻，时区仅影响返回的 time.Time 的 Location
//   - year_month/date 的绝对时刻由时区决定（"20260414" 在 +0800 和 UTC 差8小时）
func (t *TimeRange) Parse(tz ...any) (start, end time.Time, err error) {
	return t.parseRange(tz...)
}

// ToSec 将当前 TimeRange 归一化为秒级时间戳形式，并返回一个新的 TimeRange。
//
// 返回值特征：
//   - TimeRangeType 固定为 TimeRangeTimestamp
//   - StartTime/EndTime 为秒级 Unix 时间戳
//
// 不修改原对象（接收者按值语义处理结果），便于链式调用和并发安全。
//
// 行为说明：
//   - 源类型已是 timestamp 时，无解析开销，原值透传
//   - 源类型是 timestamp_ms 时，按整除 1000 转换（向零截断）
//   - 源类型是 year_month/date 时，end 取所在月/日的 23:59:59
//     （Unix() 会丢弃纳秒，如需毫秒精度请使用 ToMs）
func (t *TimeRange) ToSec(tz ...any) (TimeRange, error) {
	// 快速路径：源已是秒级时间戳，无需走时区解析，避免 time.Time 往返开销
	if t.effectiveType() == TimeRangeTimestamp {
		return TimeRange{
			StartTime:     t.StartTime,
			EndTime:       t.EndTime,
			TimeRangeType: TimeRangeTimestamp,
		}, nil
	}
	start, end, err := t.parseRange(tz...)
	if err != nil {
		return TimeRange{}, err
	}
	return TimeRange{
		StartTime:     start.Unix(),
		EndTime:       end.Unix(),
		TimeRangeType: TimeRangeTimestamp,
	}, nil
}

// ToMs 将当前 TimeRange 归一化为毫秒级时间戳形式，并返回一个新的 TimeRange。
//
// 返回值特征：
//   - TimeRangeType 固定为 TimeRangeTimestampMs
//   - StartTime/EndTime 为毫秒级 Unix 时间戳
//
// 不修改原对象。
//
// 行为说明：
//   - 源类型已是 timestamp_ms 时，原值透传
//   - 源类型是 timestamp 时，按 *1000 升级到毫秒
//   - 源类型是 year_month/date 时，end 取所在月/日 23:59:59.999（毫秒精度）
func (t *TimeRange) ToMs(tz ...any) (TimeRange, error) {
	// 快速路径：源已是毫秒级时间戳，原值透传
	if t.effectiveType() == TimeRangeTimestampMs {
		return TimeRange{
			StartTime:     t.StartTime,
			EndTime:       t.EndTime,
			TimeRangeType: TimeRangeTimestampMs,
		}, nil
	}
	start, end, err := t.parseRange(tz...)
	if err != nil {
		return TimeRange{}, err
	}
	return TimeRange{
		StartTime:     start.UnixMilli(),
		EndTime:       end.UnixMilli(),
		TimeRangeType: TimeRangeTimestampMs,
	}, nil
}

// parseRange 是 Parse/ToSec/ToMs 共用的内部实现。
//
// 抽出独立内部方法是为了让公共方法（Parse/ToSec/ToMs）都直接调用它，
// 避免出现"公共方法相互调用"的链式嵌套，便于单元测试和后续维护。
func (t *TimeRange) parseRange(tz ...any) (start, end time.Time, err error) {
	loc, err := resolveLocation(tz...)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	switch t.effectiveType() {
	case TimeRangeTimestamp:
		start = time.Unix(t.StartTime, 0).In(loc)
		end = time.Unix(t.EndTime, 0).In(loc)

	case TimeRangeTimestampMs:
		start = time.UnixMilli(t.StartTime).In(loc)
		end = time.UnixMilli(t.EndTime).In(loc)

	case TimeRangeYearMonth:
		start, err = parseYearMonth(t.StartTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse StartTime: %w", err)
		}
		end, err = parseYearMonth(t.EndTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse EndTime: %w", err)
		}
		end = endOfMonth(end)

	case TimeRangeDate:
		start, err = parseDate(t.StartTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse StartTime: %w", err)
		}
		end, err = parseDate(t.EndTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse EndTime: %w", err)
		}
		end = endOfDay(end)

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported TimeRangeType: %q", t.TimeRangeType)
	}
	return start, end, nil
}

// effectiveType 返回实际生效的 TimeRangeType，空值默认为 TimeRangeTimestamp。
//
// 设计动机：让 TimeRange 在不显式传 TimeRangeType 时也能正常工作，
// 兼容历史调用方"只传 start_time/end_time 两个秒级时间戳"的习惯。
func (t *TimeRange) effectiveType() TimeRangeType {
	if t.TimeRangeType == "" {
		return TimeRangeTimestamp
	}
	return t.TimeRangeType
}

// resolveLocation 解析可选的时区参数，支持 string（IANA名）和 *time.Location。
// 不传或传 nil/空字符串时返回 time.Local（系统本地时区）。
func resolveLocation(tz ...any) (*time.Location, error) {
	if len(tz) == 0 || tz[0] == nil {
		return time.Local, nil
	}
	switch v := tz[0].(type) {
	case *time.Location:
		if v == nil {
			return time.Local, nil
		}
		return v, nil
	case string:
		if v == "" {
			return time.Local, nil
		}
		return time.LoadLocation(v)
	default:
		return nil, fmt.Errorf("unsupported tz argument type %T, want string or *time.Location", tz[0])
	}
}

// parseYearMonth 将 yyyyMM 格式的 int64（如 202604）解析为该月第一天 00:00:00 的 time.Time
func parseYearMonth(v int64, loc *time.Location) (time.Time, error) {
	s := strconv.FormatInt(v, 10)
	t, err := time.ParseInLocation(defined.YearMonthLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %d as yyyyMM: %w", v, err)
	}
	return t, nil
}

// parseDate 将 yyyyMMdd 格式的 int64（如 20260414）解析为该天 00:00:00 的 time.Time
func parseDate(v int64, loc *time.Location) (time.Time, error) {
	s := strconv.FormatInt(v, 10)
	t, err := time.ParseInLocation(defined.YearMonthDayLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %d as yyyyMMdd: %w", v, err)
	}
	// 反向格式化校验：防止 time.Parse 自动溢出（如 20260230 → 20260302）
	if t.Format(defined.YearMonthDayLayout) != s {
		return time.Time{}, fmt.Errorf("invalid date %d (overflow)", v)
	}
	return t, nil
}

// endOfMonth 返回 t 所在月份最后一天的 23:59:59.999999999。
// 通过"下月第一天减1纳秒"的方式计算，天然处理大小月和闰年。
func endOfMonth(t time.Time) time.Time {
	year, month, _ := t.Date()
	firstOfNextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, t.Location())
	return firstOfNextMonth.Add(-time.Nanosecond)
}

// endOfDay 返回 t 当天的 23:59:59.999999999
func endOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	startOfNextDay := time.Date(year, month, day+1, 0, 0, 0, 0, t.Location())
	return startOfNextDay.Add(-time.Nanosecond)
}
