package utils

import (
	"github.com/Cotary/go-lib/common/defined"
	"time"
)

type Time struct {
	loc     *time.Location
	nowTime time.Time
}

func NewUTC() *Time {
	return &Time{time.UTC, time.Now()}
}

func NewLocal() *Time {
	return &Time{time.Local, time.Now()}
}

func GetSecTime(t int64) int64 {
	if t > 1e12 {
		return t / 1000
	}
	return t
}

func GetMillTime(t int64, end ...bool) int64 {
	if t == 0 || t > 1e12 {
		return t
	}
	if len(end) > 0 && end[0] {
		return t*1000 + 999
	}
	return t * 1000
}

func (t *Time) StrToTime(str string, layout string) (int64, error) {
	tm, err := time.ParseInLocation(layout, str, t.loc)
	if err != nil {
		return 0, err
	}
	return tm.Unix(), nil
}

func (t *Time) TimeFormat(timestamp int64, layout string) string {
	return time.Unix(GetSecTime(timestamp), 0).In(t.loc).Format(layout)
}

func (t *Time) TimeFormatTime(time int64, layout string) (int64, error) {
	str := t.TimeFormat(time, layout)
	return t.StrToTime(str, layout)
}

func (t *Time) GetDayTimes(d int) (int64, int64) {
	now := time.Now().In(t.loc).AddDate(0, 0, d)
	year, month, day := now.Date()
	startOfDay := time.Date(year, month, day, 0, 0, 0, 0, t.loc)
	endOfDay := time.Date(year, month, day, 23, 59, 59, 0, t.loc)
	startTime := startOfDay.Unix()
	endTime := endOfDay.Unix()
	return startTime, endTime
}

func (t *Time) GetMonthTimes(d int) (int64, int64) {
	now := time.Now().In(t.loc).AddDate(0, d, 1)
	year, month, _ := now.Date()
	startOfDay := time.Date(year, month, 1, 0, 0, 0, 0, t.loc)
	startTime := startOfDay.Unix()
	if month == 12 {
		year++
		month = 1
	} else {
		month++
	}
	endOfDay := startOfDay.AddDate(0, 1, 0)
	endTime := endOfDay.Unix() - 1
	return startTime, endTime
}

func (t *Time) GetHourUnixMilli(h int) (int64, int64) {
	now := time.Now().In(t.loc).Add(time.Duration(h) * time.Hour)
	year, month, day := now.Date()
	hour := now.Hour()
	startOfDay := time.Date(year, month, day, hour, 0, 0, 0, t.loc)
	endOfDay := time.Date(year, month, day, hour, 59, 59, 999999, t.loc)
	startTime := startOfDay.UnixMilli()
	endTime := endOfDay.UnixMilli()
	return startTime, endTime
}

type CalculateMonthAndDayItem struct {
	Month int
	Day   int
}

func (t *Time) GetDayTimesBetween(start, end int64) []int64 {
	start, _ = t.TimeFormatTime(start, defined.YearMonthDayLayout)
	end, _ = t.TimeFormatTime(end, defined.YearMonthDayLayout)
	var dayTimes []int64
	if start > end {
		return dayTimes
	}
	for i := start; i <= end; i += 86400 {
		dayTime, _ := t.TimeFormatTime(i, defined.YearMonthDayLayout)
		dayTimes = append(dayTimes, dayTime)
	}
	return dayTimes
}

func (t *Time) GetDaysBetween(start, end int64) []int64 {
	start, _ = t.TimeFormatTime(start, defined.YearMonthDayLayout)
	end, _ = t.TimeFormatTime(end, defined.YearMonthDayLayout)
	var day []int64
	if start > end {
		return day
	}
	for i := start; i <= end; i += 86400 {
		dayTime := AnyToInt(t.TimeFormat(i, defined.YearMonthDayLayout))
		day = append(day, dayTime)
	}
	return day
}

func ShiftTimeRange(start, end int64) (int64, int64) {
	diff := end - start
	interval := time.Duration(diff) * time.Second
	startTime := time.Unix(start, 0)
	endTime := time.Unix(end, 0)
	beforeStartTime := startTime.Add(-interval)
	beforeEndTime := endTime.Add(-interval)
	return beforeStartTime.Unix(), beforeEndTime.Unix()
}

// 获取毫秒级时间戳的当日开始时间戳和结束时间戳
func (t *Time) GetDataStartAndEnd(tInt int64) (int64, int64) {
	timestamp := time.Unix(tInt, 0).In(time.UTC)

	y, m, d := timestamp.Date()
	startOfDay := time.Date(y, m, d, 0, 0, 0, 0, t.loc)
	endOfDay := time.Date(y, m, d, 23, 59, 59, 999999, t.loc)
	startUnix := startOfDay.UTC().Unix()
	endUnix := endOfDay.UTC().Unix()
	return startUnix, endUnix
}
