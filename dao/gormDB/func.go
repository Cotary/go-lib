package gormDB

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Cotary/go-lib/common/community"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ===== 类型与常量定义 =====

// Operation 标识 QueryAndSave 的实际执行操作
type Operation int8

const (
	// OperationNothing 表示未执行任何写入操作（如 Update 时 fields 为空 / 影响 0 行的情况）
	OperationNothing Operation = iota
	// OperationInsert 表示执行了插入操作
	OperationInsert
	// OperationUpdate 表示执行了更新操作（至少 1 行被更新）
	OperationUpdate
)

// String 返回 Operation 的字符串表示
func (o Operation) String() string {
	switch o {
	case OperationInsert:
		return "insert"
	case OperationUpdate:
		return "update"
	default:
		return "nothing"
	}
}

const (
	// CreateField 是创建时间字段名，Upsert 更新时通过 Omit 排除，避免覆盖原始创建时间
	CreateField = "created_at"
	// ModifyField 是修改时间字段名
	ModifyField = "updated_at"
)

// SqlRowsAffectedZero 表示 SQL 执行成功但影响行数为 0 的哨兵错误类型。
// 使用独立类型而非 errors.New，以便通过 errors.Is 精确匹配。
type SqlRowsAffectedZero string

func (e SqlRowsAffectedZero) Error() string { return string(e) }

// RowsAffectedZero 是 SqlRowsAffectedZero 的预定义实例，可通过 errors.Is 判断
const RowsAffectedZero = SqlRowsAffectedZero("RowsAffectedZero")

// ErrRowsAffectedMismatch 表示 Insert 实际写入行数与传入数据条数不一致。
//
// 常见原因：数据库存在唯一索引冲突（INSERT IGNORE / 静默忽略 / 部分行被跳过）。
// Insert 不容忍此行为，如需冲突可跳过的语义请改用 Save。
// 通过 errors.Is(err, ErrRowsAffectedMismatch) 判断，expected/actual 细节见 Error() 输出。
var ErrRowsAffectedMismatch = errors.New("RowsAffectedMismatch")

// BaseModel 是通用的数据库模型基类，包含主键 + 创建/更新时间戳。
// 业务模型可嵌入此结构体，配合 Scope / Paginate 等功能使用。
type BaseModel struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// ===== 错误辅助 =====

// DbErr 将预期内的空结果错误归一化为 nil（gorm.ErrRecordNotFound 和 RowsAffectedZero 视为正常）
func DbErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, RowsAffectedZero) {
		return nil
	}
	return err
}

// DbCheckErr 将查询结果拆分为「是否存在」和「是否出错」两部分
//   - has=true,  err=nil 表示查到数据
//   - has=false, err=nil 表示无数据但无错误（记录不存在）
//   - has=false, err≠ nil 表示真实错误
func DbCheckErr(err error) (has bool, dbErr error) {
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// DbAffectedErr 检查执行结果，若 RowsAffected == 0 则返回 RowsAffectedZero。
// 当 db.Error 不为 nil 时优先返回原始错误。
func DbAffectedErr(db *gorm.DB) error {
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return RowsAffectedZero
	}
	return nil
}

// ===== 内部 helper =====

// newStruct 根据传入值的类型创建一个新的零值实例，用于 Model() 调用避免污染原始数据
func newStruct(s interface{}) interface{} {
	if s == nil {
		return nil
	}
	t := reflect.TypeOf(s)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return reflect.New(t).Interface()
}

// applyScopes 依次应用 Scope 列表到 db，nil 项自动跳过
func applyScopes(db *gorm.DB, scopes []Scope) *gorm.DB {
	for _, sc := range scopes {
		if sc != nil {
			db = sc(db)
		}
	}
	return db
}

// ===== CRUD =====

