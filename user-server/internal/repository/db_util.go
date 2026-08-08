package repository

import (
	"errors"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// 不引入 github.com/jackc/pgerrcode 依赖，直接用字面量
const pgUniqueViolationCode = "23505"

// isDuplicateKeyErr 判断是否为 PG 唯一约束冲突
//
// 兼容两条路径：
//  1. GORM TranslateError 模式：gorm.ErrDuplicatedKey
//  2. 原生 lib/pq：*pq.Error with code 23505 (unique_violation)
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == pgUniqueViolationCode
	}
	return false
}
