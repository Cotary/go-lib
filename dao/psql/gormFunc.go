package psql

import (
	"context"
	"errors"
	"github.com/Cotary/go-lib/common/community"
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

func CheckRowsAffected(tx *gorm.DB) error {
	if tx.RowsAffected == 0 && tx.Error == nil {
		return RowsAffectedZero
	}
	return tx.Error
}

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

func (t *GormDrive) MustGet(ctx context.Context, item interface{}, condition map[string]interface{}) error {
	err := t.WithContext(ctx).Where(condition).First(item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (t *GormDrive) Get(ctx context.Context, item interface{}, condition map[string]interface{}) error {
	return t.WithContext(ctx).Where(condition).First(item).Error
}

func (t *GormDrive) List(ctx context.Context, list interface{}, condition map[string]interface{}) error {
	return t.WithContext(ctx).Where(condition).Find(list).Error
}

func (t *GormDrive) Insert(ctx context.Context, data interface{}) error {
	return CheckRowsAffected(t.WithContext(ctx).Create(data))
}

func (t *GormDrive) Update(ctx context.Context, data interface{}, updateFields []string, condition map[string]interface{}) error {
	dbModel := t.WithContext(ctx).Where(condition)
	if len(updateFields) > 0 {
		if len(updateFields) == 1 && updateFields[0] == "*" {
			dbModel.Select("*")
		} else {
			dbModel.Select(updateFields)
		}
	}
	return CheckRowsAffected(dbModel.Updates(data))
}

func (t *GormDrive) QueryAndSave(ctx context.Context, data interface{}, updateFields []string, condition map[string]interface{}) (operation string, err error) {
	err = t.Transaction(func(tx *gorm.DB) error {
		txDrive := NewGormDrive(tx)
		queryStruct := newStruct(data)
		err = txDrive.Get(ctx, queryStruct, condition)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var clausesFields []string
				for key := range condition {
					clausesFields = append(clausesFields, key)
				}
				insertResult := txDrive.Save(ctx, data, updateFields, clausesFields)
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
		updateModel := txDrive.WithContext(ctx).Where(condition)
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
func (t *GormDrive) Save(ctx context.Context, data interface{}, updateFields []string, clausesFields []string) *gorm.DB {

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

	dbModel := t.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateFields),
		DoNothing: notUpdate,
		UpdateAll: updateAll,
	})
	return dbModel.Create(data)
}

func Paging(session *gorm.DB, paging *community.Paging) *gorm.DB {
	if paging.PageSize < 1 {
		paging.PageSize = 20
	}
	if paging.Page < 1 {
		paging.Page = 1
	}

	return session.Limit(paging.PageSize).Offset((paging.Page - 1) * paging.PageSize)
}
func Page(session *gorm.DB, count *int64) *gorm.DB {
	return session.Limit(-1).Offset(-1).Count(count)
}
