package psql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Cotary/go-lib/common/community"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// 测试用的模型
type TestUser struct {
	BaseModel
	Name  string `gorm:"column:name;type:varchar(100)"`
	Email string `gorm:"column:email;type:varchar(100)"`
	Age   int    `gorm:"column:age"`
}

func (TestUser) TableName() string {
	return "test_users"
}

type TestOrder struct {
	BaseModel
	UserID    int64   `gorm:"column:user_id"`
	Amount    float64 `gorm:"column:amount"`
	Status    string  `gorm:"column:status;type:varchar(20)"`
	ProductID int64   `gorm:"column:product_id"`
}

func (TestOrder) TableName() string {
	return "test_orders"
}

// 创建测试数据库连接
func createTestDB(t *testing.T) *GormDrive {
	config := &GormConfig{
		Driver:      "sqlite",
		Dsn:         []string{":memory:"},
		LogLevel:    "info",
		MaxOpens:    10,
		MaxIdles:    5,
		IdleTimeout: 3600,
	}

	db := NewGorm(config)

	// 创建测试表
	err := db.AutoMigrate(&TestUser{}, &TestOrder{})
	assert.NoError(t, err)

	return db
}

// 测试基本事务操作
func TestGormDrive_CtxTx_Basic(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 开始事务
	tx, newCtx := db.CtxTx(ctx)
	fmt.Println("=== 开始事务 ===")

	// 在事务中插入数据
	user := &TestUser{
		Name:  "张三",
		Email: "zhangsan@example.com",
		Age:   25,
	}

	err := Insert(newCtx, tx, user)
	assert.NoError(t, err)
	fmt.Printf("插入用户: %+v\n", user)

	// 查询数据
	foundUser, err := Get[TestUser](newCtx, tx, WithWhere(map[string]interface{}{"name": "张三"}))
	assert.NoError(t, err)
	fmt.Printf("查询用户: %+v\n", foundUser)

	// 提交事务
	err = db.CtxCommit(newCtx)
	assert.NoError(t, err)
	fmt.Println("=== 事务提交成功 ===")

	// 验证数据已保存
	savedUser, err := Get[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": "张三"}))
	assert.NoError(t, err)
	assert.Equal(t, "张三", savedUser.Name)
}

// 测试事务回滚
func TestGormDrive_CtxTx_Rollback(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 开始事务
	tx, newCtx := db.CtxTx(ctx)
	fmt.Println("=== 开始事务（将回滚）===")

	// 在事务中插入数据
	user := &TestUser{
		Name:  "李四",
		Email: "lisi@example.com",
		Age:   30,
	}

	err := Insert(newCtx, tx, user)
	assert.NoError(t, err)
	fmt.Printf("插入用户: %+v\n", user)

	// 回滚事务
	err = db.CtxRollback(newCtx)
	assert.NoError(t, err)
	fmt.Println("=== 事务回滚成功 ===")

	// 验证数据未保存
	_, err = Get[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": "李四"}))
	assert.Error(t, err) // 应该找不到数据
}

// 测试嵌套事务
func TestGormDrive_CtxTx_Nested(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 外层事务
	outerTx, outerCtx := db.CtxTx(ctx)
	fmt.Println("=== 开始外层事务 ===")

	// 外层事务插入用户
	user := &TestUser{
		Name:  "王五",
		Email: "wangwu@example.com",
		Age:   28,
	}

	err := Insert(outerCtx, outerTx, user)
	assert.NoError(t, err)
	fmt.Printf("外层事务插入用户: %+v\n", user)

	// 内层事务（应该复用外层事务）
	innerTx, innerCtx := db.CtxTx(outerCtx)
	fmt.Println("=== 开始内层事务（复用外层事务）===")

	// 内层事务插入订单
	order := &TestOrder{
		UserID:    user.ID,
		Amount:    100.50,
		Status:    "pending",
		ProductID: 1,
	}

	err = Insert(innerCtx, innerTx, order)
	assert.NoError(t, err)
	fmt.Printf("内层事务插入订单: %+v\n", order)

	// 提交内层事务（实际上不会提交，因为复用外层事务）
	err = db.CtxCommit(innerCtx)
	assert.NoError(t, err)
	fmt.Println("=== 内层事务提交（实际未提交）===")

	// 提交外层事务
	err = db.CtxCommit(outerCtx)
	assert.NoError(t, err)
	fmt.Println("=== 外层事务提交成功 ===")

	// 验证数据已保存
	_, err = Get[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": "王五"}))
	assert.NoError(t, err)

	savedOrder, err := Get[TestOrder](ctx, db.DB, WithWhere(map[string]interface{}{"user_id": user.ID}))
	assert.NoError(t, err)
	fmt.Printf("保存的订单: %+v\n", savedOrder)
}

// 测试事务函数
func TestGormDrive_CtxTransaction(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	fmt.Println("=== 测试事务函数 ===")

	err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 插入用户
		user := &TestUser{
			Name:  "赵六",
			Email: "zhaoliu@example.com",
			Age:   35,
		}

		err := Insert(ctx, tx, user)
		if err != nil {
			return err
		}
		fmt.Printf("事务函数中插入用户: %+v\n", user)

		// 插入订单
		order := &TestOrder{
			UserID:    user.ID,
			Amount:    200.00,
			Status:    "completed",
			ProductID: 2,
		}

		err = Insert(ctx, tx, order)
		if err != nil {
			return err
		}
		fmt.Printf("事务函数中插入订单: %+v\n", order)

		return nil
	})

	assert.NoError(t, err)
	fmt.Println("=== 事务函数执行成功 ===")

	// 验证数据已保存
	savedUser, err := Get[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": "赵六"}))
	assert.NoError(t, err)
	fmt.Printf("保存的用户: %+v\n", savedUser)
}

// 测试事务函数回滚
func TestGormDrive_CtxTransaction_Rollback(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	fmt.Println("=== 测试事务函数回滚 ===")

	err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 插入用户
		user := &TestUser{
			Name:  "钱七",
			Email: "qianqi@example.com",
			Age:   40,
		}

		err := Insert(ctx, tx, user)
		if err != nil {
			return err
		}
		fmt.Printf("事务函数中插入用户: %+v\n", user)

		// 故意返回错误，触发回滚
		return fmt.Errorf("模拟错误，触发回滚")
	})

	assert.Error(t, err)
	fmt.Println("=== 事务函数回滚成功 ===")

	// 验证数据未保存
	_, err = Get[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": "钱七"}))
	assert.Error(t, err) // 应该找不到数据
}

// 测试WithContext方法
func TestGormDrive_WithContext(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 先插入一些测试数据
	user := &TestUser{
		Name:  "孙八",
		Email: "sunba@example.com",
		Age:   45,
	}
	err := Insert(ctx, db.DB, user)
	assert.NoError(t, err)

	// 开始事务
	_, newCtx := db.CtxTx(ctx)
	fmt.Println("=== 测试WithContext方法 ===")

	// 使用WithContext查询（应该在事务中）
	dbWithCtx := db.WithContext(newCtx)
	foundUser, err := Get[TestUser](newCtx, dbWithCtx, WithWhere(map[string]interface{}{"name": "孙八"}))
	assert.NoError(t, err)
	fmt.Printf("在事务中查询用户: %+v\n", foundUser)

	// 提交事务
	err = db.CtxCommit(newCtx)
	assert.NoError(t, err)
}

// 测试事务选项
func TestGormDrive_CtxTx_WithOptions(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 使用事务选项
	txOptions := &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  false,
	}

	tx, newCtx := db.CtxTx(ctx, txOptions)
	fmt.Println("=== 测试事务选项 ===")

	user := &TestUser{
		Name:  "周九",
		Email: "zhoujiu@example.com",
		Age:   50,
	}

	err := Insert(newCtx, tx, user)
	assert.NoError(t, err)
	fmt.Printf("使用事务选项插入用户: %+v\n", user)

	err = db.CtxCommit(newCtx)
	assert.NoError(t, err)
}

