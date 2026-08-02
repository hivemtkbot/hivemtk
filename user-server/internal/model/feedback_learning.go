package model

import "time"

// ============================================================================
// G7 反馈学习闭环模型
// ----------------------------------------------------------------------------
// 对应 PRD §5.2 G7：系统自我进化
//
// 核心数据流：
//   销冠对话记录 → 销冠画像更新（5 维度雷达）
//                   ↓
//         SOP 节点转化率分析
//                   ↓
//         低转化节点 → 优化建议
//                   ↓
//         A/B 测试验证 → 自动选优
//
// 5 维度销冠画像：
//   1. 异议处理能力 objection_handling
//   2. 逼单邀约能力 closing_invitation
//   3. 跟进激活能力 followup_activation
//   4. 培育转化能力 nurturing_conversion
//   5. 复购运营能力 repurchase_operation
// ============================================================================

// SalesChampionDimension 销冠能力维度标识
type SalesChampionDimension string

const (
	// DimensionObjectionHandling 异议处理能力：识别客户异议并有效回应
	DimensionObjectionHandling SalesChampionDimension = "objection_handling"
	// DimensionClosingInvitation 逼单邀约能力：推动客户下单/到店/体验
	DimensionClosingInvitation SalesChampionDimension = "closing_invitation"
	// DimensionFollowupActivation 跟进激活能力：唤醒沉默/流失客户
	DimensionFollowupActivation SalesChampionDimension = "followup_activation"
	// DimensionNurturingConversion 培育转化能力：长期培育→最终转化
	DimensionNurturingConversion SalesChampionDimension = "nurturing_conversion"
	// DimensionRepurchaseOperation 复购运营能力：老客复购/增购/推荐
	DimensionRepurchaseOperation SalesChampionDimension = "repurchase_operation"
)

// AllSalesChampionDimensions 全部 5 维度
var AllSalesChampionDimensions = []SalesChampionDimension{
	DimensionObjectionHandling,
	DimensionClosingInvitation,
	DimensionFollowupActivation,
	DimensionNurturingConversion,
	DimensionRepurchaseOperation,
}

// SalesChampionProfileSnapshot 销冠画像快照
// 每次从对话记录提取能力维度后，生成一份快照持久化
type SalesChampionProfileSnapshot struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	StaffID       uint      `gorm:"index" json:"staff_id"`                            // 员工 ID（0 表示系统级 智能体）
	StaffName     string    `gorm:"type:varchar(100)" json:"staff_name"`              // 员工名称
	Scenario      string    `gorm:"type:varchar(50);index" json:"scenario"`           // 场景（如 "ai_champion"/"staff_123"）
	Dimension     string    `gorm:"type:varchar(50);not null;index" json:"dimension"` // SalesChampionDimension
	Score         float64   `gorm:"type:decimal(5,2);not null" json:"score"`          // 0-100
	SampleCount   int       `gorm:"default:0" json:"sample_count"`                    // 样本数（支撑该得分的对话数）
	PositiveCount int       `gorm:"default:0" json:"positive_count"`                  // 正向样本数
	NegativeCount int       `gorm:"default:0" json:"negative_count"`                  // 负向样本数
	EvidenceTags  JSONArray `gorm:"type:text" json:"evidence_tags"`                   // 证据标签（如 ["价格异议","已解答"]）
	PeriodStart   time.Time `json:"period_start"`                                     // 统计周期起始
	PeriodEnd     time.Time `json:"period_end"`                                       // 统计周期结束
	GeneratedAt   time.Time `gorm:"autoCreateTime" json:"generated_at"`
}

// TableName 表名
func (SalesChampionProfileSnapshot) TableName() string { return "sales_champion_profile_snapshots" }

