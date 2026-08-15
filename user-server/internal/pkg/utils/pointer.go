package utils

// BoolPtr 将 bool 转换为 *bool
func BoolPtr(b bool) *bool {
	return &b
}

// GetBoolValue 获取指针的 bool 值,如果为 nil 则返回默认值
func GetBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

