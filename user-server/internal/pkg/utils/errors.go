package utils

import "errors"

// ErrInvalidInput 表示请求参数/业务输入校验失败。
// controller 层可用 response.ErrorFromDB 将其映射为 HTTP 400，
// 避免把「缺必填字段」之类的可预期错误误报为 500。
// service 层在返回此类错误时应以 %w 包裹本哨兵。
var ErrInvalidInput = errors.New("invalid input")
