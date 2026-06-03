//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"myproject/config"

	http2 "go-lib/net/http"

	"github.com/pkg/errors"
)

const requestTimeout = 10 * time.Second

// QuerypayStatus 调用外部支付网关查询支付状态。
// 展示 go-lib/net/http 的 FastHTTP 客户端 + 中间件链用法。
func QuerypayStatus(ctx context.Context, tradeNo string) (string, error) {
	url := fmt.Sprintf("%s/api/pay/status", config.Config.payGateway.Host)

	query := map[string][]string{
		"trade_no": {tradeNo},
	}

	var status string
	err := http2.FastHTTP().
		Use(
			http2.TimeoutMiddleware(requestTimeout),
			http2.CodeCheckMiddleware(0),
		).
		Execute(ctx, http.MethodGet, url, query, nil, nil).
		ParseTo("data.status", &status)
	if err != nil {
		return "", errors.Wrap(err, "query pay status error")
	}
	return status, nil
}

// Createpay 调用外部网关创建支付单。
// 展示 POST + JSON body + 自定义 Header 的写法。
func Createpay(ctx context.Context, orderID string, amount string, currency string) (string, error) {
	url := fmt.Sprintf("%s/api/pay/create", config.Config.payGateway.Host)

	body := map[string]string{
		"order_id": orderID,
		"amount":   amount,
		"currency": currency,
	}
	headers := map[string]string{
		"X-Api-Key": config.Config.payGateway.SecretKey,
	}

	var payURL string
	err := http2.FastHTTP().
		Use(
			http2.TimeoutMiddleware(requestTimeout),
			http2.StatusCodeCheckMiddleware(200),
			http2.CodeCheckMiddleware(0),
		).
		Execute(ctx, http.MethodPost, url, nil, body, headers).
		ParseTo("data.pay_url", &payURL)
	if err != nil {
		return "", errors.Wrap(err, "create pay error")
	}
	return payURL, nil
}
