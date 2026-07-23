package model

// confidence_signal.go 置信度驱动转人工 P0-3 模型层
//
// 五层架构归属: L5 数据层
// 设计依据: docs/核心链路优化.md 第十五章 §15.3 表结构设计
//
// 包含 7 张表对应的 GORM 模型：
//  1. ConfidenceSignal       - 每次 5 维信号快照
//  2. ConfidenceCalibration  - 温度缩放校准参数历史
//  3. HandoffDecision        - 转人工决策记录
//  4. ReviewQueue            - 边界审核队列
//  5. ThresholdPolicy        - 动态阈值策略配置
//  6. SLAMonitor             - SLA 监控指标时序
//  7. ABTest                 - A/B 测试配置与统计
//
// 私域独立部署: 无 merchant_id 字段

import "time"

// ConfidenceSignal 5 维置信度信号快照（对应 confidence_signals 表）
type ConfidenceSignal struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SignalID             string    `gorm:"column:signal_id;uniqueIndex;size:64;not null" json:"signal_id"`
	SessionID            string    `gorm:"column:session_id;size:128;not null;index:idx_signals_session,priority:1" json:"session_id"`
	CustomerID           string    `gorm:"column:customer_id;size:128;not null;index:idx_signals_customer,priority:1" json:"customer_id"`
	MessageID            string    `gorm:"column:message_id;size:128;not null" json:"message_id"`
	IntentType           string    `gorm:"column:intent_type;size:64;not null" json:"intent_type"`
	IntentConf           float64   `gorm:"column:intent_conf;type:decimal(5,4);not null" json:"intent_conf"`
	IntentConfCalibrated float64   `gorm:"column:intent_conf_calibrated;type:decimal(5,4);not null" json:"intent_conf_calibrated"`
	EntityComp           float64   `gorm:"column:entity_comp;type:decimal(5,4);not null" json:"entity_comp"`
	CtxRelev             float64   `gorm:"column:ctx_relev;type:decimal(5,4);not null" json:"ctx_relev"`
	RAGQual              float64   `gorm:"column:rag_qual;type:decimal(5,4);not null" json:"rag_qual"`
	LLMEntropy           float64   `gorm:"column:llm_entropy;type:decimal(5,4);not null" json:"llm_entropy"`
	AggregatedConf       float64   `gorm:"column:aggregated_conf;type:decimal(5,4);not null" json:"aggregated_conf"`
	VetoTriggered        string    `gorm:"column:veto_triggered;size:64;default:''" json:"veto_triggered"`
	DynamicThreshold     float64   `gorm:"column:dynamic_threshold;type:decimal(5,4);not null" json:"dynamic_threshold"`
	DecisionBand         string    `gorm:"column:decision_band;size:32;not null;index:idx_signals_band,priority:1" json:"decision_band"`
	Temperature          float64   `gorm:"column:temperature;type:decimal(6,4);not null;default:1.0" json:"temperature"`
	CreatedAt            time.Time `gorm:"column:created_at;not null;default:now();index:idx_signals_session,priority:2;index:idx_signals_customer,priority:2;index:idx_signals_band,priority:2" json:"created_at"`
}

// TableName 表名
func (ConfidenceSignal) TableName() string { return "confidence_signals" }