// Insert 执行严格的批量插入操作。
//
// data 支持：
//   - 单条记录（结构体指针）/ map
//   - 切片类型（GORM 会自动 batch insert）
//
// 返回值：
//   - data 为空（nil / 空切片）时返回 RowsAffectedZero
//   - 实际写入条数 ≠ 传入条数时返回 ErrRowsAffectedMismatch
//   - 其他数据库错误原样返回
//
// 设计说明：严格的 Insert 不允许静默忽略行，如果业务允许冲突跳过，
// 请使用 Save（ON CONFLICT DO NOTHING / DO UPDATE）替代。
func (g *GormDrive) Insert(ctx context.Context, data interface{}) error {
	expected := int64(countRows(data))
	if expected == 0 {
		return RowsAffectedZero
	}
	db := g.WithContext(ctx).Create(data)
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected != expected {
		return fmt.Errorf("%w: expected=%d got=%d", ErrRowsAffectedMismatch, expected, db.RowsAffected)
	}
	return nil
}

// countRows 计算待插入的行数：切片/数组取 len，其他类型返回 1。
// nil 指针 / 空 interface 返回 0，防止 Create(nil) 导致 GORM 内部 panic。
func countRows(data any) int {
	if data == nil {
		return 0
	}
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return rv.Len()
	default:
		return 1
	}
}

// Update 按 scopes 指定的 WHERE 条件，用 data 更新记录。
//
// fields 控制更新字段：
//   - 为空时 GORM 使用默认行为（非零值字段）
//   - ["*"] 表示更新所有字段（含零值）
//   - 其他值表示仅更新指定列
func (g *GormDrive) Update(ctx context.Context, data interface{}, fields []string, scopes ...Scope) error {
	db := g.WithContext(ctx).Model(newStruct(data))
	db = applyScopes(db, scopes)
	db = applySelectFields(db, fields)
	return DbAffectedErr(db.Updates(data))
}

// Delete 按 scopes 指定条件删除记录，data 用于推断 Model 和表名
func (g *GormDrive) Delete(ctx context.Context, data interface{}, scopes ...Scope) error {
	db := g.WithContext(ctx).Model(newStruct(data))
	db = applyScopes(db, scopes)
	return DbAffectedErr(db.Delete(data))
}

// applySelectFields 为 Update / QueryAndSave 设置 Select 子句
//   - 为空时不加 Select，使用 GORM 默认行为
//   - ["*"] 时 Select("*") 更新所有字段（含零值）
//   - 其他值按指定字段更新
func applySelectFields(db *gorm.DB, fields []string) *gorm.DB {
	if len(fields) == 0 {
		return db
	}
	if len(fields) == 1 && fields[0] == "*" {
		return db.Select("*")
	}
	return db.Select(fields)
}

// ===== Upsert（Save） =====

// Save 执行 ON CONFLICT（Upsert）语义的写入操作。
//
// fields 控制冲突时的更新行为：
//   - nil / 空切片：DoNothing（INSERT ... ON CONFLICT DO NOTHING）
//   - ["*"]：UpdateAll（冲突时更新所有列）
//   - 其他：仅更新指定列
//
// conflictKeys 是冲突检测的列名，对应唯一索引或主键。
// MySQL 可省略，PostgreSQL/SQLite 必须显式指定。
//
// 设计说明：统一的 Save 替代了老版本 SaveIgnore/SaveOverwrite/SaveUpdate 三个方法。
// QueryAndSave 是先查后写的模式，fields 语义与此一致，保持统一的 API 设计。
// 如需更新所有字段传 ["*"]，如需仅更新部分字段传具体 fields 列名，
// 具体列名应与 Model 的数据库字段名对应。
func (g *GormDrive) Save(ctx context.Context, data interface{}, fields []string, conflictKeys ...string) error {
	return DbAffectedErr(g.saveRaw(ctx, data, fields, conflictKeys))
}

// saveRaw 是 Save / QueryAndSave 的内部实现，返回原始 *gorm.DB 而非经 DbAffectedErr 处理后的 error。
// 将 fields 到 OnConflict 子句的映射逻辑集中于此，确保 Save 与 QueryAndSave 行为一致。
func (g *GormDrive) saveRaw(ctx context.Context, data interface{}, fields []string, conflictKeys []string) *gorm.DB {
	doNothing := len(fields) == 0
	updateAll := len(fields) == 1 && fields[0] == "*"
	updateFields := fields
	if updateAll {
		updateFields = nil
	}

	var columns []clause.Column
	for _, v := range conflictKeys {
		columns = append(columns, clause.Column{Name: v})
	}
	return g.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   columns,
		DoUpdates: clause.AssignmentColumns(updateFields),
		DoNothing: doNothing,
		UpdateAll: updateAll,
	}).Create(data)
}

