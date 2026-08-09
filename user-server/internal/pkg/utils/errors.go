package utils

import "errors"

// ErrInvalidInput 表示请求参数/业务输入校验失败。
// controller 层可用 response.ErrorFromDB 将其映射为 HTTP 400，
// 避免把「缺必填字段」之类的可预期错误误报为 500。
// service 层在返回此类错误时应以 %w 包裹本哨兵。
var ErrInvalidInput = errors.New("invalid input")

// ErrUnauthorized 表示鉴权/凭证校验失败（缺失、无效、已禁用、已过期等）。
// controller 层可用 response.ErrorFromDB 将其映射为 HTTP 401，
// 避免把「token 缺失/无效」之类的可预期错误误报为 500。
// service 层在返回此类错误时应以 %w 包裹本哨兵。
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden 表示已鉴权但缺少相应权限（越权、无 scope 等）。
// controller 层可用 response.ErrorFromDB 将其映射为 HTTP 403。
// service 层在返回此类错误时应以 %w 包裹本哨兵。
var ErrForbidden = errors.New("forbidden")