// ConfidenceCalibration 温度缩放校准参数历史
type ConfidenceCalibration struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CalibrationID string    `gorm:"column:calibration_id;uniqueIndex;size:64;not null" json:"calibration_id"`
	SignalType    string    `gorm:"column:signal_type;size:32;not null;index:idx_calibrations_active,priority:1" json:"signal_type"`
	Method        string    `gorm:"column:method;size:32;not null" json:"method"`
	Temperature   float64   `gorm:"column:temperature;type:decimal(6,4);not null" json:"temperature"`
	PlattA        float64   `gorm:"column:platt_a;type:decimal(8,4);default:0" json:"platt_a"`
	PlattB        float64   `gorm:"column:platt_b;type:decimal(8,4);default:0" json:"platt_b"`
	ECEBefore     float64   `gorm:"column:ece_before;type:decimal(5,4);not null" json:"ece_before"`
	ECEAfter      float64   `gorm:"column:ece_after;type:decimal(5,4);not null" json:"ece_after"`
	NLLBefore     float64   `gorm:"column:nll_before;type:decimal(8,4);not null" json:"nll_before"`
	NLLAfter      float64   `gorm:"column:nll_after;type:decimal(8,4);not null" json:"nll_after"`
	SampleSize    int       `gorm:"column:sample_size;not null" json:"sample_size"`
	FitStartedAt  time.Time `gorm:"column:fit_started_at;not null" json:"fit_started_at"`
	FitFinishedAt time.Time `gorm:"column:fit_finished_at;not null" json:"fit_finished_at"`
	IsActive      bool      `gorm:"column:is_active;not null;default:false;index:idx_calibrations_active,priority:2" json:"is_active"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

// TableName 表名
func (ConfidenceCalibration) TableName() string { return "confidence_calibrations" }

// HandoffDecisionRecord 转人工决策记录
//
// 注意：命名为 HandoffDecisionRecord 而非 HandoffDecision 以避免与 service 层的 HandoffDecision 服务类型重名
type HandoffDecisionRecord struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	DecisionID        string     `gorm:"column:decision_id;uniqueIndex;size:64;not null" json:"decision_id"`
	SessionID         string     `gorm:"column:session_id;size:128;not null;index:idx_handoff_session,priority:1" json:"session_id"`
	CustomerID        string     `gorm:"column:customer_id;size:128;not null" json:"customer_id"`
	SignalID          string     `gorm:"column:signal_id;size:64;not null" json:"signal_id"`
	Reason            string     `gorm:"column:reason;size:64;not null" json:"reason"`
	ReasonDetail      string     `gorm:"column:reason_detail;type:text;default:''" json:"reason_detail"`
	Confidence        float64    `gorm:"column:confidence;type:decimal(5,4);not null" json:"confidence"`
	Threshold         float64    `gorm:"column:threshold;type:decimal(5,4);not null" json:"threshold"`
	IntentType        string     `gorm:"column:intent_type;size:64;not null" json:"intent_type"`
	CustomerLevel     string     `gorm:"column:customer_level;size:16;default:'normal'" json:"customer_level"`
	Timeslot          string     `gorm:"column:timeslot;size:16;default:''" json:"timeslot"`
	AgentAvailability float64    `gorm:"column:agent_availability;type:decimal(5,4);default:0" json:"agent_availability"`
	AssignedAgentID   int64      `gorm:"column:assigned_agent_id;default:0;index:idx_handoff_agent,priority:1" json:"assigned_agent_id"`
	AssignedAt        *time.Time `gorm:"column:assigned_at" json:"assigned_at,omitempty"`
	AcceptedAt        *time.Time `gorm:"column:accepted_at" json:"accepted_at,omitempty"`
	ResolvedAt        *time.Time `gorm:"column:resolved_at" json:"resolved_at,omitempty"`
	CustomerAccepted  bool       `gorm:"column:customer_accepted;default:false" json:"customer_accepted"`
	SLABreached       bool       `gorm:"column:sla_breached;default:false;index:idx_handoff_sla,priority:1" json:"sla_breached"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now();index:idx_handoff_session,priority:2;index:idx_handoff_agent,priority:2;index:idx_handoff_sla,priority:2" json:"created_at"`
}

// TableName 表名
func (HandoffDecisionRecord) TableName() string { return "handoff_decisions" }

