package community

type IDRequest struct {
	ID int64 `form:"id" json:"id"`
}

type IDsRequest struct {
	IDs []int64 `form:"ids" json:"ids"  binding:"required"`
}

type TimeRange struct {
	StartTime int64 `form:"start_time" json:"start_time" `
	EndTime   int64 `form:"end_time" json:"end_time" `
}

type ListResponse struct {
	List interface{} `json:"list"`
}

type ListPageResponse struct {
	List interface{} `json:"list"`
	Page
}

type Paging struct {
	Page     int  `json:"page" form:"page"`
	PageSize int  `json:"page_size" form:"page_size"`
	Export   bool `json:"export" form:"export"`
}

type Page struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
}

func HandlePage(paging Paging, total int64) Page {
	if paging.Page < 1 {
		paging.Page = 1
	}
	if paging.PageSize < 1 {
		paging.PageSize = 20
	}
	return Page{
		Page:       paging.Page,
		PageSize:   paging.PageSize,
		TotalCount: total,
	}
}
