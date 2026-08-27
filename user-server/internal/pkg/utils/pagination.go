// Package utils - 分页参数解析（顶层 utils 公开 API）。
//
// 与 internal/pkg/utils/pagination 子包的区别：
//   - 子包 pagination.Parse / ParseWithMax 提供硬上限（100）校验，超限直接 400
//   - 本 ParsePagination 提供函数式选项（WithDefaultSize / WithMaxSize / WithMinSize / WithAllowOverMax），
//     管理端可传 WithMaxSize(1000) 临时放大，配合 WithAllowOverMax(true) 自动 clamp 而不是报错
//
// 用法（默认行为，与 pagination 子包一致）：
//
//	page, pageSize, err := utils.ParsePagination(c)
//	if err != nil {
//	    response.Error(c, 400, err.Error())
//	    return
//	}
//
// 管理端 1000 条：
//
//	page, pageSize, err := utils.ParsePagination(c,
//	    utils.WithMaxSize(1000),
//	    utils.WithAllowOverMax(true),
//	)
package utils

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 分页参数默认值
const (
	defaultDefaultPageSize = 20
	defaultMaxPageSize     = 200 // 全局硬上限（用户端）
	defaultMinPageSize     = 1
)

// 公开常用 PageOption 的具体值供调用方一眼看出语义
const (
	// MaxPageSizeAdmin 管理端常见的单页条数，配合 WithMaxSize + WithAllowOverMax 使用
	MaxPageSizeAdmin = 1000
	// MaxPageSizeUser 用户端默认硬上限（与 pagination 子包 MaxPageSize=100 解耦，独立值）
	MaxPageSizeUser = 200
)

// PageOption 配置 ParsePagination 的行为。
//
// 通过函数式选项组合可在不破坏调用方的前提下扩展行为（vs. 加参数列）。
type PageOption func(*pageConfig)

// pageConfig 内部配置；通过 PageOption 函数修改。
type pageConfig struct {
	defaultSize   int
	maxSize       int
	minSize       int
	allowOverMax  bool
	pageAlias     string
	pageSizeAlias string
}

func newDefaultConfig() *pageConfig {
	return &pageConfig{
		defaultSize:   defaultDefaultPageSize,
		maxSize:       defaultMaxPageSize,
		minSize:       defaultMinPageSize,
		allowOverMax:  false,
		pageAlias:     "page",
		pageSizeAlias: "page_size",
	}
}

// WithDefaultSize 设置 pageSize 默认值（当请求未传时生效）。
func WithDefaultSize(n int) PageOption {
	return func(c *pageConfig) {
		if n > 0 {
			c.defaultSize = n
		}
	}
}

// WithMaxSize 设置 pageSize 上限（推荐：用户端 200，管理端 1000）。
//
// 与 allowOverMax 的关系：
//   - allowOverMax=false：超过上限 → 返回 ErrInvalidPageSize（响应 400）
//   - allowOverMax=true：  超过上限 → clamp 到上限值（不报错）
func WithMaxSize(n int) PageOption {
	return func(c *pageConfig) {
		if n > 0 {
			c.maxSize = n
		}
	}
}

// WithMinSize 设置 pageSize 下限。请求 pageSize < minSize 时钳制到 minSize（不报错）。
//
// 与 page<1 行为不同：
//   - page<1 → 钳制到 1（不报错，认为是"默认首页"）
//   - pageSize<minSize → 钳制到 minSize（不报错，认为是"默认值太离谱，强制拉回"）
func WithMinSize(n int) PageOption {
	return func(c *pageConfig) {
		if n > 0 {
			c.minSize = n
		}
	}
}

// WithAllowOverMax 是否允许 pageSize 超过 maxSize。
//
// false（默认）：超过 → 返回 ErrInvalidPageSize（响应 400）
// true：超过 → 自动 clamp 到 maxSize
func WithAllowOverMax(b bool) PageOption {
	return func(c *pageConfig) {
		c.allowOverMax = b
	}
}

// withPageAlias 与 withPageSizeAlias 允许在解析时支持多种 query 字段名。
// 当前实现优先读取 page / page_size，未命中时回退到 pageSize / limit。
//
// 暴露这两个 Option 是为了未来在不破坏现有调用方的前提下扩展（例如支持 cursor）。
func withPageAlias(name string) PageOption {
	return func(c *pageConfig) {
		if name != "" {
			c.pageAlias = name
		}
	}
}

func withPageSizeAlias(name string) PageOption {
	return func(c *pageConfig) {
		if name != "" {
			c.pageSizeAlias = name
		}
	}
}