// ReviewQueue 边界审核队列
type ReviewQueue struct {
	ID                 int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ItemID             string     `gorm:"column:item_id;uniqueIndex;size:64;not null" json:"item_id"`
	SessionID          string     `gorm:"column:session_id;size:128;not null;index:idx_review_session,priority:1" json:"session_id"`
	CustomerID         string     `gorm:"column:customer_id;size:128;not null" json:"customer_id"`
	SignalID           string     `gorm:"column:signal_id;size:64;not null" json:"signal_id"`
	DraftReply         string     `gorm:"column:draft_reply;type:text;not null" json:"draft_reply"`
	OriginalConfidence float64    `gorm:"column:original_confidence;type:decimal(5,4);not null" json:"original_confidence"`
	Threshold          float64    `gorm:"column:threshold;type:decimal(5,4);not null" json:"threshold"`
	IntentType         string     `gorm:"column:intent_type;size:64;not null" json:"intent_type"`
	Status             string     `gorm:"column:status;size:32;not null;default:'pending';index:idx_review_status,priority:1" json:"status"`
	AssignedAgentID    int64      `gorm:"column:assigned_agent_id;default:0" json:"assigned_agent_id"`
	EditedReply        string     `gorm:"column:edited_reply;type:text;default:''" json:"edited_reply"`
	AgentAction        string     `gorm:"column:agent_action;size:32;default:''" json:"agent_action"`
	SLADeadline        time.Time  `gorm:"column:sla_deadline;not null;index:idx_review_status,priority:2" json:"sla_deadline"`
	ActedAt            *time.Time `gorm:"column:acted_at" json:"acted_at,omitempty"`
	AutoReleased       bool       `gorm:"column:auto_released;default:false" json:"auto_released"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null;default:now();index:idx_review_session,priority:2" json:"created_at"`
}

// TableName 表名
func (ReviewQueue) TableName() string { return "review_queue" }

