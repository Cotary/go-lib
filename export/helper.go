package export

import (
	"reflect"
	"strings"
)

// StructSliceToRows 将结构体切片转为表头 + 数据行，通过 struct tag 读取列名。
// tagKey 为 struct tag 的键名（如 "export"、"json"），无该 tag 或值为 "-" 的字段会被跳过。
//
//	type User struct {
//	    Name  string `export:"姓名"`
//	    Age   int    `export:"年龄"`
//	    inner string // 未导出，自动跳过
//	}
//	header, rows := export.StructSliceToRows(users, "export")
func StructSliceToRows[T any](items []T, tagKey string) (header []interface{}, rows [][]interface{}) {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil
	}

	type fieldInfo struct {
		index int
		name  string
	}

	var fields []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get(tagKey)
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		if tag == "" {
			continue
		}
		fields = append(fields, fieldInfo{index: i, name: tag})
	}

	header = make([]interface{}, len(fields))
	for i, f := range fields {
		header[i] = f.name
	}

	rows = make([][]interface{}, 0, len(items))
	for _, item := range items {
		v := reflect.ValueOf(item)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		row := make([]interface{}, len(fields))
		for i, f := range fields {
			row[i] = v.Field(f.index).Interface()
		}
		rows = append(rows, row)
	}

	return header, rows
}
