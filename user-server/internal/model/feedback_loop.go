package model

import "time"

// ============================================================================
// 反馈学习闭环模型
// ----------------------------------------------------------------------------
// 对应 PRD §5.2 闭环 5：转化漏斗 → 销冠画像 → SOP A/B 测试 → 自动选优
// 设计依据: docs/核心链路优化.md 第十七章 §17.3 表结构设计
//
// 私域独立部署：严禁 merchant_id / tenant_id 多租户字段
// 所有数据归属当前部署实例，不引入多租户隔离字段
//
// 6 张新表职责分层：
//   Layer 1 原始事件层  - feedback_events     反馈事件流水（append-only）
//   Layer 2 信号聚合层  - feedback_signals    按 session 聚合的奖励值（Bandit/画像唯一输入）
//   Layer 3 知识沉淀层  - champion_dialogues  销冠对话 + pgvector 向量
//                        prompt_candidates    Prompt 候选版本池
//   Layer 4 学习状态层  - bandit_arms         Thompson Sampling 臂状态
//                        prompt_ab_tests      A/B 测试配置与结果
// ============================================================================

// ----------------------------------------------------------------------------
// Layer 1: feedback_events 反馈事件原始表
// ----------------------------------------------------------------------------

// FeedbackEvent 反馈事件（append-only 流水）
//
// 三类信号统一入库：
//   - explicit  显式反馈（点赞/点踩/评分/投诉/转人工原因/人工修订）
//   - implicit  隐式反馈（转化率/回复率/会话时长/转人工）
//   - champion  销冠标记（is_champion/script_adopt/5 维度评分）
type FeedbackEvent struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID           string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"event_id"` // 业务唯一 ID（防重复）
	SessionID         string    `gorm:"type:varchar(50);index;not null" json:"session_id"`
	CustomerID        string    `gorm:"type:varchar(64);index;not null" json:"customer_id"`
	SOPID             uint      `gorm:"index" json:"sop_id"`                                // 关联 SOP（可空，0 表示空）
	ExecutionID       uint      `json:"execution_id"`                                       // SOP 执行 ID
	Variant           string    `gorm:"type:varchar(50);index" json:"variant"`              // A/B variant
	PromptCandidateID uint      `gorm:"index" json:"prompt_candidate_id"`                   // Prompt 候选 ID
	EventType         string    `gorm:"type:varchar(30);index;not null" json:"event_type"`  // explicit/implicit/champion
	SignalKey         string    `gorm:"type:varchar(50);index;not null" json:"signal_key"`  // like/dislike/rating/complaint/...
	SignalValue       JSONMap   `gorm:"type:jsonb;not null" json:"signal_value"`            // 信号原始值
	Weight            float64   `gorm:"type:decimal(4,2);not null;default:0" json:"weight"` // 信号权重
	Reward            float64   `gorm:"type:decimal(6,3);not null;default:0" json:"reward"` // weight * normalized_value
	AIReply           string    `gorm:"type:text" json:"ai_reply"`                          // 触发反馈的 AI 回复快照
	CustomerMsg       string    `gorm:"type:text" json:"customer_msg"`                      // 客户消息快照
	Metadata          JSONMap   `gorm:"type:jsonb" json:"metadata"`                         // 扩展元数据
	CreatedBy         uint      `json:"created_by"`                                         // 提交者（0 表示系统）
	CreatedAt         time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (FeedbackEvent) TableName() string { return "feedback_events" }

// FeedbackSignalKey 反馈信号类型常量（与 service/feedback_loop SignalKey 字符串值一致）
const (
	FeedbackSignalLike         = "like"          // 客户点赞（显式）
	FeedbackSignalDislike      = "dislike"       // 客户点踩（显式）
	FeedbackSignalRating       = "rating"        // 评分 1-5（显式）
	FeedbackSignalComplaint    = "complaint"     // 投诉（显式）
	FeedbackSignalConversion   = "conversion"    // 转化成功（隐式）
	FeedbackSignalReplyRate    = "reply_rate"    // 客户回复率（隐式）
	FeedbackSignalDuration     = "duration"      // 会话时长（隐式）
	FeedbackSignalTransfer     = "transfer"      // 转人工（隐式）
	FeedbackSignalChampionMark = "champion_mark" // 销冠标记（销冠）
	FeedbackSignalScriptAdopt  = "script_adopt"  // 话术采用（销冠）
)

// FeedbackEventType 反馈事件类型常量
const (
	FeedbackEventTypeExplicit = "explicit"
	FeedbackEventTypeImplicit = "implicit"
	FeedbackEventTypeChampion = "champion"
)

// ----------------------------------------------------------------------------
// Layer 2: feedback_signals 反馈信号聚合表
// ----------------------------------------------------------------------------

// FeedbackSignal 反馈信号聚合（按 session_id 唯一）
//
// 一个 session 多次反馈事件累加为单一 reward 值，供 Bandit / 画像分析消费
type FeedbackSignal struct {
	ID                uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID         string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"session_id"`
	CustomerID        string     `gorm:"type:varchar(64);not null" json:"customer_id"`
	SOPID             uint       `gorm:"index" json:"sop_id"`
	Variant           string     `gorm:"type:varchar(50);index" json:"variant"`
	PromptCandidateID uint       `gorm:"index" json:"prompt_candidate_id"`
	AggregatedReward  float64    `gorm:"type:decimal(8,3);not null;default:0" json:"aggregated_reward"` // 该 session 所有事件奖励之和
	SignalCount       int        `gorm:"not null;default:0" json:"signal_count"`
	SignalBreakdown   JSONMap    `gorm:"type:jsonb;not null" json:"signal_breakdown"`                // {like:1, conversion:1, ...}
	Outcome           string     `gorm:"type:varchar(20);not null;default:'pending'" json:"outcome"` // success/fail/pending
	IsChampion        bool       `gorm:"not null;default:false" json:"is_champion"`                  // 是否销冠对话
	SessionStartedAt  *time.Time `json:"session_started_at"`
	SessionEndedAt    *time.Time `json:"session_ended_at"`
	CreatedAt         time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (FeedbackSignal) TableName() string { return "feedback_signals" }

