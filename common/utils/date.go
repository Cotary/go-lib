package utils

import (
	"time"
)

// ============================================================================
// 时间戳类型定义
// ============================================================================

// TimestampSec 秒级时间戳类型
type TimestampSec int64

// TimestampMilli 毫秒级时间戳类型
type TimestampMilli int64

// ============================================================================
// 时间戳转换函数
// ============================================================================

// NewTimestampSec 将 int64 转换为秒级时间戳
func NewTimestampSec(value int64) TimestampSec {
	return TimestampSec(value)
}

// NewTimestampMilli 将 int64 转换为毫秒级时间戳
func NewTimestampMilli(value int64) TimestampMilli {
	return TimestampMilli(value)
}

// ToInt64 转换为 int64
func (ts TimestampSec) ToInt64() int64 {
	return int64(ts)
}

// ToInt64 转换为 int64
func (tm TimestampMilli) ToInt64() int64 {
	return int64(tm)
}

// ToSec 转换为秒级时间戳
func (tm TimestampMilli) ToSec() TimestampSec {
	return TimestampSec(int64(tm) / 1000)
}

// ToMilli 转换为毫秒级时间戳
func (ts TimestampSec) ToMilli() TimestampMilli {
	return TimestampMilli(int64(ts) * 1000)
}

// NewTimestampSecFromInt64 将 int64 转换为秒级时间戳（辅助函数）
func NewTimestampSecFromInt64(value int64) TimestampSec {
	return TimestampSec(value)
}

// ============================================================================
// Time 结构体和类型定义
// ============================================================================

// Time 扩展的时间结构体，提供更丰富的时间操作方法
type Time struct {
	time.Time
}

// TimeType 支持的构造类型（泛型约束）
type TimeType interface {
	TimestampSec | TimestampMilli | time.Time
}

// ============================================================================
// Time 对象创建函数
// ============================================================================

// NewTime 创建 Time 对象
// 支持多种时间类型：秒级/毫秒级时间戳、time.Time
// location 可选，默认使用本地时区
func NewTime[T TimeType](timeValue T, location ...*time.Location) Time {
	loc := time.Local
	if len(location) > 0 && location[0] != nil {
		loc = location[0]
	}

	switch v := any(timeValue).(type) {
	case time.Time:
		return Time{v.In(loc)}
	case TimestampSec:
		return Time{time.Unix(int64(v), 0).In(loc)}
	case TimestampMilli:
		return Time{time.UnixMilli(int64(v)).In(loc)}
	default:
		panic("unsupported time type for NewTime")
	}
}

// NewUTC 返回当前 UTC 时间
func NewUTC() *Time {
	return &Time{time.Now().UTC()}
}

// NewLocal 返回当前本地时间（可指定时区）
func NewLocal(location ...*time.Location) *Time {
	loc := time.Local
	if len(location) > 0 && location[0] != nil {
		loc = location[0]
	}
	return &Time{time.Now().In(loc)}
}

// ============================================================================
// 时间戳转换工具函数
// ============================================================================

// ToSecTimestamp 将时间戳转换为秒级时间戳
func ToSecTimestamp[T TimeType](timeValue T) int64 {
	switch v := any(timeValue).(type) {
	case TimestampSec:
		return int64(v)
	case TimestampMilli:
		return int64(v) / 1000
	case time.Time:
		return v.Unix()
	default:
		panic("unsupported time type for ToSecTimestamp")
	}
}

// ToMilliTimestamp 将时间戳转换为毫秒级时间戳
// isEndOfSecond=true 时返回该秒的最后一毫秒（用于区间查询）
func ToMilliTimestamp[T TimeType](timeValue T, isEndOfSecond ...bool) int64 {
	switch v := any(timeValue).(type) {
	case TimestampSec:
		if len(isEndOfSecond) > 0 && isEndOfSecond[0] {
			return int64(v)*1000 + 999
		}
		return int64(v) * 1000
	case TimestampMilli:
		return int64(v)
	case time.Time:
		return v.UnixMilli()
	default:
		panic("unsupported time type for ToMilliTimestamp")
	}
}

// ============================================================================
// 时间解析和格式化函数
// ============================================================================

// ParseTimeFromString 解析字符串为 Time 对象
func ParseTimeFromString(timeString string, layout string, location ...*time.Location) (Time, error) {
	loc := time.Local
	if len(location) > 0 && location[0] != nil {
		loc = location[0]
	}
	timeValue, err := time.ParseInLocation(layout, timeString, loc)
	if err != nil {
		return Time{}, err
	}
	return NewTime(timeValue), nil
}

// ParseTimeFromString 解析字符串为 Time 对象（使用当前时区）
func (t *Time) ParseTimeFromString(timeString string, layout string) (Time, error) {
	return ParseTimeFromString(timeString, layout, t.Location())
}

// Format 格式化时间
func (t *Time) Format(layout string) string {
	return t.Time.Format(layout)
}

// FormatToUnix 格式化后再解析为秒级时间戳（会丢失秒以下精度）
func (t *Time) FormatToUnix(layout string) (Time, error) {
	formattedString := t.Format(layout)
	return t.ParseTimeFromString(formattedString, layout)
}

// ============================================================================
// 时间区间获取函数
// ============================================================================

