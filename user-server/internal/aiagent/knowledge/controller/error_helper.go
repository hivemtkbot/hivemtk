package controller

import "strings"

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "不存在") || strings.Contains(msg, "not found")
}