// QueryAndSave 先加锁查询，不存在则 Upsert 插入，已存在则 Update 更新指定字段。
//
// 整个操作运行在 CtxTransaction 中，自动复用外层事务或开启新事务。
//
// fields 的含义与 Update 一致：
//   - nil：记录已存在时不更新，返回 OperationNothing
//   - ["*"]：更新所有字段（含零值）
//   - 其他：仅更新指定列
func (g *GormDrive) QueryAndSave(
	ctx context.Context, data interface{}, fields []string, condition map[string]interface{},
) (op Operation, err error) {
	err = g.CtxTransaction(ctx, func(ctx context.Context) error {
		db := g.WithContext(ctx)

		queryStruct := newStruct(data)
		// Locking UPDATE 加行锁，防止并发事务同时读取并写入同一行
		takeErr := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where(condition).Take(queryStruct).Error

		if takeErr != nil {
			if errors.Is(takeErr, gorm.ErrRecordNotFound) {
				var conflictKeys []string
				for key := range condition {
					conflictKeys = append(conflictKeys, key)
				}
				insertResult := g.saveRaw(ctx, data, fields, conflictKeys)
				if insertResult.Error != nil {
					return insertResult.Error
				}
				if insertResult.RowsAffected > 0 {
					op = OperationInsert
				}
				return nil
			}
			return takeErr
		}

		if fields == nil {
			op = OperationNothing
			return nil
		}

		updateModel := db.Model(newStruct(data)).Where(condition)
		updateModel = applySelectFields(updateModel, fields)

		// Omit CreateField：更新时排除 created_at 避免覆盖原始创建时间
		updateResult := updateModel.Omit(CreateField).Updates(data)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected > 0 {
			op = OperationUpdate
		} else {
			op = OperationNothing
		}
		return nil
	})

	return op, err
}

// ===== 泛型查询 helper =====

// Get 查询单条记录，未找到时返回 gorm.ErrRecordNotFound。
// T 应为结构体类型，若使用 map[string]any 则需在 scopes 中通过 Table(...) 指定表名。
func Get[T any](ctx context.Context, g *GormDrive, scopes ...Scope) (T, error) {
	var res T
	db := g.WithContext(ctx).Model(new(T))
	db = applyScopes(db, scopes)
	err := db.Take(&res).Error
	return res, err
}

// OptionalGet 查询单条记录，将「未找到」归一化为零值返回而非错误。
//   - 找到时返回 (res, nil)
//   - 未找到时返回 (零值, nil)
//   - 数据库出错时返回 (零值, err)
//
// 命名说明：替代老版本 MustGet，因 Go 中 Must 通常表示 panic 语义，不适合此场景。
func OptionalGet[T any](ctx context.Context, g *GormDrive, scopes ...Scope) (T, error) {
	res, err := Get[T](ctx, g, scopes...)
	return res, DbErr(err)
}

// List 查询多条记录，结果为空时返回空切片而非错误
func List[T any](ctx context.Context, g *GormDrive, scopes ...Scope) ([]T, error) {
	var res []T
	db := g.WithContext(ctx).Model(new(T))
	db = applyScopes(db, scopes)
	err := db.Find(&res).Error
	return res, err
}

// PageList 在 List 基础上附加 Paginate 分页，自动执行 count 并回写 p.Total。
//
// p 可为 nil（此时不分页）。p.Total 会被自动填充，外部可直接使用 community.PageOf(list, *p) 构造响应。
func PageList[T any](ctx context.Context, g *GormDrive, p *community.Paging, scopes ...Scope) ([]T, error) {
	return List[T](ctx, g, append(scopes, Paginate(p))...)
}
