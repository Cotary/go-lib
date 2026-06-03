//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package logic

import (
	"myproject/common/defined"
	"myproject/model"
	"myproject/provider"
	apiDto "myproject/server/api/community"

	"go-lib/common/community"
	"go-lib/dao/gormDB"
	e "go-lib/err"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrderList 订单列表查询（GET + 分页 + Scope 组合）。
//
// handler.CD 自动将 URL 查询参数绑定到 req（通过 form 标签），
// 函数签名：func(c *gin.Context, req T) (R, error)
func OrderList(c *gin.Context, req apiDto.OrderListRequest) (*community.ListPageResponse, error) {
	ctx := c.Request.Context()

	list, err := model.NewOrder().PageList(ctx, &req.Paging,
		// WhereIf：零值/nil 时自动跳过，非零时生效
		gormDB.WhereIf("user_id", gormDB.OpEq, req.UserID),
		gormDB.WhereIf("status", gormDB.OpEq, req.Status),
		gormDB.WhereIf("trade_no", gormDB.OpLike, req.Keyword),
		// OrderWhitelist：白名单排序，防 SQL 注入
		gormDB.OrderWhitelist(req.Order, map[string]string{
			"created_at": "created_at",
			"amount":     "amount",
		}),
	)
	if err != nil {
		return nil, e.Err(err)
	}

	// PageOf 用 Paginate 已回写 Total 的 Paging 构造分页响应
	return community.PageOf(list, req.Paging), nil
}

// OrderDetail 订单详情查询（GET + 查询参数）。
// 请求示例：GET /api/order/detail?id=123
func OrderDetail(c *gin.Context, req apiDto.OrderDetailRequest) (*model.Order, error) {
	ctx := c.Request.Context()

	order, err := model.NewOrder().Get(ctx, gormDB.ID(req.ID))
	if err != nil {
		// gormDB.DbErr 将 RecordNotFound 转为 nil
		if gormDB.DbErr(err) == nil {
			return nil, e.NewHttpErr(defined.OrderNotFound, nil)
		}
		return nil, e.Err(err)
	}
	return &order, nil
}

// CreateOrder 创建订单（POST + JSON body）。
// handler.CD 自动将 JSON body 绑定到 req（通过 json 标签）。
func CreateOrder(c *gin.Context, req apiDto.CreateOrderRequest) (*apiDto.CreateOrderResponse, error) {
	ctx := c.Request.Context()

	tradeNo := uuid.NewString()
	order := &model.Order{
		TradeNo:     tradeNo,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		OutTradeNo:  req.OutTradeNo,
		CallbackURL: req.CallbackURL,
		Status:      model.OrderStatusPending,
	}

	if err := order.Insert(ctx); err != nil {
		return nil, e.Err(err)
	}

	// 调用外部支付网关创建支付链接
	payURL, err := provider.Createpay(ctx, tradeNo, req.Amount, req.Currency)
	if err != nil {
		// 使用业务错误码返回，底层 err 记入日志但不暴露给客户端
		return nil, e.NewHttpErr(defined.payGatewayErr, err)
	}

	return &apiDto.CreateOrderResponse{
		TradeNo: tradeNo,
		payURL:  payURL,
	}, nil
}