// 测试复杂事务场景
func TestGormDrive_ComplexTransaction(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	fmt.Println("=== 测试复杂事务场景 ===")

	err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 1. 插入用户
		user := &TestUser{
			Name:  "吴十",
			Email: "wushi@example.com",
			Age:   55,
		}

		err := Insert(ctx, tx, user)
		if err != nil {
			return err
		}
		fmt.Printf("插入用户: %+v\n", user)

		// 2. 插入多个订单
		orders := []*TestOrder{
			{
				UserID:    user.ID,
				Amount:    150.00,
				Status:    "pending",
				ProductID: 1,
			},
			{
				UserID:    user.ID,
				Amount:    250.00,
				Status:    "processing",
				ProductID: 2,
			},
		}

		for _, order := range orders {
			err = Insert(ctx, tx, order)
			if err != nil {
				return err
			}
			fmt.Printf("插入订单: %+v\n", order)
		}

		// 3. 更新用户信息
		updateData := map[string]interface{}{
			"age": 56,
		}
		err = Update(ctx, tx, updateData, []string{"age"}, map[string]interface{}{"id": user.ID})
		if err != nil {
			return err
		}
		fmt.Printf("更新用户年龄: %d\n", 56)

		// 4. 查询验证
		foundUser, err := Get[TestUser](ctx, tx, WithWhere(map[string]interface{}{"id": user.ID}))
		if err != nil {
			return err
		}
		fmt.Printf("查询用户: %+v\n", foundUser)

		foundOrders, err := List[TestOrder](ctx, tx, WithWhere(map[string]interface{}{"user_id": user.ID}))
		if err != nil {
			return err
		}
		fmt.Printf("查询订单数量: %d\n", len(foundOrders))

		return nil
	})

	assert.NoError(t, err)
	fmt.Println("=== 复杂事务场景执行成功 ===")
}

