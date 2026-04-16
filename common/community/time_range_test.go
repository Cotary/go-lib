package community

import (
	"testing"
	"time"
)

// ========== Parse 基础测试 ==========

func TestParse_Timestamp(t *testing.T) {
	tr := TimeRange{
		StartTime:     1713100800,
		EndTime:       1713187200,
		TimeRangeType: TimeRangeTimestamp,
	}
	start, end, err := tr.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Unix() != 1713100800 {
		t.Errorf("start = %d, want 1713100800", start.Unix())
	}
	if end.Unix() != 1713187200 {
		t.Errorf("end = %d, want 1713187200", end.Unix())
	}
}

func TestParse_EmptyTypeDefaultsToTimestamp(t *testing.T) {
	tr := TimeRange{StartTime: 1713100800, EndTime: 1713187200}
	start, end, err := tr.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.Unix() != 1713100800 || end.Unix() != 1713187200 {
		t.Error("空 TimeRangeType 应按 timestamp 处理")
	}
}

func TestParse_TimestampMs(t *testing.T) {
	tr := TimeRange{
		StartTime:     1713100800000,
		EndTime:       1713100800123,
		TimeRangeType: TimeRangeTimestampMs,
	}
	start, end, err := tr.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start.UnixMilli() != 1713100800000 {
		t.Errorf("start = %d, want 1713100800000", start.UnixMilli())
	}
	if end.UnixMilli() != 1713100800123 {
		t.Errorf("end = %d, want 1713100800123", end.UnixMilli())
	}
}

