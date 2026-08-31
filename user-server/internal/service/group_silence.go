package service

import (
	"context"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ===== H-4 群聊沉默检测与复活信号 =====
//
// 决策依据 M17 H-4：
//   - 三档判定：""（活跃/不干预）、>72h 且最后发言人为 bot → revive_candidate（可复活）、
//     >168h 无条件 dead（放弃唤醒）
//   - 复活信号经既有进程内事件总线（internal/event）发布，SOP/订阅方 Subscribe 消费
//   - 文案结构仅承载"价值型内容建议"字段，不写死营销话术（话术由上层话术库渲染）

// 判定结果档位
const (
	GroupReviveCandidate = "revive_candidate"
	GroupDeadVerdict     = "dead"
)

// GroupReviveSignal 复活信号 key（规格命名，供 SOP trigger / 事件订阅方识别）
const GroupReviveSignal = "group_revive"

// 公开可调阈值
const (
	// GroupSilenceReviveAfter 超 72h 无真人消息且最后发言人为 bot → 复活候选
	GroupSilenceReviveAfter = 72 * time.Hour
	// GroupSilenceDeadAfter 超 168h 无真人消息 → 无条件判死
	GroupSilenceDeadAfter = 168 * time.Hour
)

// DetectGroupSilence 三档判定（now 由调用方注入，确定性可测）：
//   - 沉默 >168h：无条件 "dead"（优先于复活判定）
//   - 沉默 >72h 且最后发言人是 bot：返回 "revive_candidate"（bot 说话无人接，值得换钩子复活）
//   - 其余：返回 ""（活跃或未达干预阈值）
func DetectGroupSilence(lastNonBotMsgAt time.Time, lastSpeakerIsBot bool, now time.Time) string {
	silence := now.Sub(lastNonBotMsgAt)
	if silence > GroupSilenceDeadAfter {
		return GroupDeadVerdict
	}
	if silence > GroupSilenceReviveAfter && lastSpeakerIsBot {
		return GroupReviveCandidate
	}
	return ""
}

// ReviveSignalKey 复活信号 key（兼容别名）
const ReviveSignalKey = GroupReviveSignal

// TopicGroupRevive 事件总线主题（命名对齐 {模块}.{动作} 规范）
const TopicGroupRevive = "group.revive"

// ReviveVerdict 复活建议结构：只给"建议字段"，话术由上层渲染（不写死营销文案）
type ReviveVerdict struct {
	SignalKey    string  `json:"signal_key"`
	GroupID      string  `json:"group_id"`
	Verdict      string  `json:"verdict"`
	SilenceHours float64 `json:"silence_hours"`
	// Suggestions 价值型内容建议（枚举，非具体话术）：value_content / faq_digest / poll / help_recap
	Suggestions []string  `json:"suggestions"`
	SuggestedAt time.Time `json:"suggested_at"`
}

// BuildReviveVerdict 构建复活建议：revive_candidate 档给价值型内容建议，dead 档不干预
func BuildReviveVerdict(groupID string, verdict string, silenceHours float64, now time.Time) ReviveVerdict {
	var suggestions []string
	if verdict == GroupReviveCandidate {
		suggestions = []string{"value_content", "poll", "help_recap"}
	}
	return ReviveVerdict{
		SignalKey:    ReviveSignalKey,
		GroupID:      groupID,
		Verdict:      verdict,
		SilenceHours: silenceHours,
		Suggestions:  suggestions,
		SuggestedAt:  now,
	}
}

// GroupRevivePayload 群复活事件载荷（经事件总线发布，SOP 侧 Subscribe(TopicGroupRevive) 消费）
type GroupRevivePayload struct {
	SignalKey string        `json:"signal_key"`
	GroupID   string        `json:"group_id"`
	Verdict   string        `json:"verdict"`
	Detail    ReviveVerdict `json:"detail"`
}

// EmitGroupRevive 挂接既有 signal/emitter 机制：进程内事件总线 internal/event。
// 找到的挂接点为全局 event.Publish（OperationLog/AssetDegraded 等同款机制）；
// 总线未初始化时 Publish 为 no-op，安全。ctx 保留以满足调用方签名约定（总线为进程内异步）。
func EmitGroupRevive(ctx context.Context, groupID string, v ReviveVerdict) {
	_ = ctx
	logger.Infof("[H-4] 发布群复活信号 group=%s verdict=%s", groupID, v.Verdict)
	event.Publish(TopicGroupRevive, GroupRevivePayload{
		SignalKey: ReviveSignalKey,
		GroupID:   groupID,
		Verdict:   v.Verdict,
		Detail:    v,
	})
}