// FeedbackSignalOutcome 反馈信号结果常量
const (
	FeedbackSignalOutcomeSuccess = "success"
	FeedbackSignalOutcomeFail    = "fail"
	FeedbackSignalOutcomePending = "pending"
)

// ----------------------------------------------------------------------------
// Layer 3a: champion_dialogues 销冠对话表（pgvector 向量化）
// ----------------------------------------------------------------------------

// ChampionDialogue 销冠对话片段
//
// 来源：ChampionDialogueAnalyzer 从 feedback_signals 筛选高价值对话聚类后入库
// 用途：1) 相似场景话术检索（pgvector 余弦相似度） 2) 销冠画像快照生成
type ChampionDialogue struct {
	ID                  uint    `gorm:"primaryKey;autoIncrement" json:"id"`
	DialogueFingerprint string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"dialogue_fingerprint"` // 对话指纹（防重复入库）
	SessionID           string  `gorm:"type:varchar(50);not null" json:"session_id"`
	CustomerID          string  `gorm:"type:varchar(64);not null" json:"customer_id"`
	StaffID             uint    `gorm:"index" json:"staff_id"` // 销冠员工 ID（0=AI 销冠）
	StaffName           string  `gorm:"type:varchar(100)" json:"staff_name"`
	Scenario            string  `gorm:"type:varchar(50);not null;index" json:"scenario"` // objection/closing/followup/nurture/repurchase
	JourneyStage        string  `gorm:"type:varchar(30)" json:"journey_stage"`           // lead/contact/consider/decide/retain
	CustomerMsg         string  `gorm:"type:text;not null" json:"customer_msg"`
	ChampionReply       string  `gorm:"type:text;not null" json:"champion_reply"`
	ContextMsgs         JSONMap `gorm:"type:jsonb" json:"context_msgs"` // 上下文消息（前 3 轮）
	// Embedding 用 pgvector 字符串字面量格式 '[v1,v2,...]' 存储
	// GORM 不直接支持 vector 类型，用 string + 自定义 SQL 写入
	Embedding          string     `gorm:"type:vector(1024);not null" json:"-"` // pgvector 1024 维
	ClusterID          uint       `gorm:"index" json:"cluster_id"`             // 聚类簇 ID
	Reward             float64    `gorm:"type:decimal(6,3);not null;default:0" json:"reward"`
	ConversionAchieved bool       `gorm:"not null;default:false" json:"conversion_achieved"`
	ExtractedScripts   JSONMap    `gorm:"type:jsonb" json:"extracted_scripts"` // 已提取的话术 [{title,content,...}]
	ExtractedAt        *time.Time `json:"extracted_at"`
	CreatedAt          time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (ChampionDialogue) TableName() string { return "champion_dialogues" }

// ChampionScenario 销冠对话场景常量
const (
	ChampionScenarioObjection  = "objection"  // 异议处理
	ChampionScenarioClosing    = "closing"    // 逼单邀约
	ChampionScenarioFollowup   = "followup"   // 跟进激活
	ChampionScenarioNurture    = "nurture"    // 培育转化
	ChampionScenarioRepurchase = "repurchase" // 复购运营
	ChampionScenarioGeneral    = "general"    // 通用
)

// ChampionJourneyStage 客户旅程阶段常量
const (
	ChampionStageLead     = "lead"
	ChampionStageContact  = "contact"
	ChampionStageConsider = "consider"
	ChampionStageDecide   = "decide"
	ChampionStageRetain   = "retain"
)

// ----------------------------------------------------------------------------
// Layer 3b: prompt_candidates Prompt 候选表
// ----------------------------------------------------------------------------

// PromptCandidate Prompt 候选版本
//
// 来源：PromptIterator 基于负反馈样本让 LLM 生成改进版本
// 用途：1) A/B 测试 variant 池 2) SalesEngine.generateCandidate 加载 system/user prompt
type PromptCandidate struct {
	ID                 uint    `gorm:"primaryKey;autoIncrement" json:"id"`
	SOPNodeID          string  `gorm:"type:varchar(50);index" json:"sop_node_id"` // 关联 SOP 节点（空表示全局）
	SOPID              uint    `gorm:"index" json:"sop_id"`
	Scenario           string  `gorm:"type:varchar(50);not null;index" json:"scenario"` // llm_system/sop_reply/objection/...
	Version            string  `gorm:"type:varchar(20);not null" json:"version"`        // v1.0/v1.1/v2.0...
	Title              string  `gorm:"type:varchar(100);not null" json:"title"`
	SystemPrompt       string  `gorm:"type:text;not null" json:"system_prompt"`
	UserPromptTemplate string  `gorm:"type:text;not null" json:"user_prompt_template"`                // 含变量占位符 {{product}}/{{customer_name}}
	Variables          JSONMap `gorm:"type:jsonb" json:"variables"`                                   // 变量定义
	ParentID           uint    `gorm:"index" json:"parent_id"`                                        // 父版本 ID（迭代溯源）
	ImprovementNotes   string  `gorm:"type:text" json:"improvement_notes"`                            // 改进点说明
	Status             string  `gorm:"type:varchar(20);not null;default:'draft';index" json:"status"` // draft/pending/approved/active/promoted/retired
	// Bandit 状态（每个候选作为 Bandit 的一个 arm）
	Alpha         float64    `gorm:"type:decimal(8,2);not null;default:2" json:"alpha"` // Beta(alpha, beta) 后验
	Beta          float64    `gorm:"type:decimal(8,2);not null;default:2" json:"beta"`
	SampleCount   int        `gorm:"not null;default:0" json:"sample_count"`
	SuccessCount  int        `gorm:"not null;default:0" json:"success_count"`
	AvgReward     float64    `gorm:"type:decimal(6,3);not null;default:0" json:"avg_reward"`
	PromotedAt    *time.Time `json:"promoted_at"`
	RetiredAt     *time.Time `json:"retired_at"`
	RetiredReason string     `gorm:"type:varchar(100)" json:"retired_reason"`
	ReviewedBy    uint       `json:"reviewed_by"`
	ReviewedAt    *time.Time `json:"reviewed_at"`
	GeneratedBy   string     `gorm:"type:varchar(20);not null;default:'auto'" json:"generated_by"` // auto/manual/llm
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (PromptCandidate) TableName() string { return "prompt_candidates" }

// PromptCandidateStatus Prompt 候选状态常量
const (
	PromptCandidateStatusDraft    = "draft"    // 草稿（待 LLM 生成完成）
	PromptCandidateStatusPending  = "pending"  // 待人工审核
	PromptCandidateStatusApproved = "approved" // 已审核通过，待入 Bandit
	PromptCandidateStatusActive   = "active"   // 当前生效（A/B 测试中）
	PromptCandidateStatusPromoted = "promoted" // 胜出已上线
	PromptCandidateStatusRetired  = "retired"  // 已淘汰
)

// ----------------------------------------------------------------------------
// Layer 4a: bandit_arms Bandit 臂状态表
// ----------------------------------------------------------------------------

// BanditArm Multi-Armed Bandit 臂状态
//
// 每个 arm 关联一个 variant（prompt_candidate / sop_variant / script）
// 维护 Beta(alpha, beta) 后验，由 BanditAllocator 增量更新
type BanditArm struct {
	ID                uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID      string `gorm:"type:varchar(64);index;not null" json:"experiment_id"`                     // 实验 ID（同一 SOP 多次实验隔离）
	ExperimentType    string `gorm:"type:varchar(20);not null" json:"experiment_type"`                         // prompt/sop_variant/script
	ArmKey            string `gorm:"type:varchar(100);not null;uniqueIndex:idx_bandit_exp_arm" json:"arm_key"` // variant 标识
	SOPID             uint   `gorm:"index" json:"sop_id"`
	Variant           string `gorm:"type:varchar(50)" json:"variant"`
	PromptCandidateID uint   `json:"prompt_candidate_id"`
	// Beta 后验
	Alpha float64 `gorm:"type:decimal(10,2);not null;default:1" json:"alpha"`
	Beta  float64 `gorm:"type:decimal(10,2);not null;default:1" json:"beta"`
	// 统计
	TotalTrials   int64   `gorm:"not null;default:0" json:"total_trials"`
	SuccessTrials int64   `gorm:"not null;default:0" json:"success_trials"`
	SumReward     float64 `gorm:"type:decimal(12,3);not null;default:0" json:"sum_reward"`
	AvgReward     float64 `gorm:"type:decimal(8,4);not null;default:0" json:"avg_reward"`
	// 流量控制
	MinTrafficPct     float64 `gorm:"type:decimal(5,2);not null;default:10.00" json:"min_traffic_pct"` // 最低流量保障
	MaxTrafficPct     float64 `gorm:"type:decimal(5,2);not null;default:60.00" json:"max_traffic_pct"` // 最高流量限制
	CurrentTrafficPct float64 `gorm:"type:decimal(5,2);not null;default:0" json:"current_traffic_pct"`
	// 状态
	Status     string     `gorm:"type:varchar(20);not null;default:'exploring';index" json:"status"` // exploring/exploiting/promoted/retired
	PromotedAt *time.Time `json:"promoted_at"`
	RetiredAt  *time.Time `json:"retired_at"`
	// 收敛诊断
	PosteriorBestProb float64    `gorm:"type:decimal(5,4)" json:"posterior_best_prob"` // P(本臂最优) 后验概率
	LastSampledAt     *time.Time `json:"last_sampled_at"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (BanditArm) TableName() string { return "bandit_arms" }

// BanditArmStatus Bandit 臂状态常量
const (
	BanditArmStatusExploring  = "exploring"  // 探索期
	BanditArmStatusExploiting = "exploiting" // 利用期
	BanditArmStatusPromoted   = "promoted"   // 已胜出
	BanditArmStatusRetired    = "retired"    // 已淘汰
)

// BanditExperimentType Bandit 实验类型常量
const (
	BanditExperimentTypePrompt     = "prompt"
	BanditExperimentTypeSOPVariant = "sop_variant"
	BanditExperimentTypeScript     = "script"
)

// ----------------------------------------------------------------------------
// Layer 4b: prompt_ab_tests Prompt A/B 测试配置表
// ----------------------------------------------------------------------------

// PromptABTest Prompt A/B 测试配置与结果
//
// 一次 A/B 测试包含多个 arm（variant），由 BanditAllocator 进行流量分配
type PromptABTest struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ExperimentID   string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"experiment_id"`
	ExperimentType string     `gorm:"type:varchar(20);not null" json:"experiment_type"` // prompt/sop_variant
	SOPID          uint       `gorm:"index" json:"sop_id"`
	SOPNodeID      string     `gorm:"type:varchar(50)" json:"sop_node_id"`
	Name           string     `gorm:"type:varchar(100);not null" json:"name"`
	Description    string     `gorm:"type:text" json:"description"`
	ArmKeys        JSONArray  `gorm:"type:jsonb;not null" json:"arm_keys"`                             // ["arm_0","arm_1",...]
	Config         JSONMap    `gorm:"type:jsonb;not null" json:"config"`                               // {min_traffic, max_traffic, min_samples, posterior_threshold}
	Status         string     `gorm:"type:varchar(20);not null;default:'running';index" json:"status"` // draft/running/paused/completed/rolled_back
	StartedAt      *time.Time `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	WinnerArmKey   string     `gorm:"type:varchar(100)" json:"winner_arm_key"`
	AutoPromote    bool       `gorm:"not null;default:true" json:"auto_promote"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (PromptABTest) TableName() string { return "prompt_ab_tests" }

// PromptABTestStatus A/B 测试状态常量
const (
	PromptABTestStatusDraft      = "draft"
	PromptABTestStatusRunning    = "running"
	PromptABTestStatusPaused     = "paused"
	PromptABTestStatusCompleted  = "completed"
	PromptABTestStatusRolledBack = "rolled_back"
)
