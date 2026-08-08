package trace_learning

import (
	"context"
	"encoding/json"

	"marketing/internal/model"
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
					if id, ok := v.(string); ok {
						agg.RecalledChunkIDs = append(agg.RecalledChunkIDs, id)
					}
				}
			}
		}
	}
	// 兜底：若 ingest 未记录 query，从 message_hub 取最近 inbound content
	if agg.Query == "" && agg.ConversationID != "" {
		var content string
		db.WithContext(ctx).Table("message_hub").
			Where("conversation_id = ? AND direction = 'inbound'", agg.ConversationID).
			Order("id DESC").Limit(1).Pluck("content", &content)
		agg.Query = content
	}
	// 兜底：若 ai_dispatch 未记录 reply 文本，从 message_hub 取最近 outbound（AI 真实回复）
	if agg.Reply == "" && agg.ConversationID != "" {
		var content string
		db.WithContext(ctx).Table("message_hub").
			Where("conversation_id = ? AND direction = 'outbound'", agg.ConversationID).
			Order("id DESC").Limit(1).Pluck("content", &content)
		agg.Reply = content
	}
	return agg, nil
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
