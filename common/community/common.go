package community

type IDRequest struct {
	ID int64 `form:"id" json:"id"`
}

type IDsRequest struct {
	IDs []int64 `form:"ids" json:"ids"`
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
type ListResponseT[T any] struct {
	List []T `json:"list"`
}
type ListPageResponseT[T any] struct {
	List []T `json:"list"`
	Paging
}
