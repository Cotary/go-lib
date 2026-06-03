//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行
//
// community 目录存放请求/响应 DTO（Data Transfer Object）。
// 关键规则：
//   - GET 请求的 DTO 字段用 form 标签（参数通过 ?key=value 传递）
//   - POST 请求的 DTO 字段用 json 标签（参数通过 JSON body 传递）
//   - 需要校验的字段加 binding 标签

package community

import (
	"go-lib/common/community"
)

// ===== GET 请求 DTO =====
// GET 请求通过 URL 查询参数传值，必须使用 form 标签
// 示例请求：GET /api/order/detail?id=123

// OrderDetailRequest 订单详情请求
type OrderDetailRequest struct {
	ID int64 `form:"id" binding:"required"` // 订单ID
}

// OrderListRequest 订单列表请求（GET + 分页）
// 示例请求：GET /api/order/list?page=1&page_size=10&status=1&keyword=test
type OrderListRequest struct {
	community.Paging         // 嵌入分页（page/page_size/all 字段，已有 form 标签）
	community.Order          // 嵌入排序（order_field/order_type 字段）
	Status           *uint32 `form:"status"`  // 订单状态，指针类型配合 WhereIf 零值跳过
	UserID           int64   `form:"user_id"` // 用户ID
	Keyword          string  `form:"keyword"` // 模糊搜索关键词
}

// ===== POST 请求 DTO =====
// POST 请求通过 JSON body 传值，使用 json 标签
// binding 标签用于参数校验：required=必填，max=最大长度

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Amount      string `json:"amount" binding:"required"`     // 金额
	Currency    string `json:"currency" binding:"required"`   // 币种
	Description string `json:"description"`                   // 描述（可选）
	OutTradeNo  string `json:"out_trade_no" binding:"max=32"` // 外部订单号（可选，最长32位）
	CallbackURL string `json:"callback_url"`                  // 回调地址（可选）
}

// ===== 响应 DTO =====

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	TradeNo string `json:"trade_no"` // 平台订单号
	payURL  string `json:"pay_url"`  // 支付链接
}
