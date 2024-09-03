package defined

import "github.com/shopspring/decimal"

// echo
const (
	DebugHeader         = "X-Debug"
	AppidHeader         = "X-Appid"
	NonceHeader         = "X-Nonce"
	TimestampHeader     = "X-Timestamp"
	SignHeader          = "X-Sign"
	SignTimestampHeader = "X-SignTimestamp"
	DeviceHeader        = "X-Device"
	TokenHeader         = "X-Token"
	Language            = "X-Language"
	RequestID           = "request_id"
	RequestURI          = "request_uri"
	RequestBody         = "request_body"
	RequestBodyJson     = "request_body_json"
	ServerName          = "server-name"
	ContextType         = "context_type"
	ENV                 = "env"
)

// language
const (
	Chinese   = "zh"
	English   = "en"
	ChineseTw = "zh-tw"
)

// date
const (
	DataYearLayout   = "2006"
	DataMonthLayout  = "2006-01"
	TimeLayout       = "2006-01-02 15:04:05"
	DataLayout       = "2006-01-02"
	DataHourString   = "2006010215"
	DataMinuteString = "200601021504"
	DataSecondString = "20060102150405"

	YearMonthDayHourLayout = "2006-01-02 15:00"
	YearMonthDayLayout     = "20060102"
	YearMonthLayout        = "200601"
	YearLayout             = "2006"
)

// 币种
var PutAccuracy decimal.Decimal = decimal.NewFromInt(10).Pow(decimal.NewFromInt(9))

const (
	PUT  string = "PUT"
	USDT string = "USDT"
)

// 报表日期类型
const (
	Hour  = "hour"
	Day   = "day"
	Month = "month"
)

// env
const (
	Local       = "local"
	Development = "development"
	TEST        = "test"
	Release     = "release"
)

// http
const (
	HTTP200 int64 = 200
)
