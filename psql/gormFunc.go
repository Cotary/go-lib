package psql

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

func NewBaseModel() *BaseModel {
	return new(BaseModel)
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

func (t BaseModel) MustGet(ctx context.Context, DB *gorm.DB, item interface{}, condition map[string]interface{}) error {
	err := DB.WithContext(ctx).Where(condition).First(item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (t BaseModel) Get(ctx context.Context, DB *gorm.DB, item interface{}, condition map[string]interface{}) error {
	return DB.WithContext(ctx).Where(condition).First(item).Error
}

func (t BaseModel) List(ctx context.Context, DB *gorm.DB, list interface{}, condition map[string]interface{}) error {
	return DB.WithContext(ctx).Where(condition).Find(list).Error
}

func (t BaseModel) Insert(ctx context.Context, DB *gorm.DB, data interface{}) error {
	return CheckRowsAffected(DB.WithContext(ctx).Create(data))
}

func (t BaseModel) Update(ctx context.Context, DB *gorm.DB, data interface{}, updateFields []string, condition map[string]interface{}) error {
	dbModel := DB.WithContext(ctx).Where(condition)
	if len(updateFields) > 0 {
		if len(updateFields) == 1 && updateFields[0] == "*" {
			dbModel.Select("*")
		} else {
			dbModel.Select(updateFields)
		}
	}
	return CheckRowsAffected(dbModel.Updates(data))
}

func (t BaseModel) QueryAndSave(ctx context.Context, DB *gorm.DB, data interface{}, updateFields []string, condition map[string]interface{}) (operation string, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		queryStruct := newStruct(data)
		err = t.Get(ctx, tx, queryStruct, condition)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var clausesFields []string
				for key := range condition {
					clausesFields = append(clausesFields, key)
				}
				insertResult := t.Save(ctx, tx, data, updateFields, clausesFields)
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
func (t BaseModel) Save(ctx context.Context, DB *gorm.DB, data interface{}, updateFields []string, clausesFields []string) *gorm.DB {

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

	dbModel := DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateFields),
		DoNothing: notUpdate,
		UpdateAll: updateAll,
	})
	return dbModel.Create(data)
}
