//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package defined

import (
	e "go-lib/err"
)

// 业务错误码从 20001 开始，与 go-lib 内置错误码（10001+）区分。
// go-lib 内置：e.ParamErr(10003)、e.DataNotExist(10004)、e.AuthErr(10006) 等可直接使用。
const (
	OrderNotFoundCode = iota + 20001
	OrderAlreadyPaidCode
	InsufficientBalanceCode
	AccountDisabledCode
	payGatewayErrCode
)

// 业务错误实例，Level 决定告警级别：
//   - e.InfoLevel：业务正常分支（如数据不存在），仅记日志
//   - e.ErrorLevel：需关注的错误，触发告警
//   - e.PanicLevel：严重错误，立即告警
var (
	OrderNotFound       = e.NewCodeErr(OrderNotFoundCode, "Order not found", e.InfoLevel)
	OrderAlreadyPaid    = e.NewCodeErr(OrderAlreadyPaidCode, "Order already paid", e.InfoLevel)
	InsufficientBalance = e.NewCodeErr(InsufficientBalanceCode, "Insufficient balance", e.InfoLevel)
	AccountDisabled     = e.NewCodeErr(AccountDisabledCode, "Account disabled", e.InfoLevel)
	payGatewayErr       = e.NewCodeErr(payGatewayErrCode, "pay gateway error", e.ErrorLevel)
)

// ===== 使用方式 =====
//
// HTTP 层返回业务错误（客户端可见）：
//   return nil, e.NewHttpErr(defined.OrderNotFound, nil)
//
// 携带底层错误信息（底层错误不暴露给客户端，但会记录到日志/告警）：
//   return nil, e.NewHttpErr(defined.payGatewayErr, err)
//
// 包装底层错误（保留堆栈，向上传播）：
//   return nil, e.Err(err)
//
// 附带额外数据返回给客户端：
//   return nil, e.NewHttpErr(e.ParamErr, err).SetData("field 'amount' is required")
