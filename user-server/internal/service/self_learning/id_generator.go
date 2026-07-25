package selflearning

// id_generator.go 自我学习机制的 ID 生成纯函数
//
// 五层架构归属: L4 能力层
//
// 设计说明：
//   - 三个函数均为纯函数（sha256 哈希计算），不访问 DB、不依赖外部状态
//   - 历史上曾放在 repository 层，但 repository 层禁止包含业务逻辑
//     （五层架构规范 §七：repository 层只做 CRUD，不做业务计算）
//   - 现下沉至 service 层，由 service 在调用 repo.Create 前预先生成 ID
//   - repo 的 Create 方法仍保留 "若 LogID 为空则使用占位" 的兼容路径，
//     但实际生产路径由 service 层显式生成

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"marketing/internal/model"
)

// GenLogID 生成 log_id（sha256 去重键）
//
// 组成：session_id|scenario 的 sha256 前 32 字符
// 相同 session + scenario 总是生成相同 log_id，配合 DB UNIQUE 约束实现幂等
func GenLogID(sessionID string, scenario model.SelfLearningScenario) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s", sessionID, string(scenario))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// GenSignalID 生成 signal_id（基于 target+metric+bucket 哈希）
//
// UPSERT 语义：相同 (target_type, target_id, metric_name, bucket_hour) 仅一条
func GenSignalID(targetType model.SupervisionTargetType, targetID, metricName string, bucket time.Time) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", string(targetType), targetID, metricName, bucket.Format(time.RFC3339))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// GenActionID 生成 action_id（sha256 去重键）
//
// 幂等设计：相同 (triggerLogID, actionType, targetID) 总是生成相同 action_id，
// 配合 self_correction_actions.action_id 的 UNIQUE 约束实现幂等：
//   - 同一触发多次调用只会创建一条 action（其他被 UNIQUE 拦截）
//   - 调用方应通过 errors.Is(err, ErrDuplicateAction) 识别幂等冲突
//
// 历史问题：原实现包含 time.Now().UnixNano()，导致每次调用生成不同 ID，
// 破坏了幂等性。现已移除时间戳，确保幂等。如需为同一触发创建多个同类型 action，
// 调用方应传入不同的 targetID 区分。
func GenActionID(triggerLogID string, actionType model.CorrectionActionType, targetID string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s", triggerLogID, string(actionType), targetID)
	return hex.EncodeToString(h.Sum(nil))[:32]
}
