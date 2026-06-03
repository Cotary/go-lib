//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package model

import (
	"context"

	"go-lib/common/community"
	"go-lib/dao/gormDB"
)

// ===== 状态枚举 =====

const (
	OrderStatusPending  = 0 // 待支付
	OrderStatusPaid     = 1 // 已支付
	OrderStatusFailed   = 2 // 支付失败
	OrderStatusRefunded = 3 // 已退款
)

// ===== Model 定义 =====

// Order 订单模型。
// 字段必须有行尾中文注释，枚举字段需列出所有值的含义。
type Order struct {
	gormDB.BaseModel
	UserID      int64  `gorm:"column:user_id"`      // 所属用户ID
	TradeNo     string `gorm:"column:trade_no"`     // 平台唯一订单号
	Amount      string `gorm:"column:amount"`       // 订单金额（字符串避免精度丢失）
	Currency    string `gorm:"column:currency"`     // 币种代码，如 USD、CNY
	Status      uint32 `gorm:"column:status"`       // 订单状态：0-待支付 1-已支付 2-失败 3-已退款
	Description string `gorm:"column:description"`  // 订单描述
	OutTradeNo  string `gorm:"column:out_trade_no"` // 商户外部订单号
	CallbackURL string `gorm:"column:callback_url"` // 支付回调地址
	PaidAt      *int64 `gorm:"column:paid_at"`      // 支付完成时间戳，未支付时为 NULL
}

// NewOrder 构造函数
func NewOrder() *Order { return &Order{} }

// TableName 指定表名
func (Order) TableName() string { return TableNameOrder }

// ===== CRUD 方法：委托 gormDB 泛型函数 =====

// Get 查询单条订单，未找到返回 gorm.ErrRecordNotFound
func (m *Order) Get(ctx context.Context, scopes ...gormDB.Scope) (Order, error) {
	return gormDB.Get[Order](ctx, DBDriver, scopes...)
}

// List 查询订单列表
func (m *Order) List(ctx context.Context, scopes ...gormDB.Scope) ([]Order, error) {
	return gormDB.List[Order](ctx, DBDriver, scopes...)
}

// PageList 分页查询，自动执行 count 并回写 p.Total
func (m *Order) PageList(ctx context.Context, p *community.Paging, scopes ...gormDB.Scope) ([]Order, error) {
	return gormDB.PageList[Order](ctx, DBDriver, p, scopes...)
}

// Insert 插入订单
func (m *Order) Insert(ctx context.Context) error {
	return DBDriver.Insert(ctx, m)
}

// Update 按条件更新指定字段
func (m *Order) Update(ctx context.Context, fields []string, scopes ...gormDB.Scope) error {
	return DBDriver.Update(ctx, m, fields, scopes...)
}

// ===== 调用示例（展示 logic 层如何组合 Scope）=====
//
// 列表查询：
//   list, err := NewOrder().PageList(ctx, &req.Paging,
//       gormDB.WhereIf("user_id", gormDB.OpEq, req.UserID),
//       gormDB.WhereIf("status", gormDB.OpEq, req.Status),
//       gormDB.WhereIf("currency", gormDB.OpEq, req.Currency),
//       gormDB.WhereIf("trade_no", gormDB.OpLike, req.Keyword),
//       gormDB.OrderWhitelist(req.Order, map[string]string{
//           "created_at": "created_at",
//           "amount":     "amount",
//       }),
//   )
//
// 单条查询：
//   order, err := NewOrder().Get(ctx, gormDB.ID(req.ID))
//
// 按状态更新：
//   order := &Order{Status: OrderStatusPaid, PaidAt: &now}
//   err := order.Update(ctx, []string{"status", "paid_at"}, gormDB.ID(orderID))
