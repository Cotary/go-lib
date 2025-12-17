package pgsql

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"time"
)

const RowsAffectedZero = SqlRowsAffectedZero("RowsAffectedZero")

type SqlRowsAffectedZero string

func (e SqlRowsAffectedZero) Error() string { return string(e) }

const (
	ClauseOperationInsert  = "insert"
	ClauseOperationUpdate  = "update"
	ClauseOperationNothing = "nothing"

	CreateField = "created_at"
	ModifyField = "updated_at"
)

type BaseModel struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func newStruct(s interface{}) interface{} {
	t := reflect.TypeOf(s)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return reflect.New(t).Interface()
}

func DbErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func DbAffectedErr(db *gorm.DB) error {
	if db.RowsAffected == 0 && db.Error == nil {
		return RowsAffectedZero
	}
	return db.Error
}
func MustGet[T any](ctx context.Context, g *GormDrive, opts ...QueryOption) (res T, err error) {
	err = g.MustGet(ctx, &res, opts...)
	return res, err
}

func Get[T any](ctx context.Context, g *GormDrive, opts ...QueryOption) (res T, err error) {
	err = g.Get(ctx, &res, opts...)
	return res, err
}

func List[T any](ctx context.Context, g *GormDrive, opts ...QueryOption) (res []T, err error) {
	err = g.List(ctx, &res, opts...)
	return res, err
}

func (g *GormDrive) prepareQuery(ctx context.Context, model interface{}, opts ...QueryOption) *gorm.DB {
	emptyModel := newStruct(model)
	db := g.WithContext(ctx).Model(emptyModel)
	for _, opt := range opts {
		if opt != nil {
			db = opt(db)
		}
	}
	return db
}

func (g *GormDrive) MustGet(ctx context.Context, res interface{}, opts ...QueryOption) error {
	return DbErr(g.Get(ctx, res, opts...))
}

func (g *GormDrive) Get(ctx context.Context, res interface{}, opts ...QueryOption) error {
	return g.prepareQuery(ctx, res, opts...).First(res).Error
}

func (g *GormDrive) List(ctx context.Context, res interface{}, opts ...QueryOption) error {
	return g.prepareQuery(ctx, res, opts...).Find(res).Error
}

// Insert 插入数据
func (g *GormDrive) Insert(ctx context.Context, data interface{}) error {
	return DbAffectedErr(g.WithContext(ctx).Create(data))
}

// Update 更新数据
func (g *GormDrive) Update(ctx context.Context, data interface{}, updateFields []string, opts ...QueryOption) error {
	db := g.prepareQuery(ctx, data, opts...)
	db = applySelectFields(db, updateFields)
	return DbAffectedErr(db.Updates(data))
}
func (g *GormDrive) Delete(ctx context.Context, data interface{}, opts ...QueryOption) error {
	db := g.prepareQuery(ctx, data, opts...)
	return DbAffectedErr(db.Delete(data))
}

// Save (Upsert) 批量或单条保存，处理冲突
func (g *GormDrive) Save(ctx context.Context, data interface{}, updateFields []string, clausesFields []string) *gorm.DB {
	notUpdate := len(updateFields) == 0
	updateAll := len(updateFields) == 1 && updateFields[0] == "*"
	if updateAll {
		updateFields = []string{}
	}

	var columns []clause.Column
	for _, v := range clausesFields {
		columns = append(columns, clause.Column{Name: v})
	}

	return g.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateFields),
		DoNothing: notUpdate,
		UpdateAll: updateAll,
	}).Create(data)
}

// QueryAndSave 复杂逻辑：先锁查询，不存在则 Save(Upsert)，存在则 Update
func (g *GormDrive) QueryAndSave(ctx context.Context, data interface{}, updateFields []string, condition map[string]interface{}) (operation string, err error) {
	// 使用 CtxTransaction 确保这一系列操作在事务中（复用现有事务或新建）
	err = g.CtxTransaction(ctx, func(ctx context.Context) error {
		// 这里 g.WithContext(ctx) 会自动获取当前闭包内的事务 tx
		db := g.WithContext(ctx)

		queryStruct := newStruct(data)
		err = db.Clauses(clause.Locking{Strength: "UPDATE"}).Where(condition).First(queryStruct).Error

		// 1. 如果没找到，尝试插入 (Save 包含 Upsert 逻辑)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var clausesFields []string
				for key := range condition {
					clausesFields = append(clausesFields, key)
				}
				// 调用自身的 Save 方法
				insertResult := g.Save(ctx, data, updateFields, clausesFields)
				if insertResult.Error != nil {
					return insertResult.Error
				}
				if insertResult.RowsAffected > 0 {
					operation = ClauseOperationInsert
				}
				return nil
			}
			return err
		}

		// 2. 如果找到了，执行更新
		if updateFields == nil {
			operation = ClauseOperationNothing
			return nil
		}

		updateModel := db.Where(condition)
		updateModel = applySelectFields(updateModel, updateFields)

		updateResult := updateModel.Omit(CreateField).Updates(data)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected > 0 {
			operation = ClauseOperationUpdate
		} else {
			operation = ClauseOperationNothing
		}
		return nil
	})

	return operation, err
}

// applySelectFields 内部复用逻辑：处理 Select 字段
func applySelectFields(db *gorm.DB, fields []string) *gorm.DB {
	if len(fields) > 0 {
		if len(fields) == 1 && fields[0] == "*" {
			return db.Select("*")
		}
		return db.Select(fields)
	}
	return db
}
