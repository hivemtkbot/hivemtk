package repository

// champion_dialogue.go 反馈闭环 - 销冠对话域仓储方法
//
// 五层架构归属: L3 仓储层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4.2
//
// 职责：封装 champion_dialogues 表的写入（含 pgvector 原生 SQL）
//        + script_templates 表的回流写入 + feedback_signals 聚合查询
//
// 说明：本文件方法挂载在 FeedbackLoopRepository 上（与 feedback_loop_repository.go 同结构体），
//        按业务域拆分文件便于维护，不引入新的仓储结构体。

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
)

// ChampionDialogueRow 销冠对话候选行（feedback_signals + feedback_events 聚合快照）
//
// 由 FetchChampionDialogueCandidates 通过 Raw SQL 填充，
// 字段顺序与 SQL SELECT 列对应。
type ChampionDialogueRow struct {
	SessionID   string
	CustomerID  string
	CustomerMsg string
	AIReply     string
	Reward      float64
	IsChampion  bool
	Scenario    string
	CreatedAt   time.Time
}

// FetchChampionDialogueCandidates 从 feedback_signals 拉取高价值候选对话
//
// SQL：聚合 reward + 关联最近一条 feedback_events 取 customer_msg / ai_reply 快照
// 按 aggregated_reward DESC 排序，取最多 maxDialogues 条
func (r *FeedbackLoopRepository) FetchChampionDialogueCandidates(ctx context.Context, scenario string, minReward float64, since time.Time, maxDialogues int) ([]ChampionDialogueRow, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []ChampionDialogueRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT fs.session_id, fs.customer_id,
		       (SELECT customer_msg FROM feedback_events WHERE session_id = fs.session_id ORDER BY created_at DESC LIMIT 1) AS customer_msg,
		       (SELECT ai_reply FROM feedback_events WHERE session_id = fs.session_id ORDER BY created_at DESC LIMIT 1) AS ai_reply,
		       fs.aggregated_reward AS reward, fs.is_champion,
		       COALESCE(fs.signal_breakdown->>'scenario', ?) AS scenario,
		       fs.created_at
		FROM feedback_signals fs
		WHERE fs.aggregated_reward >= ? AND fs.created_at >= ?
		ORDER BY fs.aggregated_reward DESC
		LIMIT ?`,
		scenario, minReward, since, maxDialogues).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ChampionDialoguePersist 销冠对话持久化参数
//
// EmbeddingLiteral 为 pgvector 字面量 '[v1,v2,...]'，由调用方格式化
type ChampionDialoguePersist struct {
	Fingerprint        string
	SessionID          string
	CustomerID         string
	Scenario           string
	CustomerMsg        string
	ChampionReply      string
	EmbeddingLiteral   string // pgvector 字面量 '[v1,v2,...]'
	ClusterID          uint
	Reward             float64
	ConversionAchieved bool
}

// PersistChampionDialogue 持久化销冠对话到 champion_dialogues
//
// 用原生 SQL 写入（embedding vector 类型 GORM 不支持，用 ?::vector 强转）
// ON CONFLICT (dialogue_fingerprint) DO UPDATE：重复对话按 reward / conversion_achieved 覆盖
func (r *FeedbackLoopRepository) PersistChampionDialogue(ctx context.Context, p ChampionDialoguePersist) error {
	if r == nil || r.db == nil {
		return nil
	}
	sql := `
		INSERT INTO champion_dialogues
			(dialogue_fingerprint, session_id, customer_id, staff_id, staff_name,
			 scenario, journey_stage, customer_msg, champion_reply, context_msgs,
			 embedding, cluster_id, reward, conversion_achieved, extracted_scripts, created_at)
		VALUES (?, ?, ?, 0, '', ?, '', ?, ?, '{}', ?::vector, ?, ?, ?, '[]', NOW())
		ON CONFLICT (dialogue_fingerprint) DO UPDATE SET
			reward = EXCLUDED.reward,
			conversion_achieved = EXCLUDED.conversion_achieved`
	return r.db.WithContext(ctx).Exec(sql,
		p.Fingerprint, p.SessionID, p.CustomerID,
		p.Scenario, p.CustomerMsg, p.ChampionReply,
		p.EmbeddingLiteral, p.ClusterID, p.Reward, p.ConversionAchieved).Error
}

// GetChampionDialogueIDByCluster 反查 cluster_id 对应的最新 champion_dialogue.id
//
// 用于 saveScriptsToTemplate 回流话术时关联销冠对话 ID
// 未找到时返回 (0, nil)（不视为错误，调用方按需处理）
func (r *FeedbackLoopRepository) GetChampionDialogueIDByCluster(ctx context.Context, clusterID uint) (uint, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var dialogueID uint
	err := r.db.WithContext(ctx).Model(&model.ChampionDialogue{}).
		Where("cluster_id = ?", clusterID).
		Order("id DESC").
		Limit(1).
		Pluck("id", &dialogueID).Error
	return dialogueID, err
}

// InsertScriptTemplate 写入话术库 script_templates（原生 SQL）
//
// 用原生 SQL 写入：script_templates 模型在 model 包，本包不直接依赖该子包避免循环依赖
// Source 固定为 'champion_extract'，is_system=TRUE
//
// 参数：
//   - category, title, content, tags    - 话术元信息
//   - effectivenessScore                 - LLM 评分（0-1，调用方已做边界裁剪）
//   - journeyStage                       - 客户旅程阶段
//   - championDialogueID                 - 关联销冠对话 ID
func (r *FeedbackLoopRepository) InsertScriptTemplate(ctx context.Context, category, title, content, tags string, effectivenessScore float64, journeyStage string, championDialogueID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	sql := `
		INSERT INTO script_templates
			(category, title, content, tags, rating, is_system, variables,
			 source, effectiveness_score, trigger_keywords, journey_stage, champion_dialogue_id,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, TRUE, '[]',
		        'champion_extract', ?, ?, ?, ?,
		        NOW(), NOW())`
	return r.db.WithContext(ctx).Exec(sql,
		category, title, content, tags, effectivenessScore,
		effectivenessScore, tags, journeyStage, championDialogueID).Error
}
