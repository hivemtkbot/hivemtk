package controller

import "strings"

// isNotFoundError 判断错误是否表示记录不存在
//
// P2-3：原寄居在共享 internal/controller 的 IsNotFoundError 唯一使用方即本包，
// 下沉为本包私有实现，切断 aiagent→controller 反向依赖。
// 使用字符串匹配而非 gorm.ErrRecordNotFound，避免依赖 gorm。
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "不存在") || strings.Contains(msg, "not found")
}

