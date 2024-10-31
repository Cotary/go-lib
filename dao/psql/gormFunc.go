package psql

import (
	"context"
	"errors"
	"github.com/Cotary/go-lib/common/community"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"strings"
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

func MustGet(ctx context.Context, db *gorm.DB, item interface{}, condition map[string]interface{}) error {
	err := db.WithContext(ctx).Where(condition).First(item).Error
	return DbErr(err)
}

func Get(ctx context.Context, db *gorm.DB, item interface{}, condition map[string]interface{}) error {
	return db.WithContext(ctx).Where(condition).First(item).Error
}

func List(ctx context.Context, db *gorm.DB, list interface{}, condition map[string]interface{}) error {
	return db.WithContext(ctx).Where(condition).Find(list).Error
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

func Paging(db *gorm.DB, paging *community.Paging) *gorm.DB {
	if paging.PageSize < 1 {
		paging.PageSize = 20
	}
	if paging.Page < 1 {
		paging.Page = 1
	}
	if paging.All {
		return db
	}
	return db.Limit(paging.PageSize).Offset((paging.Page - 1) * paging.PageSize)
}
func Total(db *gorm.DB, count *int64) *gorm.DB {
	return db.Limit(-1).Offset(-1).Count(count)
}

func Order(db *gorm.DB, order community.Order, bind map[string]string) *gorm.DB {
	if order.OrderField == "" {
		return db
	}
	orderType := "asc"
	if order.OrderType == "desc" {
		orderType = "desc"
	}
	field, ok := bind[order.OrderField]
	orderStr := ""
	if ok {
		if strings.Contains(field, "{order_type}") {
			orderStr = strings.Replace(field, "{order_type}", orderType, -1)
		} else {
			orderStr = field + " " + orderType
		}
		return db.Order(orderStr)
	}
	return db
}
