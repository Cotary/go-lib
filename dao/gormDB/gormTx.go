package gormDB

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/google/uuid"
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
	if g.db == nil {
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
		// 嵌套事务使用父事务的 ID，如果没有则生成新的
		txID, _ := ctx.Value(defined.TransactionID).(string)
		if txID == "" {
			txID = uuid.NewString()
		}
		newCtx := context.WithValue(ctx, defined.TransactionID, txID)

		// 记录嵌套事务开始
		if g.Logger != nil {
			g.Logger.Info(newCtx, fmt.Sprintf("TRANSACTION BEGIN NESTED "))
		}

		err := tx.Transaction(func(subTx *gorm.DB) error {
			// 将 tx 和事务 ID 注入到新的 Context 中
			newCtx = context.WithValue(newCtx, ctxTransactionKey{DbID: g.ID}, subTx)
			return fn(newCtx)
		}, opts...)

		// 记录嵌套事务结束
		if g.Logger != nil {
			elapsed := time.Since(beginTime)
			if err != nil {
				g.Logger.Warn(newCtx, fmt.Sprintf("[%v] TRANSACTION ROLLBACK NESTED :%s", elapsed, err.Error()))
			} else {
				g.Logger.Info(newCtx, fmt.Sprintf("[%v] TRANSACTION COMMIT NESTED", elapsed))
			}
		}

		return err
	}

	// 2. 开启新事务，使用 GORM 的 Transaction 方法，它自动处理 Commit/Rollback/Panic
	// 为新事务生成唯一的事务 ID（如果 context 中已有则使用已有的）
	txID, _ := ctx.Value(defined.TransactionID).(string)
	if txID == "" {
		txID = uuid.NewString()
	}
	newCtx := context.WithValue(ctx, defined.TransactionID, txID)

	// 记录事务开始
	if g.Logger != nil {
		g.Logger.Info(newCtx, fmt.Sprintf("TRANSACTION BEGIN "))
	}

	err := g.db.WithContext(newCtx).Transaction(func(tx *gorm.DB) error {
		// 将 tx 和事务 ID 注入到新的 Context 中
		newCtx = context.WithValue(newCtx, ctxTransactionKey{DbID: g.ID}, tx)
		return fn(newCtx)
	}, opts...)

	// 记录事务结束
	if g.Logger != nil {
		elapsed := time.Since(beginTime)
		if err != nil {
			g.Logger.Warn(newCtx, fmt.Sprintf("[%v] TRANSACTION ROLLBACK :%s", elapsed, err.Error()))
		} else {
			g.Logger.Info(newCtx, fmt.Sprintf("[%v] TRANSACTION COMMIT", elapsed))
		}
	}

	return err
}

// WithContext 获取当前可用的 DB（如果有事务用事务，没事务用原生 DB）
func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
	if tx := g.getTxFromCtx(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return g.db.WithContext(ctx)
}