func TestParse_YearMonth(t *testing.T) {
	tr := TimeRange{
		StartTime:     202604,
		EndTime:       202604,
		TimeRangeType: TimeRangeYearMonth,
	}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 30, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

func TestParse_YearMonth_CrossMonths(t *testing.T) {
	tr := TimeRange{
		StartTime:     202601,
		EndTime:       202603,
		TimeRangeType: TimeRangeYearMonth,
	}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 3, 31, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

func TestParse_Date(t *testing.T) {
	tr := TimeRange{
		StartTime:     20260414,
		EndTime:       20260414,
		TimeRangeType: TimeRangeDate,
	}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 14, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

func TestParse_Date_CrossDays(t *testing.T) {
	tr := TimeRange{
		StartTime:     20260414,
		EndTime:       20260416,
		TimeRangeType: TimeRangeDate,
	}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 16, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

// ========== 默认时区 = time.Local ==========

func TestParse_DefaultUsesLocal(t *testing.T) {
	tr := TimeRange{
		StartTime:     20260414,
		EndTime:       20260414,
		TimeRangeType: TimeRangeDate,
	}
	start, _, err := tr.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 4, 14, 0, 0, 0, 0, time.Local)
	if !start.Equal(wantStart) {
		t.Errorf("默认应使用 time.Local, start = %v, want %v", start, wantStart)
	}
	if start.Location().String() != time.Local.String() {
		t.Errorf("Location = %s, want %s", start.Location(), time.Local)
	}
}

// ========== 传 *time.Location 变量 ==========

func TestParse_PassLocationVariable(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	tr := TimeRange{
		StartTime:     20260414,
		EndTime:       20260414,
		TimeRangeType: TimeRangeDate,
	}
	start, end, err := tr.Parse(loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 4, 14, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 4, 14, 23, 59, 59, 999999999, loc)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

// ========== 传 string 时区名 ==========

func TestParse_YearMonth_Shanghai(t *testing.T) {
	tr := TimeRange{
		StartTime:     202604,
		EndTime:       202604,
		TimeRangeType: TimeRangeYearMonth,
	}
	start, end, err := tr.Parse("Asia/Shanghai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	wantStart := time.Date(2026, 4, 1, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 4, 30, 23, 59, 59, 999999999, loc)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

func TestParse_Date_NewYork(t *testing.T) {
	tr := TimeRange{
		StartTime:     20260414,
		EndTime:       20260414,
		TimeRangeType: TimeRangeDate,
	}
	start, _, err := tr.Parse("America/New_York")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	wantStart := time.Date(2026, 4, 14, 0, 0, 0, 0, loc)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
}

func TestParse_Timestamp_TimezoneOnlyAffectsLocation(t *testing.T) {
	tr := TimeRange{
		StartTime:     1713100800,
		EndTime:       1713187200,
		TimeRangeType: TimeRangeTimestamp,
	}
	startUTC, _, _ := tr.Parse(time.UTC)
	startSH, _, _ := tr.Parse("Asia/Shanghai")

	if !startUTC.Equal(startSH) {
		t.Error("同一时间戳在不同时区应为同一绝对时刻")
	}
	if startUTC.Location().String() == startSH.Location().String() {
		t.Error("Location 应不同")
	}
}

// ========== 时区差异验证 ==========

func TestParse_SameYearMonth_DifferentTZ_DifferentUnix(t *testing.T) {
	tr := TimeRange{
		StartTime:     202604,
		EndTime:       202604,
		TimeRangeType: TimeRangeYearMonth,
	}
	startSH, _, _ := tr.Parse("Asia/Shanghai")
	startUTC, _, _ := tr.Parse(time.UTC)

	// 上海 00:00 比 UTC 00:00 早8小时，所以 Unix 时间戳更小
	diff := startUTC.Unix() - startSH.Unix()
	if diff != 8*3600 {
		t.Errorf("上海与UTC差 = %d秒, want %d", diff, 8*3600)
	}
}

// ========== 边界测试 ==========

func TestParse_LeapYear_February(t *testing.T) {
	tr := TimeRange{StartTime: 202402, EndTime: 202402, TimeRangeType: TimeRangeYearMonth}
	_, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end.Day() != 29 {
		t.Errorf("2024闰年2月末 = %d, want 29", end.Day())
	}
}

func TestParse_NonLeapYear_February(t *testing.T) {
	tr := TimeRange{StartTime: 202502, EndTime: 202502, TimeRangeType: TimeRangeYearMonth}
	_, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end.Day() != 28 {
		t.Errorf("2025非闰年2月末 = %d, want 28", end.Day())
	}
}

func TestParse_YearEnd_December(t *testing.T) {
	tr := TimeRange{StartTime: 202612, EndTime: 202612, TimeRangeType: TimeRangeYearMonth}
	_, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantEnd := time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC)
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

// ========== 闭区间：start == end 不应返回空区间 ==========

func TestParse_ClosedInterval_SameDate(t *testing.T) {
	tr := TimeRange{StartTime: 20260414, EndTime: 20260414, TimeRangeType: TimeRangeDate}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !end.After(start) {
		t.Error("同一天闭区间，end 应晚于 start")
	}
	diff := end.Sub(start)
	if diff < 23*time.Hour || diff > 24*time.Hour {
		t.Errorf("同一天区间时长 = %v, 应接近24小时", diff)
	}
}

func TestParse_ClosedInterval_SameMonth(t *testing.T) {
	tr := TimeRange{StartTime: 202604, EndTime: 202604, TimeRangeType: TimeRangeYearMonth}
	start, end, err := tr.Parse(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !end.After(start) {
		t.Error("同月闭区间，end 应晚于 start")
	}
}

// ========== 错误处理 ==========

func TestParse_InvalidType(t *testing.T) {
	tr := TimeRange{StartTime: 1, EndTime: 2, TimeRangeType: "invalid"}
	_, _, err := tr.Parse()
	if err == nil {
		t.Error("非法 TimeRangeType 应返回错误")
	}
}

func TestParse_InvalidTimezone_String(t *testing.T) {
	tr := TimeRange{StartTime: 1713100800, EndTime: 1713187200, TimeRangeType: TimeRangeTimestamp}
	_, _, err := tr.Parse("Invalid/Zone")
	if err == nil {
		t.Error("非法时区字符串应返回错误")
	}
}

func TestParse_InvalidTimezone_WrongType(t *testing.T) {
	tr := TimeRange{StartTime: 1713100800, EndTime: 1713187200, TimeRangeType: TimeRangeTimestamp}
	_, _, err := tr.Parse(12345)
	if err == nil {
		t.Error("传入非 string/非 *time.Location 应返回错误")
	}
}

func TestParse_InvalidDate_Overflow(t *testing.T) {
	tr := TimeRange{StartTime: 20260101, EndTime: 20260230, TimeRangeType: TimeRangeDate}
	_, _, err := tr.Parse()
	if err == nil {
		t.Error("20260230 应被检测为无效日期")
	}
}
