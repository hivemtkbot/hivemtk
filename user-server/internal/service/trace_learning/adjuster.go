package trace_learning

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// AdjustedChunk 单次权重调整记录
type AdjustedChunk struct {
	ID  uint64  `json:"id"`
	Old float64 `json:"old"`
	New float64 `json:"new"`
}

func weightAction(score int, bad bool, safety float64, safetyPresent bool, cfg Config) string {
	isBad := bad || score < cfg.BadThreshold
	if safetyPresent && safety < 70 {
		isBad = true
	}
	if isBad {
		return "decay"
	}
	if !bad && score >= cfg.GoodThreshold {
		return "boost"
	}
	return "none"
}

// AdjustWeights 根据打分调整涉及 chunk 的权重。
//
// 规则：
//   - 差回复（bad 或 score<BadThreshold 或 safety<70）→ 降权（weight *= Decay）
//   - 好回复（!bad 且 score>=GoodThreshold）→ 升权（weight *= Boost）
//   - 中间分 → 不调整
//
// 权重限制在 [MinWeight, MaxWeight]，用事务批量更新。调整本身是幂等的（同一 chunk
// 重复评估时由调用方决定是否跳过，避免权重被反复乘算造成漂移）。
func AdjustWeights(ctx context.Context, db *gorm.DB, chunkIDs []string, res EvalResult, cfg Config) ([]AdjustedChunk, error) {
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
	safety, safetyPresent := res.Dimensions["safety"]
	action := weightAction(res.Score, res.Bad, safety, safetyPresent, cfg)
	adjusted := make([]AdjustedChunk, 0, len(rows))
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			oldW := r.Weight
			if oldW <= 0 {
				oldW = 1.0
			}
			newW := computeNewWeight(oldW, action, cfg)
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
		logger.Infof("[trace_learning] score=%d action=%s 调整权重 chunks=%d adjusted=%d", res.Score, action, len(rows), len(adjusted))
	}
	return adjusted, nil
}

// PreviewAdjustments 只读预览：读取当前权重并按打分计算「计划调整」，不写库。
// 用于 dry-run / 前端预览，评估自学习「将如何调权」而不实际改变知识库权重。
func PreviewAdjustments(ctx context.Context, db *gorm.DB, chunkIDs []string, res EvalResult, cfg Config) ([]AdjustedChunk, error) {
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
	safety, safetyPresent := res.Dimensions["safety"]
	action := weightAction(res.Score, res.Bad, safety, safetyPresent, cfg)
	adjusted := make([]AdjustedChunk, 0, len(rows))
	for _, r := range rows {
		oldW := r.Weight
		if oldW <= 0 {
			oldW = 1.0
		}
		newW := computeNewWeight(oldW, action, cfg)
		if newW == oldW {
			continue
		}
		adjusted = append(adjusted, AdjustedChunk{ID: r.ID, Old: oldW, New: newW})
	}
	return adjusted, nil
}

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

func clampWeight(w, min, max float64) float64 {
	if w < min {
		return min
	}
	if w > max {
		return max
	}
	return w
}

func computeNewWeight(oldW float64, action string, cfg Config) float64 {
	var acted float64
	switch action {
	case "decay":
		acted = oldW * cfg.Decay
	case "boost":
		acted = oldW * cfg.Boost
	default:
		acted = oldW
	}
	acted = clampWeight(acted, cfg.MinWeight, cfg.MaxWeight)
	reverted := cfg.getMeanReversion()*1.0 + (1-cfg.getMeanReversion())*acted
	return clampWeight(reverted, cfg.MinWeight, cfg.MaxWeight)
}

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
