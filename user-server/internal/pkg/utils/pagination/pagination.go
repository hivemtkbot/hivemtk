// Package pagination 提供统一的分页参数解析与校验。
//
// 设计动机：
//   - 历史 controller 普遍使用 `page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))`
//     忽略解析错误，且未校验边界，导致 page=0 / page=-1 / page_size=100000 等
//     异常输入直接进入 GORM Offset/Limit，引发负数 Offset SQL 错误或大结果集 OOM。
//   - R9 修复：集中校验 page>=1、pageSize 在 [1, MaxPageSize]，并兼容
//     page_size / pageSize / limit 三种命名约定。
//
// 用法：
//
//	page, pageSize, err := pagination.Parse(ctx)
//	if err != nil {
//	    response.Error(ctx, 400, err.Error())
//	    return
//	}
package pagination

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// MaxPageSize 单页最大条数，防止 OOM 与 DoS。
const MaxPageSize = 100

// DefaultPageSize 默认每页条数。
const DefaultPageSize = 20

// DefaultPage 默认页码。
const DefaultPage = 1

// ErrInvalidPage 表示 page 参数非法（非正整数或解析失败）。
var ErrInvalidPage = errors.New("page 参数必须为正整数")

// ErrInvalidPageSize 表示 page_size/limit 参数非法（超出 [1, MaxPageSize] 范围或解析失败）。
var ErrInvalidPageSize = fmt.Errorf("page_size 参数必须为 1-%d 之间的整数", MaxPageSize)

// Parse 从 gin.Context 解析分页参数。
//
// 兼容以下 query 字段：
//   - page：页码，默认 1，必须 >= 1
//   - page_size / pageSize / limit：每页条数，默认 20，必须 [1, MaxPageSize]
//
// 解析失败或越界时返回对应错误，调用方应返回 400。
func Parse(c *gin.Context) (page, pageSize int, err error) {
	return ParseWithMax(c, MaxPageSize)
}

// ParseWithMax 允许调用方指定更严格的 pageSize 上限（仍受 MaxPageSize 全局上限约束）。
//
// 例如某些重型查询可传入 50 限制单页不超过 50 条。
func ParseWithMax(c *gin.Context, maxPageSize int) (page, pageSize int, err error) {
	if maxPageSize <= 0 || maxPageSize > MaxPageSize {
		maxPageSize = MaxPageSize
	}

	page, err = parsePositiveInt(c.Query("page"), DefaultPage)
	if err != nil {
		return 0, 0, ErrInvalidPage
	}

	// 兼容三种命名：page_size（snake）、pageSize（camel）、limit（短名）
	raw := c.Query("page_size")
	if raw == "" {
		raw = c.Query("pageSize")
	}
	if raw == "" {
		raw = c.Query("limit")
	}
	pageSize, err = parsePositiveInt(raw, DefaultPageSize)
	if err != nil || pageSize > maxPageSize {
		return 0, 0, ErrInvalidPageSize
	}

	return page, pageSize, nil
}

// ParseOffset 是 Parse 的便捷变体，直接返回 Offset 与 Limit。
//
// 用于 GORM 链式调用：
//
//	offset, limit, err := pagination.ParseOffset(ctx)
//	if err != nil { ... }
//	db.Offset(offset).Limit(limit).Find(&items)
func ParseOffset(c *gin.Context) (offset, limit int, err error) {
	page, pageSize, err := Parse(c)
	if err != nil {
		return 0, 0, err
	}
	return (page - 1) * pageSize, pageSize, nil
}

// parsePositiveInt 解析字符串为正整数，空字符串返回 defaultVal。
func parsePositiveInt(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return 0, fmt.Errorf("invalid positive int: %q", s)
	}
	return v, nil
}
