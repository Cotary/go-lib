package psql

import (
	"context"
	"gorm.io/gorm"
)

var DBDriver *GormDrive
var DB *gorm.DB

type Goods struct {
	BaseModel
	Name string `gorm:"column:name"`
}

func (p Goods) TableName() string {
	return "goods"
}

func NewGoods() *Goods {
	return new(Goods)
}

func (p *Goods) GetSingle(ctx context.Context) error {
	DBDriver.WithContext(ctx).First(p)
	return nil
}

func test() {
	DBDriver.CtxTransaction(context.Background(), func(ctx context.Context, tx *gorm.DB) error {
		g := NewGoods()
		g.Name = "test"
		return g.GetSingle(ctx)
	})

	QueryAndSave(context.Background(), DBDriver.WithContext(context.Background()), NewGoods(), nil, map[string]interface{}{"name": "test"})
}