// 测试并发事务
func TestGormDrive_ConcurrentTransactions(t *testing.T) {
	db := createTestDB(t)

	fmt.Println("=== 测试并发事务 ===")

	// 使用通道来同步goroutine
	done := make(chan bool, 2)

	// 第一个事务
	go func() {
		ctx := context.Background()
		err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
			user := &TestUser{
				Name:  "并发用户1",
				Email: "concurrent1@example.com",
				Age:   25,
			}

			err := Insert(ctx, tx, user)
			if err != nil {
				return err
			}
			fmt.Printf("并发事务1插入用户: %+v\n", user)

			// 模拟一些处理时间
			time.Sleep(100 * time.Millisecond)

			return nil
		})

		assert.NoError(t, err)
		done <- true
	}()

	// 第二个事务
	go func() {
		ctx := context.Background()
		err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
			user := &TestUser{
				Name:  "并发用户2",
				Email: "concurrent2@example.com",
				Age:   30,
			}

			err := Insert(ctx, tx, user)
			if err != nil {
				return err
			}
			fmt.Printf("并发事务2插入用户: %+v\n", user)

			// 模拟一些处理时间
			time.Sleep(100 * time.Millisecond)

			return nil
		})

		assert.NoError(t, err)
		done <- true
	}()

	// 等待两个事务完成
	<-done
	<-done

	fmt.Println("=== 并发事务执行完成 ===")

	// 验证数据
	ctx := context.Background()
	users, err := List[TestUser](ctx, db.DB, WithWhere(map[string]interface{}{"name": []string{"并发用户1", "并发用户2"}}))
	assert.NoError(t, err)
	assert.Len(t, users, 2)
}

