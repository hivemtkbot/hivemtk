package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const pgUniqueViolationCode = "23505"

func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		return pgxErr.Code == pgUniqueViolationCode
	}

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
