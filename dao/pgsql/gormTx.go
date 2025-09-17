package pgsql

import (
	"context"
	"database/sql"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type txKeyType struct{}

var txKey = txKeyType{}

func (g *GormDrive) getTxFromCtx(ctx context.Context) (*gorm.DB, bool) {
	txMap, ok := ctx.Value(txKey).(map[string]*gorm.DB)
	if !ok {
		return nil, false
	}
	tx, ok := txMap[g.ID]
	return tx, ok
}

func (g *GormDrive) setTxToCtx(ctx context.Context, tx *gorm.DB) context.Context {
	txMap, ok := ctx.Value(txKey).(map[string]*gorm.DB)
	if !ok {
		txMap = make(map[string]*gorm.DB)
		ctx = context.WithValue(ctx, txKey, txMap)
	}
	txMap[g.ID] = tx
	return ctx
}

func (g *GormDrive) deleteTxFromCtx(ctx context.Context) {
	if txMap, ok := ctx.Value(txKey).(map[string]*gorm.DB); ok {
		delete(txMap, g.ID)
	}
}

func (g *GormDrive) CtxTx(ctx context.Context, opts ...*sql.TxOptions) (*gorm.DB, context.Context) {
	if tx, ok := g.getTxFromCtx(ctx); ok && tx != nil {
		return tx, ctx
	}
	tx := g.DB.WithContext(ctx).Begin(opts...)
	ctx = g.setTxToCtx(ctx, tx)
	return tx, ctx
}

func (g *GormDrive) CtxCommit(ctx context.Context) error {
	tx, ok := g.getTxFromCtx(ctx)
	if !ok || tx == nil {
		return nil
	}
	defer g.deleteTxFromCtx(ctx)

	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "commit failed")
	}
	return nil
}

func (g *GormDrive) CtxRollback(ctx context.Context) error {
	tx, ok := g.getTxFromCtx(ctx)
	if !ok || tx == nil {
		return nil
	}
	defer g.deleteTxFromCtx(ctx)
	return tx.Rollback().Error
}

func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
	if tx, ok := g.getTxFromCtx(ctx); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return g.DB.WithContext(ctx)
}

func (g *GormDrive) CtxTransaction(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error, opts ...*sql.TxOptions) (err error) {
	// 已有事务则直接执行
	if tx, ok := g.getTxFromCtx(ctx); ok && tx != nil {
		return fn(ctx, tx.WithContext(ctx))
	}

	// 新事务
	tx, newCtx := g.CtxTx(ctx, opts...)

	defer func() {
		if r := recover(); r != nil {
			_ = g.CtxRollback(newCtx)
			panic(r)
		}
	}()

	if err = fn(newCtx, tx.WithContext(newCtx)); err != nil {
		_ = g.CtxRollback(newCtx)
		return err
	}
	return g.CtxCommit(newCtx)
}
