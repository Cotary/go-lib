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

// ctxTransactionKey 是将 *gorm.DB 事务存入 context 时使用的 key。
//
// 以 DbID 作为区分，使同一进程内多个 GormDrive 实例各自独立管理事务 + 嵌套事务。
// 不同数据库实例使用不同的 key，互不干扰。
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

// CtxTransaction 以闭包方式执行事务，自动管理生命周期。
//   - fn 返回 error 时自动 Rollback，返回 nil 时自动 Commit
//   - fn 中可通过 WithContext(ctx) 获取 *gorm.DB 来执行事务内操作
//   - 检测到外层已有事务时使用 SavePoint（GORM 原生嵌套事务支持）
//   - 每层事务自动生成或复用 TransactionID，用于日志追踪关联
//
// opts 可指定事务隔离级别，嵌套事务时仅对最外层 SavePoint 生效。
// 典型用法无需传递，默认使用数据库的 sql.LevelReadCommitted。
func (g *GormDrive) CtxTransaction(ctx context.Context, fn func(ctx context.Context) error, opts ...*sql.TxOptions) error {
	if g.db == nil {
		return errors.New("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	beginTime := time.Now()

	if tx := g.getTxFromCtx(ctx); tx != nil {
		txID, _ := ctx.Value(defined.TransactionID).(string)
		if txID == "" {
			txID = uuid.NewString()
		}
		newCtx := context.WithValue(ctx, defined.TransactionID, txID)

		if g.Logger != nil {
			g.Logger.Info(newCtx, "TRANSACTION BEGIN NESTED")
		}

		err := tx.Transaction(func(subTx *gorm.DB) error {
			newCtx = context.WithValue(newCtx, ctxTransactionKey{DbID: g.ID}, subTx)
			return fn(newCtx)
		}, opts...)

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

	txID, _ := ctx.Value(defined.TransactionID).(string)
	if txID == "" {
		txID = uuid.NewString()
	}
	newCtx := context.WithValue(ctx, defined.TransactionID, txID)

	if g.Logger != nil {
		g.Logger.Info(newCtx, "TRANSACTION BEGIN")
	}

	err := g.db.WithContext(newCtx).Transaction(func(tx *gorm.DB) error {
		newCtx = context.WithValue(newCtx, ctxTransactionKey{DbID: g.ID}, tx)
		return fn(newCtx)
	}, opts...)

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

// WithContext 返回带有上下文的 *gorm.DB 实例。
//   - 若 ctx 中存在事务则返回事务 *gorm.DB，与 CtxTransaction 配合实现读/写在同一事务
//   - 若 ctx 中不存在事务则返回普通 *gorm.DB
//
// 典型用法如下（不需关心事务细节）：
//
//	db := g.WithContext(ctx)
//	db.Scopes(...).Find(&out)
func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
	if tx := g.getTxFromCtx(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return g.db.WithContext(ctx)
}
