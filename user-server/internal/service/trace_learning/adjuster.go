package trace_learning

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"marketing/internal/pkg/utils/logger"
	"gorm.io/gorm"
)

// AdjustedChunk 单次权重调整记录
type AdjustedChunk struct {
	ID  uint64  `json:"id"`
	Old float64 `json:"old"`
	New float64 `json:"new"`
}

// AdjustWeights 根据打分调整涉及 chunk 的权重。
//
// 规则：
//   - 差回复（bad 或 score<BadThreshold）→ 降权（weight *= Decay）
//   - 好回复（score>=GoodThreshold）→ 升权（weight *= Boost）
//   - 中间分 → 不调整
//
// 权重限制在 [MinWeight, MaxWeight]，用事务批量更新，幂等可重复运行。
func AdjustWeights(ctx context.Context, db *gorm.DB, chunkIDs []string, score int, cfg Config) ([]AdjustedChunk, error) {
	ctx = ensureCtx(ctx)
	ids := dedupeParseIDs(chunkIDs)
	if len(ids) == 0 || db == nil {
		return nil, nil
	}
	type wrow struct {
		ID     uint64
		Weight float64
	}
	var rows []wrow
	if err := db.WithContext(ctx).Table("knowledge_chunks").Select("id, weight").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	bad := score < cfg.BadThreshold
	good := score >= cfg.GoodThreshold
	adjusted := make([]AdjustedChunk, 0, len(rows))
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			oldW := r.Weight
			if oldW <= 0 {
				oldW = 1.0
			}
			var newW float64
			switch {
			case bad:
				newW = oldW * cfg.Decay
			case good:
				newW = oldW * cfg.Boost
			default:
				newW = oldW
			}
			newW = clampWeight(newW, cfg.MinWeight, cfg.MaxWeight)
			if newW == oldW {
				continue
			}
			if e := tx.Table("knowledge_chunks").Where("id = ?", r.ID).Update("weight", newW).Error; e != nil {
				return e
			}
			adjusted = append(adjusted, AdjustedChunk{ID: r.ID, Old: oldW, New: newW})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(adjusted) > 0 {
		logger.Infof("[trace_learning] score=%d 调整权重 chunks=%d adjusted=%d", score, len(rows), len(adjusted))
	}
	return adjusted, nil
}

// dedupeParseIDs 去重解析 chunk ID（string→uint64）
func dedupeParseIDs(ids []string) []uint64 {
	seen := map[uint64]struct{}{}
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		u, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64)
		if err == nil {
			if _, ok := seen[u]; !ok {
				seen[u] = struct{}{}
				out = append(out, u)
			}
		}
	}
	return out
}

// clampWeight 把权重限制在 [min,max]
func clampWeight(w, min, max float64) float64 {
	if w < min {
		return min
	}
	if w > max {
		return max
	}
	return w
}

// marshalJSON 安全序列化（忽略错误）
func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
