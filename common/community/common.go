package community

type IDRequest struct {
	ID int64 `form:"id" json:"id"`
}

type IDsRequest struct {
	IDs []int64 `form:"ids" json:"ids"`
}

type TimeRange struct {
	StartTime int64 `form:"start_time" json:"start_time" `
	EndTime   int64 `form:"end_time" json:"end_time" `
}

type Between struct {
	Start int64 `form:"start" json:"start" `
	End   int64 `form:"end" json:"end" `
}
type ListResponse struct {
	List interface{} `json:"list"`
}

type ListPageResponse struct {
	List interface{} `json:"list"`
	Paging
}

type Order struct {
	OrderType  string `form:"order_type" json:"order_type"`
	OrderField string `form:"order_field" json:"order_field"`
}

type Paging struct {
	Page       int   `json:"page" form:"page"`
	PageSize   int   `json:"page_size" form:"page_size"`
	All        bool  `json:"all" form:"all"`
	TotalCount int64 `json:"total_count"`
}
