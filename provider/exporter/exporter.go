package exporter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	baseExport "github.com/Cotary/go-lib/export"
	"github.com/gin-gonic/gin"
)

// 请求头常量
const (
	HeaderDownload     = "X-DOWNLOAD"      // 是否下载
	HeaderDownloadName = "X-DOWNLOAD-NAME" // 下载文件名
	HeaderExportFormat = "X-EXPORT-FORMAT" // 导出格式: excel, csv
)

// 错误定义
var (
	// ErrNoListField List字段不存在错误
	ErrNoListField = errors.New("response does not contain a valid 'List' field")
	// ErrNoExportFields 没有可导出的字段
	ErrNoExportFields = errors.New("no fields with 'export' tag found")
)

// IsDownload 检查请求是否为下载请求
func IsDownload(c *gin.Context) bool {
	return c.Request.Header.Get(HeaderDownload) == "TRUE"
}

// GetExportFormat 获取导出格式，默认为excel
func GetExportFormat(c *gin.Context) string {
	format := c.Request.Header.Get(HeaderExportFormat)
	format = strings.ToLower(format)
	if format == "" {
		return baseExport.FormatExcel
	}
	return format
}

// escapeInfo 转义函数信息
type escapeInfo struct {
	funcName string
	funcArg  string
}

// fieldInfo 字段信息
type fieldInfo struct {
	Path      string       // 字段路径
	escapes   []escapeInfo // 转义函数列表
	hasExport bool         // 是否有Export方法
	hasString bool         // 是否有String方法
}

// typeCache 类型缓存
type typeCache struct {
	fields  []*fieldInfo
	headers []interface{}
}

// 全局类型缓存
var typeCacheMap sync.Map // key: reflect.Type, value: *typeCache

// Exporter 导出器
type Exporter struct {
	list   reflect.Value
	header []interface{}
	fields []*fieldInfo
	writer baseExport.Writer
}

// NewExporter 创建新的导出器实例
func NewExporter() *Exporter {
	return &Exporter{}
}

// Run 执行导出操作
func (e *Exporter) Run(c *gin.Context, res interface{}) error {
	ctx := c.Request.Context()

	// 获取导出格式
	format := GetExportFormat(c)

	// 获取文件名
	fileName := c.Request.Header.Get(HeaderDownloadName)
	if fileName == "" {
		fileName = utils.MD5Sum(time.Now().String())
	}

	// 创建写入器
	writer, err := baseExport.NewWriter(ctx, format, fileName)
	if err != nil {
		return fmt.Errorf("create writer error: %w", err)
	}
	e.writer = writer

	// 解析数据并写入
	if err := e.analysis(ctx, res); err != nil {
		_ = writer.Close()
		_ = os.Remove(writer.FileName())
		return err
	}

	// 关闭写入器
	if err := writer.Close(); err != nil {
		_ = os.Remove(writer.FileName())
		return fmt.Errorf("close writer error: %w", err)
	}

	// 设置响应头并发送文件
	defer func() {
		_ = os.Remove(writer.FileName())
	}()

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", writer.FileName()))
	c.Header("Content-Type", writer.ContentType())
	c.File(writer.FileName())
	return nil
}

func (e *Exporter) analysis(ctx context.Context, res interface{}) error {
	personValue := reflect.ValueOf(res)
	if personValue.Kind() == reflect.Ptr {
		personValue = personValue.Elem()
	}

	// 检查是否为结构体
	if personValue.Kind() != reflect.Struct {
		return ErrNoListField
	}

	// 获取List字段
	listValue := personValue.FieldByName("List")
	if !listValue.IsValid() {
		return ErrNoListField
	}

	if listValue.Kind() == reflect.Ptr {
		if listValue.IsNil() {
			return ErrNoListField
		}
		listValue = listValue.Elem()
	}

	if listValue.Kind() != reflect.Slice {
		return ErrNoListField
	}

	e.list = listValue

	// 获取元素类型
	elemType := listValue.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	// 尝试从缓存获取类型信息
	if cached, ok := typeCacheMap.Load(elemType); ok {
		tc := cached.(*typeCache)
		e.fields = tc.fields
		e.header = tc.headers
	} else {
		// 解析类型信息
		e.header = make([]interface{}, 0)
		e.fields = make([]*fieldInfo, 0)

		if elemType.Kind() == reflect.Struct {
			e.parseType(elemType, "")
		}

		// 缓存类型信息
		if len(e.fields) > 0 {
			typeCacheMap.Store(elemType, &typeCache{
				fields:  e.fields,
				headers: e.header,
			})
		}
	}

	// 检查是否有可导出的字段
	if len(e.header) == 0 {
		return ErrNoExportFields
	}

	// 写入表头
	if err := e.writer.WriteHeader(e.header); err != nil {
		return fmt.Errorf("write header error: %w", err)
	}

	// 批量写入数据行
	if listValue.Len() > 0 {
		const batchSize = 100
		rows := make([][]interface{}, 0, batchSize)

		for i := 0; i < listValue.Len(); i++ {
			row := e.getRowValues(ctx, listValue.Index(i))
			rows = append(rows, row)

			if len(rows) >= batchSize {
				if err := e.writer.WriteRows(rows); err != nil {
					return fmt.Errorf("write rows error: %w", err)
				}
				rows = rows[:0]
			}
		}

		// 写入剩余数据
		if len(rows) > 0 {
			if err := e.writer.WriteRows(rows); err != nil {
				return fmt.Errorf("write rows error: %w", err)
			}
		}
	}

	return nil
}

