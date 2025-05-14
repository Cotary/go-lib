package psql

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"sync"
)

const (
	DBTransactionKey = "DBTransactionManager"
)

func (g *GormDrive) CtxTx(ctx context.Context, opts ...*sql.TxOptions) (*gorm.DB, context.Context) {
	var txMap *sync.Map

	// 从上下文中获取 sync.Map
	if val := ctx.Value(DBTransactionKey); val != nil {
		if m, ok := val.(*sync.Map); ok {
			txMap = m
		}
	}
	// 如果没有，则初始化一个新的 sync.Map 并放入上下文
	if txMap == nil {
		txMap = &sync.Map{}
		ctx = context.WithValue(ctx, DBTransactionKey, txMap)
	}

	// 尝试从 sync.Map 中加载事务
	if tx, ok := txMap.Load(g.ID); ok {
		if db, ok := tx.(*gorm.DB); ok && db != nil {
			return db, ctx
		}
	}

	// 如果还没有事务，开始新事务并存入 sync.Map
	txBegin := g.Begin(opts...)
	txMap.Store(g.ID, txBegin)
	return txBegin, ctx
}

func (g *GormDrive) CtxCommit(ctx context.Context) error {
	// 从上下文中获取 sync.Map
	val := ctx.Value(DBTransactionKey)
	if val == nil {
		return nil
	}
	txMap, ok := val.(*sync.Map)
	if !ok {
		return nil
	}
	// 从 sync.Map 中加载对应事务
	if v, ok := txMap.Load(g.ID); ok {
		tx, _ := v.(*gorm.DB)
		// 尝试提交事务
		if err := tx.Commit().Error; err != nil {
			// 如果提交失败，则进行回滚，并返回包含两者错误信息的错误
			rbErr := tx.Rollback().Error
			return errors.New(fmt.Sprintf("commit error: %v, rollback error: %v", err, rbErr))
		}
		// 提交成功后清理事务记录
		txMap.Delete(g.ID)
	}
	return nil
}

//
//// DBTransactionManager 用于管理数据库事务的状态和嵌套层级。
//type DBTransactionManager struct {
//	tx           *gorm.DB
//	nestingLevel int64 // 嵌套层级，普通是1，没有事务是0
//	nestingId    int64 // 嵌套最大id，只能累加，避免重复
//	isNested     bool  // 是否正在嵌套事务中
//}
//
//func (t DBTransactionManager) isNormal() bool {
//	return t.nestingLevel > 0
//}
//
//func newTx(db *gorm.DB, opts ...*sql.TxOptions) *DBTransactionManager {
//	return &DBTransactionManager{
//		tx:           db.Begin(opts...),
//		nestingLevel: 1,
//		nestingId:    1,
//		isNested:     false,
//	}
//
//}
//
//const (
//	DBTransactionKey = "DBTransactionManager" // 用于上下文中的键
//)
//
//// TxCtxFunc 是一个处理事务的函数类型。
//type TxCtxFunc func(ctx context.Context, tx *gorm.DB) error
//
//func (g *GormDrive) getManager(ctx context.Context) (*DBTransactionManager, bool) {
//	name := DBTransactionKey + "-" + g.id
//	manager, ok := ctx.Value(name).(*DBTransactionManager)
//	return manager, ok
//}
//
//// WithContext 返回与给定上下文关联的事务。
//func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
//	manager, ok := g.getManager(ctx)
//	if ok && manager.isNormal() {
//		return manager.tx
//	}
//	return g.DB.WithContext(ctx)
//}
//
//// BeginTx 开始一个新的事务或返回当前事务。
//func (g *GormDrive) BeginTx(ctx context.Context) context.Context {
//	manager, ok := g.getManager(ctx)
//	if ok && manager.isNormal() {
//		return ctx
//	}
//	tx := newTx(g.DB)
//
//	name := DBTransactionKey + "-" + g.id
//	ctx = context.WithValue(ctx, name, tx)
//	return ctx
//}
//
//// NestedTx 开始一个嵌套事务。
//func (g *GormDrive) NestedTx(ctx context.Context, t TxCtxFunc) error {
//	ctx = g.BeginTx(ctx)
//	manager, _ := g.getManager(ctx)
//
//	manager.nestingLevel++
//	manager.nestingId++
//	savePointName := fmt.Sprintf("sp_%d", manager.nestingId)
//	manager.tx.SavePoint(savePointName)
//	manager.isNested = true
//
//	if err := t(ctx, manager.tx); err != nil {
//		manager.tx.RollbackTo(savePointName)
//		manager.nestingLevel--
//		manager.isNested = false
//		return err
//	}
//	return nil
//}
//
//// NestedTxCommit 提交一个嵌套事务。报错必须结束
//func (g *GormDrive) NestedTxCommit(ctx context.Context, t TxCtxFunc) error {
//	err := g.NestedTx(ctx, t)
//	manager, _ := g.getManager(ctx)
//	if err != nil {
//		manager.tx.Rollback() //嵌套事务中，有错误就直接回滚了,如果需要不返回就自己处理
//		return err
//	}
//	if manager.isNested {
//		return nil
//	}
//	return g.CommitTx(ctx)
//}
//
//// CommitTx 提交所有事务。
//func (g *GormDrive) CommitTx(ctx context.Context) error {
//
//	manager, ok := g.getManager(ctx)
//	defer func() {
//		manager.nestingLevel--
//	}()
//	if !ok || manager.nestingLevel < 1 {
//		return errors.New("no transaction to commit")
//	}
//	if manager.isNested {
//		return errors.New("in nested transaction")
//	}
//
//	if err := manager.tx.Commit().Error; err != nil {
//		manager.tx.Rollback() // 如果提交失败，回滚事务
//		return err
//	}
//	return nil
//}