// ThresholdPolicy 动态阈值策略配置
type ThresholdPolicy struct {
	ID                      int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	PolicyID                string    `gorm:"column:policy_id;uniqueIndex;size:64;not null" json:"policy_id"`
	IntentType              string    `gorm:"column:intent_type;size:64;not null;index:idx_policies_intent,priority:1" json:"intent_type"`
	// 前端置信度面板(Tab3)富字段
	Name          string  `gorm:"column:name;size:128;not null;default:''" json:"name"`
	Scenario      string  `gorm:"column:scenario;size:64;not null;default:'all'" json:"scenario"`
	VipThreshold  float64 `gorm:"column:vip_threshold;type:decimal(5,4);not null;default:0.85" json:"vip_threshold"`
	TransferLow   float64 `gorm:"column:transfer_low;type:decimal(5,4);not null;default:0.30" json:"transfer_low"`
	Remark        string  `gorm:"column:remark;type:text" json:"remark"`
	BaseThreshold           float64   `gorm:"column:base_threshold;type:decimal(5,4);not null" json:"base_threshold"`
	CustomerLevelWeight     float64   `gorm:"column:customer_level_weight;type:decimal(5,4);not null;default:0.05" json:"customer_level_weight"`
	TimeslotWeight          float64   `gorm:"column:timeslot_weight;type:decimal(5,4);not null;default:0.05" json:"timeslot_weight"`
	AgentAvailabilityWeight float64   `gorm:"column:agent_availability_weight;type:decimal(5,4);not null;default:0.10" json:"agent_availability_weight"`
	BandHandoffUpper        float64   `gorm:"column:band_handoff_upper;type:decimal(5,4);not null;default:0.40" json:"band_handoff_upper"`
	BandFallbackUpper       float64   `gorm:"column:band_fallback_upper;type:decimal(5,4);not null;default:0.60" json:"band_fallback_upper"`
	BandReviewUpper         float64   `gorm:"column:band_review_upper;type:decimal(5,4);not null;default:0.75" json:"band_review_upper"`
	ReviewSLASeconds        int       `gorm:"column:review_sla_seconds;not null;default:30" json:"review_sla_seconds"`
	IsActive                bool      `gorm:"column:is_active;not null;default:true;index:idx_policies_intent,priority:2" json:"is_active"`
	Version                 int       `gorm:"column:version;not null;default:1;index:idx_policies_intent,priority:3" json:"version"`
	CreatedAt               time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt               time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

// TableName 表名
func (ThresholdPolicy) TableName() string { return "threshold_policies" }

// SLAMonitor SLA 监控指标时序
type SLAMonitor struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	MonitorID             string    `gorm:"column:monitor_id;uniqueIndex;size:64;not null" json:"monitor_id"`
	BucketMinute          time.Time `gorm:"column:bucket_minute;not null;index:idx_sla_bucket,priority:1" json:"bucket_minute"`
	AutoReplyRate         float64   `gorm:"column:auto_reply_rate;type:decimal(5,4);not null" json:"auto_reply_rate"`
	HandoffRate           float64   `gorm:"column:handoff_rate;type:decimal(5,4);not null" json:"handoff_rate"`
	ReviewTimeoutRate     float64   `gorm:"column:review_timeout_rate;type:decimal(5,4);not null" json:"review_timeout_rate"`
	AvgAssignmentSeconds  float64   `gorm:"column:avg_assignment_seconds;type:decimal(8,2);not null" json:"avg_assignment_seconds"`
	PostHandoffAcceptRate float64   `gorm:"column:post_handoff_accept_rate;type:decimal(5,4);not null" json:"post_handoff_accept_rate"`
	ECE                   float64   `gorm:"column:ece;type:decimal(5,4);not null" json:"ece"`
	TotalMessages         int       `gorm:"column:total_messages;not null" json:"total_messages"`
	AlertsTriggered       string    `gorm:"column:alerts_triggered;type:text;default:''" json:"alerts_triggered"`
	CreatedAt             time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

// TableName 表名
func (SLAMonitor) TableName() string { return "sla_monitors" }

// ABTest A/B 测试配置与统计
type ABTest struct {
	ID               int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TestID           string     `gorm:"column:test_id;uniqueIndex;size:64;not null" json:"test_id"`
	TestName         string     `gorm:"column:test_name;size:128;not null" json:"test_name"`
	Description      string     `gorm:"column:description;type:text;default:''" json:"description"`
	Status           string     `gorm:"column:status;size:32;not null;default:'draft';index:idx_ab_status,priority:1" json:"status"`
	TrafficSplit     JSONMap    `gorm:"column:traffic_split;type:jsonb;not null" json:"traffic_split"`
	TargetingRule    JSONMap    `gorm:"column:targeting_rule;type:jsonb;default:'{}'" json:"targeting_rule"`
	Metrics          JSONArray  `gorm:"column:metrics;type:jsonb;not null" json:"metrics"`
	ControlStats     JSONMap    `gorm:"column:control_stats;type:jsonb;default:'{}'" json:"control_stats"`
	TreatmentStats   JSONMap    `gorm:"column:treatment_stats;type:jsonb;default:'{}'" json:"treatment_stats"`
	MannWhitneyU     float64    `gorm:"column:mann_whitney_u;type:decimal(10,4);default:0" json:"mann_whitney_u"`
	MannWhitneyP     float64    `gorm:"column:mann_whitney_p;type:decimal(8,4);default:0" json:"mann_whitney_p"`
	BootstrapCILower float64    `gorm:"column:bootstrap_ci_lower;type:decimal(5,4);default:0" json:"bootstrap_ci_lower"`
	BootstrapCIUpper float64    `gorm:"column:bootstrap_ci_upper;type:decimal(5,4);default:0" json:"bootstrap_ci_upper"`
	BootstrapN       int        `gorm:"column:bootstrap_n;default:10000" json:"bootstrap_n"`
	StartedAt        *time.Time `gorm:"column:started_at;index:idx_ab_status,priority:2" json:"started_at,omitempty"`
	StoppedAt        *time.Time `gorm:"column:stopped_at" json:"stopped_at,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

// TableName 表名
func (ABTest) TableName() string { return "ab_tests" }

// ABTestMetric A/B 测试单条指标样本
//
// 独立时序表，避免 ABTest.ControlStats/TreatmentStats JSONB 字段膨胀
type ABTestMetric struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TestID     string    `gorm:"column:test_id;size:64;not null;index:idx_abm_test,priority:1" json:"test_id"`
	Group      string    `gorm:"column:group_name;size:16;not null;index:idx_abm_test,priority:2" json:"group_name"` // control / treatment
	MetricName string    `gorm:"column:metric_name;size:64;not null;index:idx_abm_test,priority:3" json:"metric_name"`
	Value      float64   `gorm:"column:value;type:decimal(15,6);not null" json:"value"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now();index:idx_abm_test,priority:4" json:"created_at"`
}

// TableName 表名
func (ABTestMetric) TableName() string { return "ab_test_metrics" }
