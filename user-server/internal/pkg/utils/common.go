package utils

import "strings"

// BoolPtr 将bool转换为*bool
func BoolPtr(b bool) *bool {
	return &b
}

// GetBoolValue 获取指针的bool值，如果为nil则返回默认值
func GetBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// ContainsKeyword 检查消息是否包含关键词
func ContainsKeyword(message string, keywords string) bool {
	keywordList := strings.Split(keywords, ",")
	for _, keyword := range keywordList {
		if strings.Contains(message, strings.TrimSpace(keyword)) {
			return true
		}
	}
	return false
}
