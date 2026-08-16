package pagination

import "reflect"

// sliceLen 获取 slice 长度（支持 []*Model 与 []Model）
func sliceLen(dest any) int {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Slice {
		return 0
	}
	return v.Len()
}

// sliceTruncate 截断 slice 到指定长度
func sliceTruncate(dest any, n int) {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Slice {
		return
	}
	if v.Len() <= n {
		return
	}
	// 取前 n 个，替换原 slice
	truncated := v.Slice(0, n)
	// 通过 reflect.Set 写回（dest 必须传指针）
	if v.CanSet() {
		v.Set(truncated)
	} else if v.CanAddr() {
		// 处理指针 slice 情况（*[]*Model）
		// reflect.Set 不可行，改用 indirect
		ptr := v.Addr()
		if ptr.CanSet() {
			ptr.Elem().Set(truncated)
		}
	}
}

// sliceAt 返回 slice 第 i 个元素
func sliceAt(dest any, i int) any {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Slice {
		return nil
	}
	if i < 0 || i >= v.Len() {
		return nil
	}
	return v.Index(i).Interface()
}
