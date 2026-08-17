package repository


import (
	"context"
	"strings"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ClueQuery 线索查询条件。
// Type / IsVerify 用指针，nil 表示不参与过滤（区别于值为 0）。
type ClueQuery struct {
	Keyword       string
	Type          *int64
	IsVerify      *int64
	OwnerAccount  string
	IsOpportunity *int64
	IsGroup       *bool
	GroupID       string
	StartUnix     int64
	EndUnix       int64
}

// ClueTypeAgg 按类型聚合结果
type ClueTypeAgg struct {
	Type        int64 `gorm:"column:type"`
	Total       int64 `gorm:"column:total"`
	VerifyTotal int64 `gorm:"column:verify_total"`
}

// ClueTrendAgg 按天聚合结果
type ClueTrendAgg struct {
	Date     string `gorm:"column:date"`
	Count    int64  `gorm:"column:count"`
	Verified int64  `gorm:"column:verified"`
}

// applyClueQuery 把查询条件翻译为 WHERE 子句
func applyClueQuery(db *gorm.DB, q ClueQuery) *gorm.DB {
	tx := db
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + kw + "%"
		tx = tx.Where(
			"(name ILIKE ? OR account ILIKE ? OR city ILIKE ? OR address ILIKE ? OR COALESCE(owner_account,'') ILIKE ?)",
			like, like, like, like, like,
		)
	}
	if q.Type != nil {
		tx = tx.Where("type = ?", *q.Type)
	}
	if q.IsVerify != nil {
		if *q.IsVerify >= 1 {
			tx = tx.Where("COALESCE(is_verify, 0) >= 1")
		} else {
			tx = tx.Where("COALESCE(is_verify, 0) < 1")
		}
	}
	if q.OwnerAccount != "" {
		tx = tx.Where("owner_account = ?", q.OwnerAccount)
	}
	if q.IsOpportunity != nil {
		if *q.IsOpportunity >= 1 {
			tx = tx.Where("COALESCE(is_opportunity, 0) >= 1")
		} else {
			tx = tx.Where("COALESCE(is_opportunity, 0) < 1")
		}
	}
	if q.IsGroup != nil {
		tx = tx.Where("is_group = ?", *q.IsGroup)
	}
	if q.GroupID != "" {
		tx = tx.Where("group_id = ?", q.GroupID)
	}
	if q.StartUnix > 0 {
		tx = tx.Where("create_time >= ?", q.StartUnix)
	}
	if q.EndUnix > 0 {
		tx = tx.Where("create_time <= ?", q.EndUnix)
	}
	return tx
}

// ListWithQuery 按条件分页查询线索，按创建时间倒序
func (r *clueRepo) ListWithQuery(ctx context.Context, q ClueQuery, page, limit int) ([]*model.Clue, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	var total int64
	countTx := applyClueQuery(r.db.WithContext(ctx).Model(&model.Clue{}), q)
	if err := countTx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var clues []*model.Clue
	listTx := applyClueQuery(r.db.WithContext(ctx).Model(&model.Clue{}), q)
	if err := listTx.Order("create_time DESC").
		Offset((page - 1) * limit).Limit(limit).Find(&clues).Error; err != nil {
		return nil, 0, err
	}
	return clues, total, nil
}

// CountWithQuery 按条件统计线索数量
func (r *clueRepo) CountWithQuery(ctx context.Context, q ClueQuery) (int64, error) {
	var total int64
	tx := applyClueQuery(r.db.WithContext(ctx).Model(&model.Clue{}), q)
	err := tx.Count(&total).Error
	return total, err
}

// TypeDistribution 按线索类型聚合（真实来源分布）
func (r *clueRepo) TypeDistribution(ctx context.Context, q ClueQuery) ([]ClueTypeAgg, error) {
	var rows []ClueTypeAgg
	tx := applyClueQuery(r.db.WithContext(ctx).Model(&model.Clue{}), q)
	err := tx.Select("type AS type, COUNT(*) AS total, COALESCE(SUM(CASE WHEN COALESCE(is_verify,0) >= 1 THEN 1 ELSE 0 END), 0) AS verify_total").
		Group("type").Order("type").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// TrendByDay 按天聚合线索新增趋势（create_time 为秒级 unix，转本地日期分组）
func (r *clueRepo) TrendByDay(ctx context.Context, q ClueQuery) ([]ClueTrendAgg, error) {
	var rows []ClueTrendAgg
	tx := applyClueQuery(r.db.WithContext(ctx).Model(&model.Clue{}), q)
	// 注意：GORM 的 Group("1") 会被引号包裹成 GROUP BY "1"（PG 报 column "1" does not exist），
	// 因此必须传完整表达式让 GORM 按原样输出。
	const dayExpr = "to_char(to_timestamp(create_time), 'YYYY-MM-DD')"
	err := tx.Select(
		dayExpr + " AS date, " +
			"COUNT(*) AS count, " +
			"COALESCE(SUM(CASE WHEN COALESCE(is_verify,0) >= 1 THEN 1 ELSE 0 END), 0) AS verified",
	).Group(dayExpr).Order(dayExpr).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}


