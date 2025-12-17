package pgsql

import (
	"context"
)

var DBDriver *GormDrive

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
	DBDriver.CtxTransaction(context.Background(), func(ctx context.Context) error {
		g := NewGoods()
		g.Name = "test"
		return g.GetSingle(ctx)
	})

	DBDriver.QueryAndSave(context.Background(), NewGoods(), nil, map[string]interface{}{"name": "test"})
}
