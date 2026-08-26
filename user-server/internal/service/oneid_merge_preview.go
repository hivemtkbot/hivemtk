package service

import (
	"context"
	"sort"
	"strings"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// MergePreviewRule 预览请求中的单条合并规则
//
// 契约对齐前端 MergeRuleConfig.vue 的扁平规则结构：
// {id, name, type, fields:[], threshold, priority, action, enabled}
// 后端仅消费与合并判定相关的字段（fields/op/threshold/enabled）。
type MergePreviewRule struct {
	Name      string   `json:"name"`
	Field     string   `json:"field"`  // 兼容单字段写法
	Fields    []string `json:"fields"` // 前端 checkbox 多选字段
	Op        string   `json:"op"`     // eq / prefix / like；空视为 eq
	Threshold int      `json:"threshold"`
	Enabled   bool     `json:"enabled"`
}

// MergePreviewSample 一条候选合并对样例
type MergePreviewSample struct {
	From  string `json:"from"` // 客户 A 的 OneID（customer id）
	To    string `json:"to"`   // 客户 B 的 OneID
	Score int    `json:"score"`
}

// MergePreview 预览结果
type MergePreview struct {
	CandidateCount int                  `json:"candidateCount"`
	Samples        []MergePreviewSample `json:"samples"`
}

// previewSampleLimit 样例条数上限（前端表格 max-height 240，20 条足够）
const previewSampleLimit = 20

// identityFieldValue 取客户在指定身份字段上的值。
// 返回空串表示该客户不持有此身份（跳过）。
func identityFieldValue(c *model.Customer, field string) string {
	switch field {
	case "phone":
		return c.Phone
	case "email":
		return c.Email
	case "wechat_open_id", "openid":
		return c.WechatOpenID
	case "douyin_id", "douyin_open_id":
		return c.DouyinOpenID
	case "xiaohongshu_id":
		return c.XiaohongshuID
	default:
		// external_id / unionid / nickname 等暂无对应列，预览阶段跳过
		return ""
	}
}

// PreviewMergeRules 预览「按当前规则会合并哪些身份」：
// 拉取全量客户，按每条启用规则的匹配字段分组，
// 同组（值相等或前缀相同）≥2 个客户即产生候选合并对。
// candidateCount 为去重后的候选对总数；samples 最多返回 20 条。
func (s *OneIDMergeRuleService) PreviewMergeRules(ctx context.Context, rules []MergePreviewRule) (*MergePreview, error) {
	repo := repository.NewCustomerRepository()
	const pageSize = 500
	customers := make([]*model.Customer, 0, pageSize*2)
	for offset := 0; ; offset += pageSize {
		batch, _, err := repo.List(ctx, offset/pageSize+1, pageSize, "")
		if err != nil {
			logger.Warnf("[OneID preview] 拉取客户失败 offset=%d: %v", offset, err)
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		customers = append(customers, batch...)
		if len(batch) < pageSize {
			break
		}
	}

	pairScore := make(map[string]int)
	addPair := func(a, b *model.Customer, score int) {
		if a.ID == b.ID || a.ID == "" || b.ID == "" {
			return
		}
		x, y := a.ID, b.ID
		if x > y {
			x, y = y, x
		}
		key := x + "|" + y
		if score > pairScore[key] {
			pairScore[key] = score
		}
	}

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		fields := r.Fields
		if len(fields) == 0 && r.Field != "" {
			fields = []string{r.Field}
		}
		score := r.Threshold
		if score <= 0 {
			score = 100
		}
		op := r.Op
		if op == "" {
			op = "eq"
		}
		for _, f := range fields {
			groups := make(map[string][]*model.Customer)
			for _, c := range customers {
				v := identityFieldValue(c, f)
				if v == "" {
					continue
				}
				key := v
				if op == "prefix" {
					if len(v) < 7 {
						continue
					}
					key = v[:7]
				}
				groups[key] = append(groups[key], c)
			}
			for _, owners := range groups {
				if len(owners) < 2 {
					continue
				}
				for i := 0; i < len(owners); i++ {
					for j := i + 1; j < len(owners); j++ {
						addPair(owners[i], owners[j], score)
					}
				}
			}
		}
	}

	keys := make([]string, 0, len(pairScore))
	for k := range pairScore {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	samples := make([]MergePreviewSample, 0, previewSampleLimit)
	for _, k := range keys {
		if len(samples) >= previewSampleLimit {
			break
		}
		parts := strings.SplitN(k, "|", 2)
		samples = append(samples, MergePreviewSample{From: parts[0], To: parts[1], Score: pairScore[k]})
	}
	return &MergePreview{CandidateCount: len(pairScore), Samples: samples}, nil
}