// parseType 从类型信息解析
func (e *Exporter) parseType(fieldType reflect.Type, path string) {
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	if fieldType.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < fieldType.NumField(); i++ {
		field := fieldType.Field(i)
		currentPath := buildPath(path, field.Name)
		e.parseField(field, currentPath)
	}
}

// parseField 解析单个字段
func (e *Exporter) parseField(field reflect.StructField, path string) {
	tag := field.Tag.Get("export")
	// 没有export tag或者为"-"，不导出
	if tag == "" || tag == "-" {
		return
	}

	fieldType := field.Type
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	f := &fieldInfo{Path: path}
	ex := strings.Split(tag, ",")
	tagHeader := ex[0]

	// 解析转义函数
	for k, v := range ex {
		if k == 0 {
			continue
		}
		vex := strings.Split(v, "=")
		funcName := vex[0]
		args := ""
		if len(vex) > 1 {
			args = vex[1]
		}
		if _, ok := GetEscapeFunc(funcName); ok {
			f.escapes = append(f.escapes, escapeInfo{
				funcName: funcName,
				funcArg:  args,
			})
		}
	}

	// 检查是否有Export/String方法（Export优先级高于String）
	f.hasExport = hasExportMethod(fieldType)
	f.hasString = hasStringMethod(fieldType)

	e.header = append(e.header, tagHeader)
	e.fields = append(e.fields, f)
}

// getRowValues 获取一行数据
func (e *Exporter) getRowValues(ctx context.Context, value reflect.Value) []interface{} {
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	row := make([]interface{}, 0, len(e.fields))
	for _, field := range e.fields {
		row = append(row, e.getFieldValue(ctx, getNestedField(value, field.Path), field))
	}
	return row
}

// getFieldValue 获取字段值
func (e *Exporter) getFieldValue(ctx context.Context, value reflect.Value, config *fieldInfo) interface{} {
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}

	// 处理转义函数
	if len(config.escapes) > 0 {
		data := value.Interface()
		for _, esc := range config.escapes {
			f, _ := GetEscapeFunc(esc.funcName)
			var err error
			data, err = f(data, esc.funcArg)
			if err != nil {
				break
			}
		}
		return data
	}

	// 优先使用 Export 方法
	if config.hasExport {
		if value.IsZero() {
			return ""
		}
		return callExport(value)
	}

	// 其次使用 String 方法
	if config.hasString {
		return callString(value)
	}

	// 使用 utils.ToString 统一处理类型转换
	return convertValue(value)
}

// convertValue 转换值为可导出格式
func convertValue(value reflect.Value) interface{} {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.Bool:
		return value.Bool()
	case reflect.String:
		return value.String()
	case reflect.Map, reflect.Slice, reflect.Struct:
		// 使用 utils.ToString 处理复杂类型
		str, err := utils.ToString(value.Interface())
		if err != nil {
			return ""
		}
		return str
	default:
		return ""
	}
}

// buildPath 构建字段路径
func buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

// getNestedField 获取嵌套字段值
func getNestedField(v reflect.Value, fieldPath string) reflect.Value {
	fields := strings.Split(fieldPath, ".")
	for _, fieldName := range fields {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		v = v.FieldByName(fieldName)
		if !v.IsValid() {
			return reflect.Value{}
		}
	}
	return v
}

// hasExportMethod 检查类型是否有Export方法
func hasExportMethod(t reflect.Type) bool {
	_, ok := t.MethodByName("Export")
	if ok {
		return true
	}
	// 检查指针类型
	if t.Kind() != reflect.Ptr {
		_, ok = reflect.PtrTo(t).MethodByName("Export")
	}
	return ok
}

// hasStringMethod 检查类型是否有String方法
func hasStringMethod(t reflect.Type) bool {
	_, ok := t.MethodByName("String")
	if ok {
		return true
	}
	// 检查指针类型
	if t.Kind() != reflect.Ptr {
		_, ok = reflect.PtrTo(t).MethodByName("String")
	}
	return ok
}

// callExport 调用Export方法
func callExport(v reflect.Value) string {
	method := v.MethodByName("Export")
	if method.Kind() != reflect.Func {
		// 尝试获取指针的方法
		if v.CanAddr() {
			method = v.Addr().MethodByName("Export")
		}
	}
	if method.Kind() == reflect.Func {
		results := method.Call(nil)
		if len(results) > 0 {
			return results[0].String()
		}
	}
	return ""
}

// callString 调用String方法
func callString(v reflect.Value) string {
	method := v.MethodByName("String")
	if method.Kind() != reflect.Func {
		// 尝试获取指针的方法
		if v.CanAddr() {
			method = v.Addr().MethodByName("String")
		}
	}
	if method.Kind() == reflect.Func {
		results := method.Call(nil)
		if len(results) > 0 {
			return results[0].String()
		}
	}
	return ""
}
