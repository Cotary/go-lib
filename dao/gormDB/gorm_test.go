package gormDB

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Goods struct {
	BaseModel
	Name string `gorm:"column:name"`
}

func (p Goods) TableName() string {
	return "goods"
}

func TestDbErr_RecordNotFound(t *testing.T) {
	err := DbErr(RowsAffectedZero)
	assert.NoError(t, err)
}

func TestDbCheckErr_NoError(t *testing.T) {
	has, err := DbCheckErr(nil)
	assert.True(t, has)
	assert.NoError(t, err)
}

func TestDbCheckErr_NotFound(t *testing.T) {
	has, err := DbCheckErr(RowsAffectedZero)
	assert.False(t, has)
	assert.Error(t, err)
}

func TestMustGet_NotFoundReturnsNil(t *testing.T) {
	db := openDryRunDB(t)
	drive := &GormDrive{ID: "test", db: db}
	ctx := context.Background()

	var res Goods
	err := drive.MustGet(ctx, &res)
	_ = err
}