// GetHourRange 获取当前时间偏移 hourOffset 小时所在的整点区间
func (t *Time) GetHourRange(hourOffset int) (Time, Time) {
	adjustedTime := t.Add(time.Duration(hourOffset) * time.Hour)
	year, month, day := adjustedTime.Date()
	hour := adjustedTime.Hour()
	startTime := time.Date(year, month, day, hour, 0, 0, 0, t.Location())
	endTime := startTime.Add(time.Hour).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// GetDayRange 获取当前时间偏移 dayOffset 天所在的日期区间
func (t *Time) GetDayRange(dayOffset int) (Time, Time) {
	adjustedTime := t.AddDate(0, 0, dayOffset)
	year, month, day := adjustedTime.Date()
	startTime := time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	endTime := startTime.AddDate(0, 0, 1).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// GetWeekRangeSunday 获取周区间（周日到周六为一周）
func (t *Time) GetWeekRangeSunday(weekOffset int) (Time, Time) {
	adjustedTime := t.AddDate(0, 0, weekOffset*7)
	year, month, day := adjustedTime.Date()
	weekday := adjustedTime.Weekday()
	startTime := time.Date(year, month, day-int(weekday), 0, 0, 0, 0, t.Location())
	endTime := startTime.AddDate(0, 0, 7).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// GetWeekRangeMonday 获取周区间（周一到周日为一周）
func (t *Time) GetWeekRangeMonday(weekOffset int) (Time, Time) {
	adjustedTime := t.AddDate(0, 0, weekOffset*7)
	year, month, day := adjustedTime.Date()
	weekday := int(adjustedTime.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	startTime := time.Date(year, month, day-(weekday-1), 0, 0, 0, 0, t.Location())
	endTime := startTime.AddDate(0, 0, 7).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// GetMonthRange 获取当前时间偏移 monthOffset 个月所在的月份区间
func (t *Time) GetMonthRange(monthOffset int) (Time, Time) {
	adjustedTime := t.AddDate(0, monthOffset, 0)
	year, month, _ := adjustedTime.Date()
	startTime := time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
	endTime := startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// GetYearRange 获取当前时间偏移 yearOffset 年所在的年份区间
func (t *Time) GetYearRange(yearOffset int) (Time, Time) {
	adjustedTime := t.AddDate(yearOffset, 0, 0)
	year, _, _ := adjustedTime.Date()
	startTime := time.Date(year, 1, 1, 0, 0, 0, 0, t.Location())
	endTime := startTime.AddDate(1, 0, 0).Add(-time.Nanosecond)
	return NewTime(startTime), NewTime(endTime)
}

// ============================================================================
// 时间列表生成函数
// ============================================================================

// GetDayListBetween 获取两个时间戳之间的所有日期（按天对齐）
// 性能优化：预分配切片容量，避免动态扩容
func GetDayListBetween[T TimeType](startTime, endTime T, location ...*time.Location) []Time {
	// 确定时区
	loc := time.Local
	if len(location) > 0 && location[0] != nil {
		loc = location[0]
	}

	startTimeObj := NewTime(startTime, loc)
	endTimeObj := NewTime(endTime, loc)

	// 按天对齐：截断到当天开始时间
	startSec := ToSecTimestamp(startTimeObj.Time.Truncate(24 * time.Hour))
	endSec := ToSecTimestamp(endTimeObj.Time.Truncate(24 * time.Hour))

	if startSec > endSec {
		return []Time{}
	}

	// 预分配切片容量以提高性能
	dayCount := int((endSec-startSec)/86400) + 1
	dayList := make([]Time, 0, dayCount)

	for timestamp := startSec; timestamp <= endSec; timestamp += 86400 {
		dayTime := NewTime(NewTimestampSecFromInt64(timestamp), loc)
		dayList = append(dayList, dayTime)
	}
	return dayList
}

// ============================================================================
// 月日差计算相关函数
// ============================================================================

// MonthDayDifference 月日差结果结构体
type MonthDayDifference struct {
	MonthDifference int // 月差
	DayDifference   int // 日差
}

// CalculateMonthDayDifferences 计算时间区间内每一天与目标日期的月差和日
// 性能优化：预分配切片容量，避免重复解析目标日期
func CalculateMonthDayDifferences[T TimeType](startTime, endTime, targetDate T, location ...*time.Location) ([]MonthDayDifference, error) {
	// 确定时区
	loc := time.Local
	if len(location) > 0 && location[0] != nil {
		loc = location[0]
	}

	// 将目标日期转换为 Time 对象（只解析一次）
	targetTimeObj := NewTime(targetDate, loc)

	startSec := ToSecTimestamp(startTime)
	endSec := ToSecTimestamp(endTime)

	if startSec >= endSec {
		return []MonthDayDifference{}, nil
	}

	// 预分配切片容量
	dayCount := int((endSec-startSec)/86400) + 1
	differenceList := make([]MonthDayDifference, 0, dayCount)

	for timestamp := startSec; timestamp < endSec; timestamp += 86400 {
		monthDiff, dayDiff := calculateMonthDayDifferenceOptimized(targetTimeObj.Time, time.Unix(timestamp, 0).In(loc))
		differenceList = append(differenceList, MonthDayDifference{MonthDifference: monthDiff, DayDifference: dayDiff})
	}
	return differenceList, nil
}

// calculateMonthDayDifferenceOptimized 优化的月日差计算函数
// 算法：年差*12 + 月差，并根据天数差异调整月份
func calculateMonthDayDifferenceOptimized(targetDate, inputDate time.Time) (int, int) {
	// 计算月差：年差*12 + 月差
	monthDifference := (inputDate.Year()-targetDate.Year())*12 + int(inputDate.Month()) - int(targetDate.Month())

	// 如果输入日期的天数小于目标日期的天数，月份减1
	if inputDate.Day() < targetDate.Day() {
		monthDifference--
	}

	return monthDifference + 1, inputDate.Day()
}
