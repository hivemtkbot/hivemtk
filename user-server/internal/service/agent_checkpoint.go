// Agent Runtime 阶段级 Checkpoint（D06，三表自研方案 v2——DBOS 试点评估后延后：
// 引入外部 durable 运行时需验证与现有连接池共存且团队无 Go durable 经验，
// 三表 schema（LangGraph Checkpointer 单表简化）+ 阶段边界恢复语义已满足五阶段流程）。
//
// 恢复纪律（LangGraph superstep 语义）：
//   - 只能从阶段边界恢复；阶段内失败整体重跑该阶段；
//   - 每 stage 完成 upsert (thread_id, stage, state)；
//   - 恢复 = LoadLatest(thread_id) → 从下一阶段继续；无记录则从感知阶段起跑。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AgentStageNames 五阶段顺序（恢复游标依据）
var AgentStageNames = []string{"perception", "alignment", "gatekeeper", "planner", "reviewer"}

// AgentCheckpointRepository checkpoint 存取
type AgentCheckpointRepository struct {
	db *gorm.DB
}

// NewAgentCheckpointRepository 构造
func NewAgentCheckpointRepository(db *gorm.DB) *AgentCheckpointRepository {
	return &AgentCheckpointRepository{db: db}
}

// Save 阶段完成即落 checkpoint（upsert，幂等）
func (r *AgentCheckpointRepository) Save(ctx context.Context, threadID, stage string, state json.RawMessage) error {
	if r.db == nil {
		return nil
	}
	if threadID == "" || stage == "" {
		return nil
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_checkpoints (thread_id, stage, state, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())
		ON CONFLICT (thread_id, stage) DO UPDATE SET state = EXCLUDED.state, updated_at = NOW()`,
		threadID, stage, string(state)).Error
}

// AgentCheckpoint 单条记录
type AgentCheckpoint struct {
	ThreadID  string
	Stage     string
	State     json.RawMessage
	UpdatedAt time.Time
}

// LoadLatest 取该 thread 最新 checkpoint（按 updated_at DESC），无记录返回 nil
func (r *AgentCheckpointRepository) LoadLatest(ctx context.Context, threadID string) (*AgentCheckpoint, error) {
	if r.db == nil || threadID == "" {
		return nil, nil
	}
	var rows []struct {
		ThreadID  string          `gorm:"column:thread_id"`
		Stage     string          `gorm:"column:stage"`
		State     json.RawMessage `gorm:"column:state"`
		UpdatedAt time.Time       `gorm:"column:updated_at"`
	}
	if err := r.db.WithContext(ctx).Table("agent_checkpoints").
		Where("thread_id = ?", threadID).
		Order("updated_at DESC").Limit(1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &AgentCheckpoint{
		ThreadID:  rows[0].ThreadID,
		Stage:     rows[0].Stage,
		State:     rows[0].State,
		UpdatedAt: rows[0].UpdatedAt,
	}, nil
}

// ResumeStage 返回应从哪个阶段续跑（无 checkpoint → 从第一阶段起跑）。
// 已完成阶段数 = checkpoint 在 AgentStageNames 中的位置 + 1。
func ResumeStage(latest *AgentCheckpoint) string {
	if latest == nil {
		return AgentStageNames[0]
	}
	for i, name := range AgentStageNames {
		if name == latest.Stage {
			if i+1 >= len(AgentStageNames) {
				return AgentStageNames[len(AgentStageNames)-1]
			}
			return AgentStageNames[i+1]
		}
	}
	return AgentStageNames[0]
}

// NewThreadID 生成 checkpoint 会话标识
func NewThreadID(sessionID string) string {
	if sessionID == "" {
		return "thr_" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return sessionID
}
