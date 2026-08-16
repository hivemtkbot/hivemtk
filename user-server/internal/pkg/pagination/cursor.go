// Package pagination 提供 cursor-based 分页工具（OPT-ARC-10）
//
// 背景：传统 offset+limit 在大表深翻页时性能差（OFFSET N 需要扫描前 N 行）
// 解决：使用基于最后一条记录主键的 cursor-based 分页
// 优势：
//   - 性能稳定：O(log N) 索引扫描，与翻页深度无关
//   - 一致性：避免 offset 期间新增数据导致的重复/漏读
//   - 安全：避免 max(limit) 过大导致全表扫描
//
// 用法示例（service 层）：
//   func ListCustomer(ctx, req CursorReq) (*CursorResp, error) {
//       items, nextCursor, err := pagination.CursorQuery(ctx, db, ...)
//       return &CursorResp{Items: items, NextCursor: nextCursor}, nil
//   }
package pagination

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CursorPageSize 单页最大行数（与原 MaxLimit 对齐 = 100，OPT-ARC-11）
const CursorPageSize = 100

// Cursor 编码后的游标（base64 + 时间戳 + ID）
type Cursor string

// EncodeCursor 将 (timestamp, id) 编码为 base64 游标
// 格式：<unix_nano>:<id>
//   例如：1700000000000000000:12345
func EncodeCursor(ts time.Time, id uint64) Cursor {
	raw := fmt.Sprintf("%d:%d", ts.UnixNano(), id)
	return Cursor(base64.URLEncoding.EncodeToString([]byte(raw)))
}

// DecodeCursor 解码游标
// 返回 (timestamp, id, ok)
func DecodeCursor(c Cursor) (time.Time, uint64, bool) {
	if c == "" {
		return time.Time{}, 0, false
	}
	raw, err := base64.URLEncoding.DecodeString(string(c))
	if err != nil {
		return time.Time{}, 0, false
	}
	var (
		tsNano int64
		id     uint64
	)
	parts := splitBytes(raw, ':')
	if len(parts) != 2 {
		return time.Time{}, 0, false
	}
	if _, err := fmt.Sscanf(string(parts[0]), "%d", &tsNano); err != nil {
		return time.Time{}, 0, false
	}
	if _, err := fmt.Sscanf(string(parts[1]), "%d", &id); err != nil {
		return time.Time{}, 0, false
	}
	return time.Unix(0, tsNano), id, true
}

// splitBytes 分割 []byte by sep
func splitBytes(b []byte, sep byte) [][]byte {
	var result [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == sep {
			result = append(result, b[start:i])
			start = i + 1
		}
	}
	result = append(result, b[start:])
	return result
}

// CursorQueryOpts cursor-based 查询参数
type CursorQueryOpts struct {
	// DB 原始表
	Table string
	// 游标（首次传空）
	Cursor Cursor
	// 单页大小
	PageSize int
	// 排序字段（默认 created_at DESC, id DESC）
	OrderBy string
	// 过滤条件（可选的 WHERE 子句）
	Where map[string]any
	// 结果集
	Dest any
}

// CursorQueryResult cursor-based 查询结果
type CursorQueryResult struct {
	Items      any
	NextCursor Cursor
	HasMore    bool
}

// CursorQuery 执行 cursor-based 分页查询
// 适用场景：created_at + id 唯一索引的大表
//
// 实现：
//   1. 首次查询：WHERE 过滤 + ORDER BY ts DESC, id DESC + LIMIT N+1
//   2. 取最后一条的 (ts, id) 作为 nextCursor
//   3. 后续查询：WHERE (ts, id) < (cursor.ts, cursor.id) + LIMIT N+1
//   4. 比 N 多取 1 条用于判断 hasMore
func CursorQuery(ctx context.Context, db *gorm.DB, opts CursorQueryQuery) (*CursorQueryResult, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 || pageSize > CursorPageSize {
		pageSize = CursorPageSize
	}

	// 构造基础查询
	q := db.WithContext(ctx).Table(opts.Table)

	// 过滤
	for k, v := range opts.Where {
		q = q.Where(k, v)
	}

	// 排序
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "created_at DESC, id DESC"
	}
	q = q.Order(orderBy)

	// 游标过滤
	if opts.Cursor != "" {
		ts, id, ok := DecodeCursor(opts.Cursor)
		if !ok {
			return nil, fmt.Errorf("invalid cursor: %s", opts.Cursor)
		}
		// 主键 (ts, id) 严格小于 cursor
		// 适用 ORDER BY ts DESC, id DESC
		q = q.Where("(created_at, id) < (?, ?)", ts, id)
	}

	// 多取 1 条用于判断 hasMore
	if err := q.Limit(pageSize + 1).Find(opts.Dest).Error; err != nil {
		return nil, err
	}

	// 判定 hasMore 并计算 nextCursor
	// 通过 reflect 检查 len(dest slice)
	count := sliceLen(opts.Dest)
	hasMore := count > pageSize
	if hasMore {
		// 截断到 pageSize
		sliceTruncate(opts.Dest, pageSize)
		count = pageSize
	}

	var nextCursor Cursor
	if hasMore && count > 0 {
		// 取最后一条
		lastItem := sliceAt(opts.Dest, count-1)
		nextCursor = extractCursorFromItem(lastItem)
	}

	return &CursorQueryResult{
		Items:      opts.Dest,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CursorQueryQuery cursor-based 查询参数（兼容 gorm.DB）
type CursorQueryQuery struct {
	Table   string
	Cursor  Cursor
	PageSize int
	OrderBy string
	Where   map[string]any
	Dest    any
}

// IsValidLimit 校验 limit 值（OPT-ARC-11）
func IsValidLimit(limit int) bool {
	return limit > 0 && limit <= CursorPageSize
}

// ClampLimit 限制 limit 在 [1, CursorPageSize] 范围内
func ClampLimit(limit int) int {
	if limit <= 0 {
		return CursorPageSize
	}
	if limit > CursorPageSize {
		return CursorPageSize
	}
	return limit
}