// SOPNodeTransition SOP 节点流转记录
// 记录每个 SOP 执行中节点之间的流转情况，用于计算节点级转化率
type SOPNodeTransition struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SOPID           uint      `gorm:"index;not null" json:"sop_id"`
	ExecutionID     uint      `gorm:"index;not null" json:"execution_id"`
	CustomerID      string    `gorm:"type:varchar(64);index" json:"customer_id"`
	SessionID       string    `gorm:"type:varchar(50)" json:"session_id"`
	Variant         string    `gorm:"type:varchar(50);index" json:"variant"`   // A/B 测试 variant
	FromNode        string    `gorm:"type:varchar(50);index" json:"from_node"` // 起始节点（空表示入口）
	ToNode          string    `gorm:"type:varchar(50);index" json:"to_node"`   // 目标节点
	NodeType        string    `gorm:"type:varchar(30)" json:"node_type"`       // start/llm/condition/action/end
	Outcome         string    `gorm:"type:varchar(30);index" json:"outcome"`   // success/abandoned/failed/pending
	DurationMs      int       `gorm:"default:0" json:"duration_ms"`            // 节点停留时长
	MessageSnippets JSONArray `gorm:"type:text" json:"message_snippets"`       // 关键消息片段（最多 3 条）
	CreatedAt       time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (SOPNodeTransition) TableName() string { return "sop_node_transitions" }

// NodeOutcome 节点结果常量
const (
	NodeOutcomeSuccess   = "success"   // 节点成功推进
	NodeOutcomeAbandoned = "abandoned" // 客户中途放弃
	NodeOutcomeFailed    = "failed"    // 执行失败
	NodeOutcomePending   = "pending"   // 进行中
)

// OptimizationSuggestion 优化建议
// 对低转化节点自动生成的优化建议，供运营/产品审核
type OptimizationSuggestion struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SOPID          uint       `gorm:"index;not null" json:"sop_id"`
	SOPName        string     `gorm:"type:varchar(100)" json:"sop_name"`
	NodeID         string     `gorm:"type:varchar(50);index" json:"node_id"`
	NodeName       string     `gorm:"type:varchar(100)" json:"node_name"`
	NodeType       string     `gorm:"type:varchar(30)" json:"node_type"`
	CurrentScore   float64    `gorm:"type:decimal(5,2)" json:"current_score"`                 // 当前转化率（0-100）
	Threshold      float64    `gorm:"type:decimal(5,2)" json:"threshold"`                     // 目标阈值
	SuggestionType string     `gorm:"type:varchar(50);index" json:"suggestion_type"`          // prompt_rewrite/branch_prune/node_merge/...
	SuggestionText string     `gorm:"type:text;not null" json:"suggestion_text"`              // 建议内容
	ExpectedImpact string     `gorm:"type:varchar(200)" json:"expected_impact"`               // 预期影响
	Priority       int        `gorm:"default:0" json:"priority"`                              // 0-低 1-中 2-高
	Status         string     `gorm:"type:varchar(20);default:'pending';index" json:"status"` // pending/approved/applied/rejected
	SampleCount    int        `gorm:"default:0" json:"sample_count"`                          // 样本数
	EvidenceData   JSONMap    `gorm:"type:text" json:"evidence_data"`                         // 证据数据（JSON）
	GeneratedAt    time.Time  `gorm:"autoCreateTime" json:"generated_at"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	ReviewedBy     uint       `json:"reviewed_by"`
	AppliedAt      *time.Time `json:"applied_at"`
}

// TableName 表名
func (OptimizationSuggestion) TableName() string { return "optimization_suggestions" }

// SuggestionStatus 建议状态常量
const (
	SuggestionStatusPending  = "pending"
	SuggestionStatusApproved = "approved"
	SuggestionStatusApplied  = "applied"
	SuggestionStatusRejected = "rejected"
)

// SuggestionType 建议类型常量
const (
	SuggestionTypePromptRewrite = "prompt_rewrite" // 重写节点 prompt
	SuggestionTypeBranchPrune   = "branch_prune"   // 剪枝低效分支
	SuggestionTypeNodeMerge     = "node_merge"     // 合并冗余节点
	SuggestionTypeAddObjection  = "add_objection"  // 补充异议处理分支
	SuggestionTypeAddEmpathy    = "add_empathy"    // 补充共情话术
	SuggestionTypeTimingAdjust  = "timing_adjust"  // 调整触达时机
)
