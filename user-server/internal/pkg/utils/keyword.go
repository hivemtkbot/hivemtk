package utils

import "strings"

// ContainsKeyword 检查消息是否包含关键词列表中的任一关键词(逗号分隔)
func ContainsKeyword(message string, keywords string) bool {
	keywordList := strings.Split(keywords, ",")
	for _, keyword := range keywordList {
		if strings.Contains(message, strings.TrimSpace(keyword)) {
			return true
		}
	}
	return false
}
