package community

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Cotary/go-lib/common/defined"
)

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
	loc, err := resolveLocation(tz...)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	typ := t.effectiveType()
	switch typ {
	case TimeRangeTimestamp:
		start = time.Unix(t.StartTime, 0).In(loc)
		end = time.Unix(t.EndTime, 0).In(loc)

	case TimeRangeTimestampMs:
		start = time.UnixMilli(t.StartTime).In(loc)
		end = time.UnixMilli(t.EndTime).In(loc)

	case TimeRangeYearMonth:
		start, err = parseYearMonth(t.StartTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("StartTime 解析失败: %w", err)
		}
		end, err = parseYearMonth(t.EndTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("EndTime 解析失败: %w", err)
		}
		end = endOfMonth(end)

	case TimeRangeDate:
		start, err = parseDate(t.StartTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("StartTime 解析失败: %w", err)
		}
		end, err = parseDate(t.EndTime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("EndTime 解析失败: %w", err)
		}
		end = endOfDay(end)

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("不支持的 TimeRangeType: %q", t.TimeRangeType)
	}
	return start, end, nil
}

// effectiveType 返回实际生效的 TimeRangeType，空值默认为 TimeRangeTimestamp
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
		return v, nil
	case string:
		if v == "" {
			return time.Local, nil
		}
		return time.LoadLocation(v)
	default:
		return nil, fmt.Errorf("不支持的时区参数类型 %T，需要 string 或 *time.Location", tz[0])
	}
}

// parseYearMonth 将 yyyyMM 格式的 int64（如 202604）解析为该月第一天 00:00:00 的 time.Time
func parseYearMonth(v int64, loc *time.Location) (time.Time, error) {
	s := strconv.FormatInt(v, 10)
	t, err := time.ParseInLocation(defined.YearMonthLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("无法将 %d 解析为 yyyyMM 格式: %w", v, err)
	}
	return t, nil
}

// parseDate 将 yyyyMMdd 格式的 int64（如 20260414）解析为该天 00:00:00 的 time.Time
func parseDate(v int64, loc *time.Location) (time.Time, error) {
	s := strconv.FormatInt(v, 10)
	t, err := time.ParseInLocation(defined.YearMonthDayLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("无法将 %d 解析为 yyyyMMdd 格式: %w", v, err)
	}
	// 反向格式化校验：防止 time.Parse 自动溢出（如 20260230 → 20260302）
	if t.Format(defined.YearMonthDayLayout) != s {
		return time.Time{}, fmt.Errorf("无效的日期 %d（日期溢出）", v)
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
