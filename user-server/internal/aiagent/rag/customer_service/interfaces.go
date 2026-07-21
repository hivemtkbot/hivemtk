package ragcustomerservice

import (
	"context"
	"time"
)

// Session 会话结构
type Session struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	Platform     string         `json:"platform"`
	KBID         string         `json:"kb_id"`
	Status       SessionStatus  `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Metadata     map[string]any `json:"metadata"`
	Conversation *Conversation  `json:"conversation"`
	Config       SessionConfig  `json:"config"`
}

// SessionStatus 会话状态
type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionPaused  SessionStatus = "paused"
	SessionClosed  SessionStatus = "closed"
	SessionExpired SessionStatus = "expired"
)

// Message 消息结构
type Message struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"session_id"`
	Role       MessageRole    `json:"role"` // user, assistant, system
	Content    string         `json:"content"`
	Timestamp  time.Time      `json:"timestamp"`
	Metadata   map[string]any `json:"metadata"`
	References []string       `json:"references"` // 引用的文档ID
}

// MessageRole 消息角色
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

// Response 回复结构
type Response struct {
	ID             string         `json:"id"`
	SessionID      string         `json:"session_id"`
	Content        string         `json:"content"`
	Intent         string         `json:"intent"`
	Confidence     float64        `json:"confidence"`
	References     []Reference    `json:"references"`
	Metadata       map[string]any `json:"metadata"`
	Timestamp      time.Time      `json:"timestamp"`
	ProcessingTime time.Duration  `json:"processing_time"`
	Source         string         `json:"source"` // rag, rule, fallback
}

// Reference 引用信息
type Reference struct {
	DocumentID string  `json:"document_id"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
	ChunkIndex int     `json:"chunk_index"`
}

// Feedback 用户反馈
type Feedback struct {
	SessionID  string         `json:"session_id"`
	ResponseID string         `json:"response_id"`
	Rating     int            `json:"rating"` // 1-5星评分
	IsHelpful  bool           `json:"is_helpful"`
	Comment    string         `json:"comment"`
	Metadata   map[string]any `json:"metadata"`
}

// SessionConfig 会话配置
type SessionConfig struct {
	MaxHistoryLength int     `json:"max_history_length"` // 最大历史记录长度
	Timeout          int     `json:"timeout"`            // 会话超时时间(秒)
	Temperature      float64 `json:"temperature"`        // LLM温度参数
	MaxTokens        int     `json:"max_tokens"`         // 最大token数
	SystemPrompt     string  `json:"system_prompt"`      // 系统提示词
	EnableContextual bool    `json:"enable_contextual"`  // 是否启用上下文理解
	EnableLearning   bool    `json:"enable_learning"`    // 是否启用学习功能
	EnableFallback   bool    `json:"enable_fallback"`    // 是否启用回退机制
}

// Conversation 对话结构
type Conversation struct {
	Messages []Message      `json:"messages"`
	Context  Context        `json:"context"`
	Metadata map[string]any `json:"metadata"`
}

// Context 对话上下文
type Context struct {
	Topic           string              `json:"topic"`            // 当前话题
	Intent          string              `json:"intent"`           // 用户意图
	Entities        map[string][]string `json:"entities"`         // 实体识别
	Sentiment       Sentiment           `json:"sentiment"`        // 情感分析
	PreviousTopics  []string            `json:"previous_topics"`  // 历史话题
	UserPreferences map[string]any      `json:"user_preferences"` // 用户偏好
	SessionContext  map[string]any      `json:"session_context"`  // 会话特定上下文
	LastInteraction time.Time           `json:"last_interaction"` // 最后交互时间
}

// IntentAnalysis 意图分析结果
type IntentAnalysis struct {
	PrimaryIntent    string            `json:"primary_intent"`
	SecondaryIntents []string          `json:"secondary_intents"`
	Confidence       float64           `json:"confidence"`
	Parameters       map[string]string `json:"parameters"`
	Categories       []string          `json:"categories"`
}

// Sentiment 情感分析
type Sentiment struct {
	Score    float64   `json:"score"` // -1 到 1，负值表示负面情感
	Label    string    `json:"label"` // positive, negative, neutral
	Emotions []Emotion `json:"emotions"`
}

// Emotion 情感
type Emotion struct {
	Type  string  `json:"type"` // joy, anger, sadness, fear, surprise
	Score float64 `json:"score"`
}

// SessionMetrics 会话指标
type SessionMetrics struct {
	SessionID        string    `json:"session_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	MessageCount     int       `json:"message_count"`
	AvgResponseTime  float64   `json:"avg_response_time"` // 平均响应时间(毫秒)
	ResolutionRate   float64   `json:"resolution_rate"`   // 解决率
	UserSatisfaction float64   `json:"user_satisfaction"` // 用户满意度
	FeedbackCount    int       `json:"feedback_count"`
}

