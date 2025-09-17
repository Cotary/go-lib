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

func MustGet[T any](ctx context.Context, db *gorm.DB, opts ...QueryOption) (res T, err error) {
	q := db.WithContext(ctx).Model(new(T))
	for _, opt := range opts {
		if opt != nil {
			q = opt(q)
		}
	}
	err = q.First(&res).Error
	return res, DbErr(err)
}

func Get[T any](ctx context.Context, db *gorm.DB, opts ...QueryOption) (res T, err error) {
	q := db.WithContext(ctx).Model(new(T))
	for _, opt := range opts {
		if opt != nil {
			q = opt(q)
		}
	}
	err = q.First(&res).Error
	return res, err
}

func List[T any](ctx context.Context, db *gorm.DB, opts ...QueryOption) (res []T, err error) {
	q := db.WithContext(ctx).Model(new(T))
	for _, opt := range opts {
		if opt != nil {
			q = opt(q)
		}
	}
	err = q.Find(&res).Error
	return res, err
}

func Insert(ctx context.Context, db *gorm.DB, data interface{}) error {
	return DbAffectedErr(db.WithContext(ctx).Create(data))
}

func Update(ctx context.Context, db *gorm.DB, data interface{}, updateFields []string, condition map[string]interface{}) error {
	dbModel := db.WithContext(ctx).Where(condition)
	if len(updateFields) > 0 {
		if len(updateFields) == 1 && updateFields[0] == "*" {
			dbModel.Select("*")
		} else {
			dbModel.Select(updateFields)
		}
	}
	return DbAffectedErr(dbModel.Updates(data))
}

func QueryAndSave(ctx context.Context, db *gorm.DB, data interface{}, updateFields []string, condition map[string]interface{}) (operation string, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		queryStruct := newStruct(data)
		err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(condition).First(queryStruct).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var clausesFields []string
				for key := range condition {
					clausesFields = append(clausesFields, key)
				}
				insertResult := Save(ctx, tx, data, updateFields, clausesFields)
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
		//更新
		if updateFields == nil {
			return nil
		}
		updateModel := tx.WithContext(ctx).Where(condition)
		//没有指定的话就只更新结构体不为空值的部分
		if len(updateFields) > 0 {
			if len(updateFields) == 1 && updateFields[0] == "*" {
				updateModel.Select("*") //全部更新
			} else {
				updateModel.Select(updateFields) //只更新个别
			}
		}
		updateResult := updateModel.Omit(CreateField).Updates(data)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected > 0 {
			operation = ClauseOperationUpdate
		}
		return nil
	})
	return

}
func Save(ctx context.Context, db *gorm.DB, data interface{}, updateFields []string, clausesFields []string) *gorm.DB {

	notUpdate := false
	if updateFields == nil {
		notUpdate = true
	}
	updateAll := false
	if len(updateFields) == 1 && updateFields[0] == "*" {
		updateAll = true
		updateFields = []string{}
	}

	var columns []clause.Column
	for _, v := range clausesFields {
		columns = append(columns, clause.Column{
			Name: v,
		})
	}

	dbModel := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateFields),
		DoNothing: notUpdate,
		UpdateAll: updateAll,
	})
	return dbModel.Create(data)
}

//整合一个list查询
//搞一个condition方式