// ErrInvalidPageSize 表示 page_size 参数非法（解析失败 / 超出允许范围且未开启 clamp）。
//
// 错误信息中包含实际 maxSize，方便调用方直接透传给前端展示。
type errInvalidPageSize struct {
	maxSize int
}

func (e *errInvalidPageSize) Error() string {
	return fmt.Sprintf("page_size 超出允许范围 (最大 %d)", e.maxSize)
}

// IsInvalidPageSize 检查 err 是否为 ErrInvalidPageSize。
//
// 用法：
//
//	if utils.IsInvalidPageSize(err) {
//	    response.Error(c, 400, err.Error())
//	    return
//	}
func IsInvalidPageSize(err error) bool {
	var target *errInvalidPageSize
	return errors.As(err, &target)
}

// ParsePagination 从 gin.Context 解析 page / page_size，返回 (page, pageSize, error)。
//
// 行为规范（与文档一致）：
//   - page 非整数或缺失 → 1（不报错，宽容前端）
//   - page < 1         → 1（同上）
//   - pageSize 缺失     → defaultSize
//   - pageSize 非整数   → defaultSize（不报错，宽容前端）
//   - pageSize < minSize → minSize（钳制，不报错）
//   - pageSize > maxSize：
//       - allowOverMax=false → 返回 *errInvalidPageSize（响应 400）
//       - allowOverMax=true  → clamp 到 maxSize（不报错）
//
// 兼容以下 query 字段：
//   - page: pageAlias（默认 "page"）
//   - pageSize: pageSizeAlias（默认 "page_size"），回退 "pageSize" / "limit"
//
// 返回 error 时调用方应按现有 {code:400, message:"参数错误"} 响应包装。
func ParsePagination(c *gin.Context, opts ...PageOption) (page, pageSize int, err error) {
	cfg := newDefaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// 防御：maxSize 必须 >= defaultSize >= minSize
	if cfg.maxSize < cfg.minSize {
		cfg.maxSize = cfg.minSize
	}
	if cfg.defaultSize < cfg.minSize {
		cfg.defaultSize = cfg.minSize
	}
	if cfg.defaultSize > cfg.maxSize {
		cfg.defaultSize = cfg.maxSize
	}

	// 解析 page：宽容策略
	page = parsePage(c.Query(cfg.pageAlias))
	if page < 1 {
		page = 1
	}

	// 解析 pageSize：兼容 page_size / pageSize / limit
	raw := c.Query(cfg.pageSizeAlias)
	if raw == "" {
		raw = c.Query("pageSize")
	}
	if raw == "" {
		raw = c.Query("limit")
	}
	pageSize, ok := parsePageSizeStrict(raw)
	if !ok {
		// 字符串非整数 / 空 → fallback 到 defaultSize（宽容策略）
		pageSize = cfg.defaultSize
	}
	// 注意：parsePageSizeStrict 永远返回 >= 1，所以"pageSize < minSize"的钳制
	// 只在 minSize > 1 时生效（管理端显式调高最小值）。
	if pageSize < cfg.minSize {
		pageSize = cfg.minSize
	}
	// 钳制/拒绝上限
	if pageSize > cfg.maxSize {
		if cfg.allowOverMax {
			pageSize = cfg.maxSize
		} else {
			return 0, 0, &errInvalidPageSize{maxSize: cfg.maxSize}
		}
	}

	return page, pageSize, nil
}

// parsePage 解析 page 字符串。非整数 / 空 / 负数 / 0 全部视为 1。
func parsePage(s string) int {
	if s == "" {
		return 1
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 1
	}
	if v < 1 {
		return 1
	}
	return v
}

// parsePageSizeStrict 严格解析 pageSize 字符串。
//
// 返回值：
//   - (n, true)：n >= 1（任何正整数）
//   - (0, false)：字符串为空 / 非整数 / <= 0 —— 调用方应按 fallback 处理
func parsePageSizeStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return 0, false
	}
	return v, true
}

// ParsePaginationOffset 是 ParsePagination 的便捷变体，返回 Offset 与 Limit（供 GORM 调用）。
//
// 用法：
//
//	offset, limit, err := utils.ParsePaginationOffset(c)
//	if err != nil { ... }
//	db.Offset(offset).Limit(limit).Find(&items)
func ParsePaginationOffset(c *gin.Context, opts ...PageOption) (offset, limit int, err error) {
	page, pageSize, err := ParsePagination(c, opts...)
	if err != nil {
		return 0, 0, err
	}
	return (page - 1) * pageSize, pageSize, nil
}