// RagCustomerService RAG客服服务接口
type RagCustomerService interface {
	// ProcessMessage 处理用户消息
	ProcessMessage(ctx context.Context, session Session, message Message) (Response, error)

	// CreateSession 创建新会话
	CreateSession(ctx context.Context, userID, platform, kbID string, config SessionConfig) (*Session, error)

	// GetSession 获取会话信息
	GetSession(ctx context.Context, sessionID string) (*Session, error)

	// UpdateSession 更新会话信息
	UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error

	// EndSession 结束会话
	EndSession(ctx context.Context, sessionID string) error

	// GetSessionHistory 获取会话历史
	GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]Message, error)

	// ProcessBatchMessages 批量处理消息
	ProcessBatchMessages(ctx context.Context, session Session, messages []Message) ([]Response, error)

	// UpdateKnowledge 更新知识库
	UpdateKnowledge(ctx context.Context, kbID string, feedback Feedback) error

	// GetSessionMetrics 获取会话指标
	GetSessionMetrics(ctx context.Context, sessionID string) (*SessionMetrics, error)

	// ListSessions 列出会话
	ListSessions(ctx context.Context, userID, platform string, status SessionStatus) ([]Session, error)

	// GetUserContext 获取用户上下文
	GetUserContext(ctx context.Context, userID, platform string) (*Context, error)
}

// DialogManagerInterface 对话管理器接口
type DialogManagerInterface interface {
	// CreateSession 创建会话
	CreateSession(ctx context.Context, userID, platform, kbID string, config SessionConfig) (*Session, error)

	// GetSession 获取会话
	GetSession(ctx context.Context, sessionID string) (*Session, error)

	// AddMessage 添加消息到会话
	AddMessage(ctx context.Context, sessionID string, message Message) error

	// GetConversationHistory 获取对话历史
	GetConversationHistory(ctx context.Context, sessionID string, limit int) (*Conversation, error)

	// UpdateSessionMetadata 更新会话元数据
	UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]any) error

	// CloseSession 关闭会话
	CloseSession(ctx context.Context, sessionID string) error

	// CleanupExpiredSessions 清理会话
	CleanupExpiredSessions(ctx context.Context) error

	// ListUserSessions 列出用户会话
	ListUserSessions(ctx context.Context, userID, platform string, status SessionStatus) ([]Session, error)
}

// ContextUnderstandingInterface 上下文理解器接口
type ContextUnderstandingInterface interface {
	// AnalyzeIntent 分析用户意图
	AnalyzeIntent(ctx context.Context, message string, history []Message) (IntentAnalysis, error)

	// ExtractEntities 提取实体
	ExtractEntities(ctx context.Context, message string) (map[string][]string, error)

	// AnalyzeSentiment 情感分析
	AnalyzeSentiment(ctx context.Context, message string) (Sentiment, error)

	// UpdateContext 更新上下文
	UpdateContext(ctx context.Context, currentContext Context, newMessage Message, intent IntentAnalysis) (Context, error)

	// DetectTopicChange 检测话题变更
	DetectTopicChange(ctx context.Context, currentTopic string, newMessage Message) (bool, string, error)

	// GetUserPreferences 获取用户偏好
	GetUserPreferences(ctx context.Context, userID, platform string) (map[string]any, error)

	// UpdateUserPreferences 更新用户偏好
	UpdateUserPreferences(ctx context.Context, userID, platform string, preferences map[string]any) error
}

// ResponseGeneratorInterface 回复生成器接口
type ResponseGeneratorInterface interface {
	GenerateResponse(ctx context.Context, request ResponseGenerationRequest) (string, error)
	GenerateStructuredResponse(ctx context.Context, request ResponseGenerationRequest, schema any) (any, error)
	BuildContextString(results []any, context Context) string
	BuildResponsePrompt(query, contextStr string, session Session, context Context) string
}

// ResponseGenerationRequest 回复生成请求
type ResponseGenerationRequest struct {
	Query         string        `json:"query"`
	Context       Context       `json:"context"`
	SearchResults []any         `json:"search_results"`
	Session       Session       `json:"session"`
	Config        SessionConfig `json:"config"`
	LLMConfig     any           `json:"llm_config"`
}

// QualityAssessmentInterface 质量评估接口
type QualityAssessmentInterface interface {
	EvaluateResponse(ctx context.Context, responseContent, query string, searchResults []any) (float64, error)
	EvaluateRelevance(ctx context.Context, response, query string) (float64, error)
	EvaluateAccuracy(ctx context.Context, response string, referenceSources []string) (float64, error)
	EvaluateCoherence(ctx context.Context, response string) (float64, error)
	GetQualityMetrics(ctx context.Context, sessionID string) (QualityMetrics, error)
}

// QualityMetrics 质量指标
type QualityMetrics struct {
	SessionID      string    `json:"session_id"`
	AvgRelevance   float64   `json:"avg_relevance"`
	AvgAccuracy    float64   `json:"avg_accuracy"`
	AvgCoherence   float64   `json:"avg_coherence"`
	ResolutionRate float64   `json:"resolution_rate"`
	UpdateTime     time.Time `json:"update_time"`
}

// FeedbackLearningInterface 反馈学习接口
type FeedbackLearningInterface interface {
	ProcessFeedback(ctx context.Context, feedback Feedback) error
	LearnFromFeedback(ctx context.Context, feedback Feedback) error
	GetLearningInsights(ctx context.Context, sessionID string) ([]LearningInsight, error)
	UpdateKnowledgeBase(ctx context.Context, kbID string, insights []LearningInsight) error
}

// LearningInsight 学习洞察
type LearningInsight struct {
	Type           string    `json:"type"`           // content_improvement, intent_recognition, context_management
	Description    string    `json:"description"`    // 洞察描述
	Confidence     float64   `json:"confidence"`     // 置信度
	Recommendation string    `json:"recommendation"` // 建议
	Source         string    `json:"source"`         // 洞察来源
	CreatedAt      time.Time `json:"created_at"`
}
