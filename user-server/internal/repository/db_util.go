package repository

import (
	"errors"

	"github.com/lib/pq"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// 不引入 github.com/jackc/pgerrcode 依赖，直接用字面量
const pgUniqueViolationCode = "23505"

// isDuplicateKeyErr 判断是否为 PG 唯一约束冲突
//
// 兼容三条路径：
//  1. GORM TranslateError 模式：gorm.ErrDuplicatedKey
//  2. 原生 lib/pq：*pq.Error with code 23505 (unique_violation)
//  3. pgx 驱动（gorm.io/driver/postgres 底层）：*pgconn.PgError with Code 23505
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	// pgx 驱动路径（本项目实际使用的驱动）
	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		return pgxErr.Code == pgUniqueViolationCode
	}
	// lib/pq 路径（历史/兼容）
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == pgUniqueViolationCode
	}
	return false
}

// IsDuplicateKeyErr 导出版本：供跨包（如 service 层）判断 PG 唯一约束冲突。
// 内部复用 isDuplicateKeyErr 的两条路径兼容逻辑（GORM ErrDuplicatedKey / lib/pq 23505）。
func IsDuplicateKeyErr(err error) bool {
	return isDuplicateKeyErr(err)
}
