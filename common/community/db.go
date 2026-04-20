package community

type Order struct {
	OrderType  string `form:"order_type" json:"order_type"`
	OrderField string `form:"order_field" json:"order_field"`
}

type Paging struct {
	Page     int   `json:"page" form:"page"`
	PageSize int   `json:"page_size" form:"page_size"`
	All      bool  `json:"all" form:"all"`
	Total    int64 `json:"total"`
}

// PageOf 用列表数据和已回写 Total 的 Paging 构造分页响应。
//
// 配合 gormDB.Paginate 的副作用回写使用，让响应组装一行搞定：
//
//	list, err := gormDB.PageList[User](ctx, g, &req.Paging, scopes...)
//	return community.PageOf(list, req.Paging), err
func PageOf(list any, p Paging) *ListPageResponse {
	return &ListPageResponse{List: list, Paging: p}
}

// PageOfT 是 PageOf 的泛型版本，适合返回类型化的列表，避免 interface{} 反序列化时丢失字段。
func PageOfT[T any](list []T, p Paging) *ListPageResponseT[T] {
	return &ListPageResponseT[T]{List: list, Paging: p}
}
