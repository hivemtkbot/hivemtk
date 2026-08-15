package trace_learning

import (
	"context"
	"encoding/json"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// AggregateTrace 从 message_trace 聚合一条 trace 的评估素材。
//
// 取该 trace 下所有节点，提取：
//   - Query：ingest 节点的 input.content（兜底从 message_hub 取最近 inbound content）
//   - Reply：ai_dispatch 节点的 output.reply（由 webhook 埋点记录）
//   - RecalledChunkIDs：ai_dispatch 节点的 output.recalled_chunk_ids（自学习关联）
//   - HasAbnormal：是否存在异常节点
func AggregateTrace(ctx context.Context, db *gorm.DB, traceID string) (*AggregatedTrace, error) {
	ctx = ensureCtx(ctx)
	var spans []model.MessageTrace
	if err := db.WithContext(ctx).Where("trace_id = ?", traceID).Order("node_order, id").Find(&spans).Error; err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, nil
	}
	agg := &AggregatedTrace{TraceID: traceID}
	for _, s := range spans {
		if agg.ConversationID == "" {
			agg.ConversationID = s.ConversationID
		}
		if agg.Channel == "" {
			agg.Channel = s.Channel
		}
		if s.Status == "abnormal" {
			agg.HasAbnormal = true
		}
		switch s.Node {
		case "ingest":
			agg.Query = extractContent(s.Input)
		case "ai_dispatch":
			out := map[string]any{}
			_ = json.Unmarshal([]byte(s.Output), &out)
			if r, ok := out["reply"].(string); ok {
				agg.Reply = r
			}
			if rc, ok := out["recalled_chunk_ids"].([]any); ok {
				for _, v := range rc {
					if id, ok := v.(string); ok && id != "" {
						agg.RecalledChunkIDs = append(agg.RecalledChunkIDs, id)
					}
				}
			}
		}
	}
	agg.RecalledChunkIDs = dedupeStrings(agg.RecalledChunkIDs)
	if agg.Query == "" && agg.ConversationID != "" {
		var content string
		if err := db.WithContext(ctx).Table("message_hub").
			Where("conversation_id = ? AND direction = 'inbound'", agg.ConversationID).
			Order("id DESC").Limit(1).Pluck("content", &content).Error; err != nil {
			logger.Warnf("[trace_learning] 兜底查询 inbound 失败 conv=%s: %v", agg.ConversationID, err)
		}
		agg.Query = content
	}
	if agg.Reply == "" && agg.ConversationID != "" {
		var content string
		if err := db.WithContext(ctx).Table("message_hub").
			Where("conversation_id = ? AND direction = 'outbound'", agg.ConversationID).
			Order("id DESC").Limit(1).Pluck("content", &content).Error; err != nil {
			logger.Warnf("[trace_learning] 兜底查询 outbound 失败 conv=%s: %v", agg.ConversationID, err)
		}
		agg.Reply = content
	}
	return agg, nil
}

// dedupeStrings 去重并保持顺序
func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// extractContent 从 ingest 节点的 input(JSON) 提取 content 字段
func extractContent(input string) string {
	if input == "" {
		return ""
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		return ""
	}
	if c, ok := m["content"].(string); ok {
		return c
	}
	return ""
}

