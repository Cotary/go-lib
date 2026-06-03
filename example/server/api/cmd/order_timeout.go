//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package cmd

import (
	"context"
	"time"

	"myproject/model"

	"go-lib/cmd"
	"go-lib/dao/gormDB"
	"go-lib/log"
)

// ===== 定时任务启动 =====

// StartScheduler 创建并启动定时任务调度器
func StartScheduler() {
	scheduler, err := cmd.NewScheduler()
	if err != nil {
		panic(err)
	}
	// 注册任务（同名 id 幂等，不会重复注册）
	_ = scheduler.AddJob("order-timeout-check", &OrderTimeoutJob{})
	scheduler.Start()
}

// ===== 定时任务实现 =====

// OrderTimeoutJob 订单超时检查任务。
// 实现 cmd.Handler 接口：Spec + MaxExecuteTime + Do
type OrderTimeoutJob struct{}

// Spec 返回 cron 表达式，支持 6 位秒级格式。
// "0 */5 * * * *" 表示每 5 分钟执行一次。
func (j *OrderTimeoutJob) Spec() string {
	return "0 */5 * * * *"
}

// MaxExecuteTime 任务最大预期执行时间，超过后触发告警（不取消任务）
func (j *OrderTimeoutJob) MaxExecuteTime() time.Duration {
	return 2 * time.Minute
}

// Do 任务执行逻辑，ctx 携带 RequestID 等链路信息
func (j *OrderTimeoutJob) Do(ctx context.Context) error {
	expireTime := time.Now().Add(-30 * time.Minute).Unix()

	orders, err := model.NewOrder().List(ctx,
		gormDB.WhereAlways("status", gormDB.OpEq, model.OrderStatusPending),
		gormDB.WhereRaw("created_at < ?", expireTime),
	)
	if err != nil {
		return err
	}

	for _, order := range orders {
		o := &model.Order{Status: model.OrderStatusFailed}
		if updateErr := o.Update(ctx, []string{"status"}, gormDB.ID(order.ID)); updateErr != nil {
			log.WithContext(ctx).WithField("order_id", order.ID).Error("update timeout order failed: " + updateErr.Error())
		}
	}

	log.WithContext(ctx).Info("order timeout check completed")
	return nil
}
