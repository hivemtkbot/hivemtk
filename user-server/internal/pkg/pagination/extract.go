package pagination

import (
	"reflect"
	"time"
)

// extractCursorFromItem 从结果集中提取最后一条的 (created_at, id) 作为游标
// 通过反射读取 CreatedAt / ID 字段（支持 CreatedAt + ID 命名约定）
func extractCursorFromItem(item any) Cursor {
	if item == nil {
		return ""
	}
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}

	// 查找 CreatedAt 字段
	createdAtField := v.FieldByName("CreatedAt")
	if !createdAtField.IsValid() {
		createdAtField = v.FieldByName("created_at")
	}
	if !createdAtField.IsValid() {
		return ""
	}

	var ts time.Time
	if createdAtField.Type() == reflect.TypeOf(time.Time{}) {
		ts = createdAtField.Interface().(time.Time)
	}

	// 查找 ID 字段
	idField := v.FieldByName("ID")
	if !idField.IsValid() {
		idField = v.FieldByName("Id")
	}
	if !idField.IsValid() || idField.Kind() != reflect.Uint64 {
		// 尝试 uint
		if idField.IsValid() && idField.Kind() == reflect.Uint {
			return EncodeCursor(ts, uint64(idField.Uint()))
		}
		return ""
	}

	return EncodeCursor(ts, idField.Uint())
}
