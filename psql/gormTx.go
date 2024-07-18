package psql

import (
	"context"
	"errors"
	"fmt"
	_ "github.com/lib/pq"
	"gorm.io/gorm"
)

// DBTransactionManager 用于管理数据库事务的状态和嵌套层级。
type DBTransactionManager struct {
	tx           *gorm.DB
	nestingLevel int64 // 嵌套层级，普通是1，没有事务是0
	nestingId    int64 // 嵌套最大id，只能累加，避免重复
	isNested     bool  // 是否正在嵌套事务中

}

func newTx(db *gorm.DB) *DBTransactionManager {
	return &DBTransactionManager{
		tx:           db.Begin(),
		nestingLevel: 1,
		nestingId:    1,
		isNested:     false,
	}

}

const (
	DBTransactionKey = "DBTransactionManager" // 用于上下文中的键
	DBOpenTxKey      = "DBOpenTx"             // 用于上下文中的键
	CtxTxListKey     = "CtxTxList"            // 用于上下文中的键
)

// TxCtxFunc 是一个处理事务的函数类型。
type TxCtxFunc func(ctx context.Context, tx *gorm.DB) error

func (g *GormDrive) getManager(ctx context.Context) (*DBTransactionManager, bool) {
	name := DBTransactionKey + "-" + g.id
	manager, ok := ctx.Value(name).(*DBTransactionManager)
	return manager, ok
}

func GetCtxAllTx(ctx context.Context) (*[]DBTransactionManager, bool) {
	managerList, ok := ctx.Value(CtxTxListKey).(*[]DBTransactionManager)
	return managerList, ok
}

// WithContext 返回与给定上下文关联的事务。
func (g *GormDrive) WithContext(ctx context.Context) *gorm.DB {
	//manager, ok := g.getManager(*ctx)
	//if ok {
	//	return manager.tx
	//}
	//openTx, ok := (*ctx).Value(DBOpenTxKey).(bool)
	//if openTx && ok {
	//	manager, _ = g.getManager(g.BeginTx(*ctx))
	//	return manager.tx
	//}
	return g.DB.WithContext(ctx)
}

// BeginTx 开始一个新的事务或返回当前事务。
func (g *GormDrive) BeginTx(ctx context.Context) context.Context {
	manager, ok := g.getManager(ctx)
	if ok && manager.nestingLevel >= 1 {
		return nil
	}
	tx := newTx(g.DB.WithContext(ctx))

	name := DBTransactionKey + "-" + g.id
	ctx = context.WithValue(ctx, name, tx)
	return ctx
}

// NestedTx 开始一个嵌套事务。
func (g *GormDrive) NestedTx(ctx context.Context, t TxCtxFunc) error {
	ctx = g.BeginTx(ctx)
	manager, _ := g.getManager(ctx)

	manager.nestingLevel++
	manager.nestingId++
	savePointName := fmt.Sprintf("sp_%d", manager.nestingId)
	manager.tx.SavePoint(savePointName)
	manager.isNested = true

	if err := t(ctx, manager.tx); err != nil {
		manager.tx.RollbackTo(savePointName)
		manager.nestingLevel--
		manager.isNested = false
		return err
	}
	return nil
}

// NestedTxCommit 提交一个嵌套事务。报错必须结束
func (g *GormDrive) NestedTxCommit(ctx context.Context, t TxCtxFunc) error {
	err := g.NestedTx(ctx, t)
	manager, _ := g.getManager(ctx)
	if err != nil {
		manager.tx.Rollback() //嵌套事务中，有错误就直接回滚了,如果需要不返回就自己处理
		return err
	}
	if manager.isNested {
		return nil
	}
	return g.CommitTx(ctx)
}

// CommitTx 提交所有事务。
func (g *GormDrive) CommitTx(ctx context.Context) error {

	manager, ok := g.getManager(ctx)
	defer func() {
		manager.nestingLevel--
	}()
	if !ok || manager.nestingLevel < 1 {
		return errors.New("no transaction to commit")
	}
	if manager.isNested {
		return errors.New("in nested transaction")
	}

	if err := manager.tx.Commit().Error; err != nil {
		manager.tx.Rollback() // 如果提交失败，回滚事务
		return err
	}
	return nil
}
