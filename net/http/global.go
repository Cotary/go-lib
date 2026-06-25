package http

// DefaultFastHTTPClient 默认的 FastHTTP 客户端实例
var DefaultFastHTTPClient = NewFastHTTPClient()

// DefaultRestyClient 默认的 Resty 客户端实例
var DefaultRestyClient = NewRestyClient()

// FastHTTP 使用默认 FastHTTP 客户端创建请求构建器
//
// 推荐优先使用此方法，FastHTTP 性能更优
//
// 使用示例:
//
//	result := http.FastHTTP().Execute(ctx, "GET", url, nil, nil, nil)
//	user, err := http.Parse[User](result, "data")
func FastHTTP() *RequestBuilder {
	return NewRequestBuilder(DefaultFastHTTPClient)
}

// RestyHTTP 使用默认 Resty 客户端创建请求构建器
//
// 使用示例:
//
//	result := http.RestyHTTP().Execute(ctx, "GET", url, nil, nil, nil)
//	user, err := http.Parse[User](result, "data")
func RestyHTTP() *RequestBuilder {
	return NewRequestBuilder(DefaultRestyClient)
}
