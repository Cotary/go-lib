package pgsql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ctxTransactionKey struct {
	DbID string
}

func (g *GormDrive) getTxFromCtx(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return nil
	}
	if val := ctx.Value(ctxTransactionKey{DbID: g.ID}); val != nil {
		if tx, ok := val.(*gorm.DB); ok {
			return tx
		}
	}
	return nil
}

// CtxTransaction 推荐只保留这一个入口，强制用户使用闭包模式，防止忘记 Commit/Rollback
func (g *GormDrive) CtxTransaction(ctx context.Context, fn func(ctx context.Context) error, opts ...*sql.TxOptions) error {
	if g.DB == nil {
		return errors.New("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 记录事务开始时间
	beginTime := time.Now()

	// 1. 检查是否已有事务（处理嵌套）
	if tx := g.getTxFromCtx(ctx); tx != nil {
		// 如果已有事务，使用 GORM 的原生嵌套事务支持 (SavePoint)
		// 记录嵌套事务开始
		if g.Logger != nil {
			g.Logger.Info(ctx, fmt.Sprintf("TRANSACTION BEGIN NESTED"))
		}

		err := tx.Transaction(func(subTx *gorm.DB) error {
			newCtx := context.WithValue(ctx, ctxTransactionKey{DbID: g.ID}, subTx)
			return fn(newCtx)
		}, opts...)

		// 记录嵌套事务结束
		if g.Logger != nil {
			elapsed := time.Since(beginTime)
			if err != nil {
				g.Logger.Warn(ctx, fmt.Sprintf("TRANSACTION ROLLBACK NESTED :%s", err.Error()))
			} else {
				g.Logger.Info(ctx, fmt.Sprintf("TRANSACTION COMMIT NESTED"))
			}
		}

		return err
	}

	// 2. 开启新事务，使用 GORM 的 Transaction 方法，它自动处理 Commit/Rollback/Panic
	// 记录事务开始
	if g.Logger != nil {
		g.Logger.Info(ctx, fmt.Sprintf("TRANSACTION BEGIN"))
	}

	err := g.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 将 tx 注入到新的 Context 中
		newCtx := context.WithValue(ctx, ctxTransactionKey{DbID: g.ID}, tx)
		return fn(newCtx)
	}, opts...)

	// 记录事务结束
	if g.Logger != nil {
		elapsed := time.Since(beginTime)
		if err != nil {
			g.Logger.Warn(ctx, fmt.Sprintf("TRANSACTION ROLLBACK :%s", err.Error()))
		} else {
			g.Logger.Info(ctx, fmt.Sprintf("TRANSACTION COMMIT"))
		}
	}

	return err
}

// WithContext 获取当前可用的 DB（如果有事务用事务，没事务用原生 DB）
func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
	if tx := g.getTxFromCtx(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return g.DB.WithContext(ctx)
}
