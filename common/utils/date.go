package utils

import (
	"github.com/Cotary/go-lib/common/defined"
	"time"
)

type Time struct {
	*time.Location
	time.Time
}

type TimeType interface {
	~int64 | time.Time
}

func NewTime[T TimeType](t T, Location ...*time.Location) *Time {
	switch v := any(t).(type) {
	case time.Time:
		if len(Location) > 0 {
			return &Time{Location[0], v}
		}
		return &Time{time.Local, v}
	case int64:
		if len(Location) > 0 {
			return &Time{Location[0], time.UnixMilli(GetMillTime(v))}
		}
		return &Time{time.Local, time.UnixMilli(GetMillTime(v))}
	default:
		return nil
	}
}

func NewUTC() *Time {
	return &Time{time.UTC, time.Now()}
}

func NewLocal(Location ...*time.Location) *Time {
	if len(Location) > 0 {
		return &Time{Location[0], time.Now()}
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
	tm, err := time.ParseInLocation(layout, str, t.Location)
	if err != nil {
		return 0, err
	}
	return tm.Unix(), nil
}

func (t *Time) TimeFormat(layout string) string {
	return t.In(t.Location).Format(layout)
}

func (t *Time) TimeFormatTime(layout string) (int64, error) {
	str := t.TimeFormat(layout)
	return t.StrToTime(str, layout)
}

func (t *Time) GetHourTimes(h int) (int64, int64) {
	now := t.In(t.Location).Add(time.Duration(h) * time.Hour)
	year, month, day := now.Date()
	hour := now.Hour()
	startOfDay := time.Date(year, month, day, hour, 0, 0, 0, t.Location)
	endOfDay := time.Date(year, month, day, hour, 59, 59, 999999, t.Location)
	startTime := startOfDay.Unix()
	endTime := endOfDay.Unix()
	return startTime, endTime
}

func (t *Time) GetDayTimes(d int) (int64, int64) {
	now := t.In(t.Location).AddDate(0, 0, d)
	year, month, day := now.Date()
	startOfDay := time.Date(year, month, day, 0, 0, 0, 0, t.Location)
	endOfDay := time.Date(year, month, day, 23, 59, 59, 999999, t.Location)
	startTime := startOfDay.Unix()
	endTime := endOfDay.Unix()
	return startTime, endTime
}

// GetWeekTimes 周日到周六为一周
func (t *Time) GetWeekTimes(d int) (int64, int64) {
	now := t.In(t.Location).AddDate(0, 0, d*7)
	year, month, day := now.Date()
	weekday := now.Weekday()
	startOfWeek := time.Date(year, month, day-int(weekday), 0, 0, 0, 0, t.Location)
	endOfWeek := time.Date(year, month, day+(6-int(weekday)), 23, 59, 59, 999999, t.Location)
	startTime := startOfWeek.Unix()
	endTime := endOfWeek.Unix()
	return startTime, endTime
}

// GetCWeekTimes 周一到周日为一周
func (t *Time) GetCWeekTimes(d int) (int64, int64) {
	now := t.In(t.Location).AddDate(0, 0, d*7)
	year, month, day := now.Date()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	startOfWeek := time.Date(year, month, day-(weekday-1), 0, 0, 0, 0, t.Location)
	endOfWeek := time.Date(year, month, day+(7-weekday), 23, 59, 59, 999999, t.Location)
	startTime := startOfWeek.Unix()
	endTime := endOfWeek.Unix()
	return startTime, endTime
}

func (t *Time) GetMonthTimes(d int) (int64, int64) {
	now := t.In(t.Location).AddDate(0, d, 1)
	year, month, _ := now.Date()
	startOfDay := time.Date(year, month, 1, 0, 0, 0, 0, t.Location)
	startTime := startOfDay.Unix()
	endOfDay := startOfDay.AddDate(0, 1, 0)
	endTime := endOfDay.Unix() - 1
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
	targetDate, _ := time.ParseInLocation(defined.YearMonthDayLayout, targetDateStr, t.Location)
	inputDate := time.Unix(GetSecTime(timestamp), 0).In(t.Location)
	months := (inputDate.Year()-targetDate.Year())*12 + int(inputDate.Month()) - int(targetDate.Month()) + 1
	return months, inputDate.Day()
}

func (t *Time) GetDayTimesBetween(start, end int64) []int64 {
	start, _ = NewTime(start, t.Location).TimeFormatTime(defined.YearMonthDayLayout)
	end, _ = NewTime(end, t.Location).TimeFormatTime(defined.YearMonthDayLayout)
	var dayTimes []int64
	if start > end {
		return dayTimes
	}
	for i := start; i <= end; i += 86400 {
		dayTime, _ := NewTime(i, t.Location).TimeFormatTime(defined.YearMonthDayLayout)
		dayTimes = append(dayTimes, dayTime)
	}
	return dayTimes
}
