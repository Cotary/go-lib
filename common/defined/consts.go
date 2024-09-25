package defined

// echo
const (
	DebugHeader         = "X-Debug"
	AppidHeader         = "X-Appid"
	NonceHeader         = "X-Nonce"
	TimestampHeader     = "X-Timestamp"
	SignHeader          = "X-Sign"
	SignTimestampHeader = "X-SignTimestamp"
	SignTypeHeader      = "X-SignType"
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
	YearMonthDayHourLayout = "2006-01-02 15:00"
	YearMonthDayLayout     = "20060102"
	YearMonthLayout        = "200601"
	YearLayout             = "2006"
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
