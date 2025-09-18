package utils

import (
	"github.com/Cotary/go-lib/common/defined"
	"time"
)

// Time 在 time.Time 基础上扩展了秒/毫秒转换与区间计算等方法
type Time struct {
	time.Time
}

// TimeType 支持的构造类型
type TimeType interface {
	~int64 | time.Time
}

// NewTime 创建 Time 对象
// t 可以是秒级或毫秒级时间戳，也可以是 time.Time
// loc 可选，默认使用本地时区
func NewTime[T TimeType](t T, loc ...*time.Location) *Time {
	location := time.Local
	if len(loc) > 0 && loc[0] != nil {
		location = loc[0]
	}

	switch v := any(t).(type) {
	case time.Time:
		return &Time{v.In(location)}
	case int64:
		return &Time{time.UnixMilli(GetMillTime(v)).In(location)}
	default:
		panic("unsupported time type for NewTime")
	}
}

// NewUTC 返回当前 UTC 时间
func NewUTC() *Time {
	return &Time{time.Now().UTC()}
}

// NewLocal 返回当前本地时间（可指定时区）
func NewLocal(loc ...*time.Location) *Time {
	location := time.Local
	if len(loc) > 0 && loc[0] != nil {
		location = loc[0]
	}
	return &Time{time.Now().In(location)}
}

// GetSecTime 将毫秒级时间戳转换为秒级
func GetSecTime(t int64) int64 {
	if t > 1e12 {
		return t / 1000
	}
	return t
}

// GetMillTime 将秒级时间戳转换为毫秒级
// end=true 时返回该秒的最后一毫秒
func GetMillTime(t int64, end ...bool) int64 {
	if t == 0 || t > 1e12 {
		return t
	}
	if len(end) > 0 && end[0] {
		return t*1000 + 999
	}
	return t * 1000
}

// StrToTime 将字符串解析为秒级时间戳
func (t *Time) StrToTime(str string, layout string) (int64, error) {
	tm, err := time.ParseInLocation(layout, str, t.Location())
	if err != nil {
		return 0, err
	}
	return tm.Unix(), nil
}

// FormatString 格式化为字符串
func (t *Time) FormatString(layout string) string {
	return t.Format(layout)
}

// FormatToUnix 格式化后再解析为秒级时间戳（会丢失秒以下精度）
func (t *Time) FormatToUnix(layout string) (int64, error) {
	str := t.FormatString(layout)
	return t.StrToTime(str, layout)
}

// GetHourRange 获取当前时间偏移 h 小时所在的整点区间（秒级时间戳）
func (t *Time) GetHourRange(h int) (int64, int64) {
	now := t.Add(time.Duration(h) * time.Hour)
	year, month, day := now.Date()
	hour := now.Hour()
	start := time.Date(year, month, day, hour, 0, 0, 0, t.Location())
	end := start.Add(time.Hour).Add(-time.Nanosecond)
	return start.Unix(), end.Unix()
}

// GetDayRange 获取当前时间偏移 d 天所在的日期区间（秒级时间戳）
func (t *Time) GetDayRange(d int) (int64, int64) {
	now := t.AddDate(0, 0, d)
	year, month, day := now.Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 0, 1).Add(-time.Nanosecond)
	return start.Unix(), end.Unix()
}

// GetWeekRangeSunday 周日到周六为一周
func (t *Time) GetWeekRangeSunday(weekOffset int) (int64, int64) {
	now := t.AddDate(0, 0, weekOffset*7)
	year, month, day := now.Date()
	weekday := now.Weekday()
	start := time.Date(year, month, day-int(weekday), 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 0, 7).Add(-time.Nanosecond)
	return start.Unix(), end.Unix()
}

// GetWeekRangeMonday 周一到周日为一周
func (t *Time) GetWeekRangeMonday(weekOffset int) (int64, int64) {
	now := t.AddDate(0, 0, weekOffset*7)
	year, month, day := now.Date()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := time.Date(year, month, day-(weekday-1), 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 0, 7).Add(-time.Nanosecond)
	return start.Unix(), end.Unix()
}

// GetMonthRange 获取当前时间偏移 monthOffset 个月所在的月份区间（秒级时间戳）
func (t *Time) GetMonthRange(monthOffset int) (int64, int64) {
	now := t.AddDate(0, monthOffset, 0)
	year, month, _ := now.Date()
	start := time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return start.Unix(), end.Unix()
}

// CalculateMonthAndDayItem 月日差结果
type CalculateMonthAndDayItem struct {
	Month int
	Day   int
}

// CalculateMonthAndDayList 计算时间区间内每一天与目标日期的月差和日
func (t *Time) CalculateMonthAndDayList(start, end int64, targetDate string) ([]CalculateMonthAndDayItem, error) {
	var list []CalculateMonthAndDayItem
	for ts := GetSecTime(start); ts < GetSecTime(end); ts += 86400 {
		month, day, err := t.CalculateMonthAndDay(ts, targetDate)
		if err != nil {
			return nil, err
		}
		list = append(list, CalculateMonthAndDayItem{Month: month, Day: day})
	}
	return list, nil
}

// CalculateMonthAndDay 计算与目标日期的月差和日
func (t *Time) CalculateMonthAndDay(timestamp int64, targetDateStr string) (int, int, error) {
	targetDate, err := time.ParseInLocation(defined.YearMonthDayLayout, targetDateStr, t.Location())
	if err != nil {
		return 0, 0, err
	}
	inputDate := time.Unix(GetSecTime(timestamp), 0).In(t.Location())
	months := (inputDate.Year()-targetDate.Year())*12 + int(inputDate.Month()) - int(targetDate.Month()) + 1
	return months, inputDate.Day(), nil
}

// GetDayListBetween 获取两个时间戳之间的所有日期（秒级时间戳，按天对齐）
func (t *Time) GetDayListBetween(start, end int64) []int64 {
	start, _ = NewTime(start, t.Location()).FormatToUnix(defined.YearMonthDayLayout)
	end, _ = NewTime(end, t.Location()).FormatToUnix(defined.YearMonthDayLayout)
	var days []int64
	if start > end {
		return days
	}
	for ts := start; ts <= end; ts += 86400 {
		day, _ := NewTime(ts, t.Location()).FormatToUnix(defined.YearMonthDayLayout)
		days = append(days, day)
	}
	return days
}
