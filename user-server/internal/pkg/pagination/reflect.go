package pagination

import "reflect"

func sliceLen(dest any) int {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Slice {
		return 0
	}
	return v.Len()
}

func sliceTruncate(dest any, n int) {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Slice {
		return
	}
	if v.Len() <= n {
		return
	}

	truncated := v.Slice(0, n)

	if v.CanSet() {
		v.Set(truncated)
	} else if v.CanAddr() {

		ptr := v.Addr()
		if ptr.CanSet() {
			ptr.Elem().Set(truncated)
		}
	}
}

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