// 测试事务中的查询操作
func TestGormDrive_TransactionQueries(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 先插入测试数据
	user := &TestUser{
		Name:  "查询测试用户",
		Email: "query@example.com",
		Age:   35,
	}
	err := Insert(ctx, db.DB, user)
	assert.NoError(t, err)

	fmt.Println("=== 测试事务中的查询操作 ===")

	err = db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 1. 基本查询
		foundUser, err := Get[TestUser](ctx, tx, WithWhere(map[string]interface{}{"id": user.ID}))
		if err != nil {
			return err
		}
		fmt.Printf("基本查询: %+v\n", foundUser)

		// 2. 条件查询
		users, err := List[TestUser](ctx, tx, WithWhere(map[string]interface{}{"age": 35}))
		if err != nil {
			return err
		}
		fmt.Printf("条件查询结果数量: %d\n", len(users))

		// 3. 分页查询
		paging := &community.Paging{
			Page:     1,
			PageSize: 10,
		}
		pagedUsers, err := List[TestUser](ctx, tx, WithPaging(paging))
		if err != nil {
			return err
		}
		fmt.Printf("分页查询结果数量: %d\n", len(pagedUsers))

		// 4. 排序查询
		order := community.Order{
			OrderField: "age",
			OrderType:  "desc",
		}
		orderedUsers, err := List[TestUser](ctx, tx, WithOrders(order))
		if err != nil {
			return err
		}
		fmt.Printf("排序查询结果数量: %d\n", len(orderedUsers))

		return nil
	})

	assert.NoError(t, err)
	fmt.Println("=== 事务查询操作完成 ===")
}

// 测试事务中的更新和删除操作
func TestGormDrive_TransactionUpdates(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	// 先插入测试数据
	user := &TestUser{
		Name:  "更新测试用户",
		Email: "update@example.com",
		Age:   40,
	}
	err := Insert(ctx, db.DB, user)
	assert.NoError(t, err)

	fmt.Println("=== 测试事务中的更新操作 ===")

	err = db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 1. 更新操作
		updateData := map[string]interface{}{
			"age":   41,
			"email": "updated@example.com",
		}
		err := Update(ctx, tx, updateData, []string{"age", "email"}, map[string]interface{}{"id": user.ID})
		if err != nil {
			return err
		}
		fmt.Printf("更新用户: ID=%d, 新年龄=%d, 新邮箱=%s\n", user.ID, 41, "updated@example.com")

		// 2. 查询验证更新
		updatedUser, err := Get[TestUser](ctx, tx, WithWhere(map[string]interface{}{"id": user.ID}))
		if err != nil {
			return err
		}
		fmt.Printf("更新后用户: %+v\n", updatedUser)

		// 3. 使用Save进行插入或更新
		newUser := &TestUser{
			Name:  "Save测试用户",
			Email: "save@example.com",
			Age:   45,
		}
		result := Save(ctx, tx, newUser, []string{"name", "email", "age"}, []string{"email"})
		if result.Error != nil {
			return result.Error
		}
		fmt.Printf("Save操作结果: RowsAffected=%d\n", result.RowsAffected)

		return nil
	})

	assert.NoError(t, err)
	fmt.Println("=== 事务更新操作完成 ===")
}

// 测试事务中的QueryAndSave操作
func TestGormDrive_TransactionQueryAndSave(t *testing.T) {
	db := createTestDB(t)
	ctx := context.Background()

	fmt.Println("=== 测试事务中的QueryAndSave操作 ===")

	err := db.CtxTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		// 1. 测试插入新记录
		newUser := &TestUser{
			Name:  "QueryAndSave用户",
			Email: "queryandsave@example.com",
			Age:   50,
		}

		operation, err := QueryAndSave(ctx, tx, newUser, []string{"name", "email", "age"}, map[string]interface{}{"email": "queryandsave@example.com"})
		if err != nil {
			return err
		}
		fmt.Printf("QueryAndSave插入操作: %s, 用户: %+v\n", operation, newUser)

		// 2. 测试更新已存在记录
		updateUser := &TestUser{
			Name:  "QueryAndSave更新用户",
			Email: "queryandsave@example.com",
			Age:   51,
		}
		operation, err = QueryAndSave(ctx, tx, updateUser, []string{"name", "age"}, map[string]interface{}{"email": "queryandsave@example.com"})
		if err != nil {
			return err
		}
		fmt.Printf("QueryAndSave更新操作: %s, 用户: %+v\n", operation, updateUser)

		return nil
	})

	assert.NoError(t, err)
	fmt.Println("=== QueryAndSave操作完成 ===")
}
