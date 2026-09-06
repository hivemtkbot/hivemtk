package pagination

import (
	"reflect"
	"time"
)

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

	idField := v.FieldByName("ID")
	if !idField.IsValid() {
		idField = v.FieldByName("Id")
	}
	if !idField.IsValid() || idField.Kind() != reflect.Uint64 {

		if idField.IsValid() && idField.Kind() == reflect.Uint {
			return EncodeCursor(ts, uint64(idField.Uint()))
		}
		return ""
	}

	return EncodeCursor(ts, idField.Uint())
}
