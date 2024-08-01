package utils

import (
	"github.com/Cotary/go-lib/common/defined"
	"time"
)

type Time struct {
	loc     *time.Location
	nowTime time.Time
}

func NewTime(t time.Time, loc ...*time.Location) *Time {
	if len(loc) > 0 {
		return &Time{loc[0], t}
	}
	return &Time{time.Local, t}
}

func NewUTC() *Time {
	return &Time{time.UTC, time.Now()}
}

func NewLocal(loc ...*time.Location) *Time {
	if len(loc) > 0 {
		return &Time{loc[0], time.Now()}
	}
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

func (t *Time) CalculateMonthAndDayList(start int64, end int64, targetDate string) (list []CalculateMonthAndDayItem) {
	for i := GetSecTime(start); i < GetSecTime(end); i += 86400 {
		month, day := t.CalculateMonthAndDay(i, targetDate)
		list = append(list, CalculateMonthAndDayItem{
			Month: month,
			Day:   day,
		})
	}
	return list
}

func (t *Time) CalculateMonthAndDay(timestamp int64, targetDateStr string) (int, int) {
	targetDate, _ := time.ParseInLocation(defined.YearMonthDayLayout, targetDateStr, t.loc)
	inputDate := time.Unix(GetSecTime(timestamp), 0).In(t.loc)
	months := (inputDate.Year()-targetDate.Year())*12 + int(inputDate.Month()) - int(targetDate.Month()) + 1
	return months, inputDate.Day()
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
