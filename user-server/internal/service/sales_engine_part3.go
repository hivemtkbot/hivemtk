// 拆分自 sales_engine.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"strings"
	"time"
)

// buildPrompt 构造 LLM prompt
func (e *SalesEngine) buildPrompt(
	req *SalesRequest,
	intent *dto.RecognizeResult,
	mem *model.DialogueMemory,
	sop *model.SOPAgent,
	stage string,
	ragChunks []RAGChunk,
	script *ScriptTemplate,
	customer *model.Customer,
) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("【客户消息】: %s\n\n", req.UserMessage))

	if customer != nil {
		name := customerNameOf(customer)
		if name != "" {
			sb.WriteString(fmt.Sprintf("【客户昵称】: %s\n", name))
		}
		if customer.Phone != "" {
			sb.WriteString(fmt.Sprintf("【客户手机】: %s\n", customer.Phone))
		}
	}

	if intent != nil {
		sb.WriteString(fmt.Sprintf("\n【识别意图】: %s (置信度 %.0f%%)\n",
			intent.IntentName, intent.Confidence*100))
	}

	if sop != nil {
		sb.WriteString(fmt.Sprintf("【适用 SOP】: %s\n", sop.Name))
	}
	if stage != "" && stage != "default" {
		sb.WriteString(fmt.Sprintf("【客户阶段】: %s\n", stage))
	}

	if len(ragChunks) > 0 {
		sb.WriteString("\n【知识库参考】:\n")
		for i, chunk := range ragChunks {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncate(chunk.Content, 200)))
		}
	}

	if script != nil {
		sb.WriteString(fmt.Sprintf("\n【话术参考】: %s\n", truncate(script.Content, 150)))
	}

	if mem != nil && len(mem.KeyFacts) > 0 {
		sb.WriteString("\n【关键事实】:\n")
		factsJSON, _ := json.Marshal(mem.KeyFacts)
		sb.WriteString(string(factsJSON))
		sb.WriteString("\n")
	}

	// 多轮对话历史（修复：原 prompt 仅依赖 DialogueMemory.KeyFacts，但 KeyFacts 的
	// 生产写入链路 AppendMessage/UpdateKeyFacts 从未在线上流程被调用，导致 AI 无任何
	// 上下文、自认“第一次对话”。此处直接从 session_messages 按会话取最近 N 轮注入，
	// 保证多轮上下文真实可用，且不依赖那条未接线的记忆管道。）
	if e.db != nil && req.SessionID != "" {
		var hist []model.SessionMessage
		if err := e.db.Where("session_id = ?", req.SessionID).
			Order("id desc").Limit(20).Find(&hist).Error; err == nil && len(hist) > 0 {
			// 去掉本轮用户消息（若最新一条即当前问题），仅保留历史
			if hist[0].Content == req.UserMessage {
				hist = hist[1:]
			}
			if len(hist) > 0 {
				for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
					hist[i], hist[j] = hist[j], hist[i]
				}
				var hb strings.Builder
				hb.WriteString(fmt.Sprintf("\n【对话历史（按时间顺序，共 %d 条）】\n", len(hist)))
				for _, m := range hist {
					role := "客户"
					if m.SenderType == "ai" || m.SenderType == "agent" {
						role = "AI"
					}
					hb.WriteString(fmt.Sprintf("%s：%s\n", role, m.Content))
				}
				sb.WriteString(hb.String())
			}
		}
	}

	sb.WriteString("\n【回复要求】:\n")
	sb.WriteString("1. 基于上述信息（含对话历史）生成回复，不要编造事实\n")
	sb.WriteString("2. 简洁（≤80 字），分自然段\n")
	sb.WriteString("3. 语气亲切、像真人对话\n")
	sb.WriteString("4. 若客户异议，按话术/SOP 引导\n")

	return sb.String()
}

// shouldTransferToHuman 是否应该转人工
//
// 升级：注入 ConfidenceAggregator 后改为基于 5 维信号 + 动态阈值的决策；
// 未注入时保留原有静态规则（IntentChurn/IntentComplaint/MessageCount>30）作为兜底
func (e *SalesEngine) shouldTransferToHuman(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory, req *SalesRequest) (bool, string) {
	if intent == nil {
		return false, ""
	}

	// [修复] 高意图置信度且非紧急/投诉/流失场景，直接走 AI 应答：
	// 避免 RAG/Logprob 等信号尚未填充时，聚合器将 0.9+ 的高意图置信度误判为 BandHandoff 而错误转人工。
	if intent.Confidence >= 0.7 &&
		intent.IntentType != IntentComplaint &&
		intent.IntentType != IntentChurn {
		return false, ""
	}

	// 注入 ConfidenceAggregator 时改用 5 维信号聚合
	if e.confidenceAggregator != nil {
		return e.shouldTransferByConfidence(ctx, intent, mem, req)
	}

	// 兼容：原有静态规则仅保留「投诉/流失倾向」转人工。
	// 注意：本项目定位为「替代人工」的 AI 客服，对话轮数过多不应触发转人工，
	// 因此移除原 mem.MessageCount > 30 的静态分支（2026-08-06 修复会话 2268 失声问题）。
	switch intent.IntentType {
	case IntentChurn, IntentComplaint:
		return true, e.transferReason(context.Background(), intent, mem)
	}
	return false, ""
}

// shouldTransferByConfidence 基于置信度聚合的转人工决策
//
// 输入：意图 + 记忆 + 请求
// 输出：(是否转人工, 原因)
func (e *SalesEngine) shouldTransferByConfidence(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory, req *SalesRequest) (bool, string) {
	if e.confidenceAggregator == nil {
		return false, ""
	}

	// 构造信号采集输入
	in := &dto.SignalCollectionInput{
		SessionID:     req.SessionID,
		CustomerID:    req.CustomerID,
		Text:          req.UserMessage,
		IntentType:    intent.IntentType,
		RawIntentConf: intent.Confidence,
		// 其他信号（RAG/Logprobs/Entities）由后续步骤填充后再聚合；
		// 本次预聚合使用 IntentConf 单维信号 + 记忆上下文
		CustomerLevel:     inferCustomerLevelFromReq(req),
		AgentAvailability: 0.5, // 默认中性
		LastTurns:         extractLastTurnsFromMem(mem),
	}

	dec, err := e.confidenceAggregator.Aggregate(ctx, in)
	if err != nil {
		// 聚合失败不阻断主流程，按原有静态规则兜底
		logger.Ctx(ctx).Warn().Err(err).Msg("[sales] confidence aggregate failed, fallback to static rule")
		switch intent.IntentType {
		case IntentChurn, IntentComplaint:
			return true, e.transferReason(context.Background(), intent, mem)
		}
		return false, ""
	}

	// 监控埋点：记录决策分布
	decisionLabel := ""
	switch dec.DecisionBand {
	case dto.BandHandoff:
		decisionLabel = "transfer"
	case dto.BandLLMFallback:
		decisionLabel = "llm_fallback"
	case dto.BandReview:
		decisionLabel = "low_confidence_hold"
	default:
		decisionLabel = "auto_reply"
	}
	_ = decisionLabel // 私域: 无 Prometheus, 记录已落库

	// 一票否决
	if dec.VetoTriggered != "" {
		return true, "一票否决: " + dec.VetoTriggered
	}

	// 按决策区间
	switch dec.DecisionBand {
	case dto.BandHandoff:
		return true, fmt.Sprintf("低置信度转人工 (aggregated=%.2f < threshold=%.2f)", dec.AggregatedConf, dec.DynamicThreshold)
	case dto.BandLLMFallback, dto.BandReview:
		// 中间区间：继续走主流程（LLM 兜底/审核），不强制转人工
		return false, ""
	default:
		return false, ""
	}
}

// inferCustomerLevelFromReq 推断客户等级（从请求或客户档案）
func inferCustomerLevelFromReq(req *SalesRequest) string {
	if req == nil || req.Config == nil {
		return "normal"
	}
	if req.Config.CustomerLevel != "" {
		return req.Config.CustomerLevel
	}
	return "normal"
}

// extractLastTurnsFromMem 从记忆中提取最近 3 轮对话文本
func extractLastTurnsFromMem(mem *model.DialogueMemory) []string {
	if mem == nil || len(mem.KeyFacts) == 0 {
		return nil
	}
	out := make([]string, 0, 3)
	for k, v := range mem.KeyFacts {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, k+": "+s)
			if len(out) >= 3 {
				break
			}
		}
	}
	return out
}

// transferReason 转人工原因（兼容旧调用方）
func (e *SalesEngine) transferReason(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory) string {
	if intent != nil {
		switch intent.IntentType {
		case IntentChurn:
			return "客户出现流失倾向，建议人工介入挽留"
		case IntentComplaint:
			return "客户正在投诉，需要人工处理"
		}
	}
	if mem != nil && mem.MessageCount > 30 {
		return "对话轮数过多，建议人工接管"
	}
	return "触发转人工规则"
}

// ms 计算耗时（毫秒）
func ms(start time.Time) int {
	return int(time.Since(start).Milliseconds())
}

// safeID 安全返回客户 ID
func safeID(c *model.Customer) string {
	if c == nil {
		return ""
	}
	return c.ID
}

// truncateForReply 把长文本截断成适合作为访客回复的长度
// 仅在 UTF-8 rune 粒度上截断，避免切到半个汉字
func truncateForReply(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// customerNameOf 提取客户名
// Customer 模型只有身份标识（Phone/Email/OpenID），没有昵称字段
// 优先返回手机号作为标识
func customerNameOf(c *model.Customer) string {
	if c == nil {
		return ""
	}
	return c.Phone
}

// ============================================================================
// 多渠道统一入口（Phase 1.3：渠道无关化）
// ----------------------------------------------------------------------------
// 不同渠道（企微/WhatsApp/Telegram/飞书）的入站消息，经 WebhookService 统一
// 编排后，调用本入口进入 智能体。本方法负责：
//   1) 渠道特定的消息清洗（飞书的 v1/v2 字段映射、WhatsApp 的 number/contact 名）
//   2) 构造渠道特定的会话 ID（保证同一用户多渠道隔离）
//   3) 透传到 Handle 执行 7 步链路
// ============================================================================

// ChannelMessage 渠道无关的入站消息（WebhookService → SalesEngine 桥接）
type ChannelMessage struct {
	Channel      string `json:"channel"`       // 渠道标识：wecom/whatsapp/telegram/feishu
	AccountID    string `json:"account_id"`    // 渠道账号 ID
	ExternalUser string `json:"external_user"` // 外部用户 ID
	Nickname     string `json:"nickname"`      // 用户昵称（可选）
	Content      string `json:"content"`       // 消息文本
	MsgType      string `json:"msg_type"`      // text/image/event/...
	ChatID       string `json:"chat_id"`       // 会话 ID（群为 group_id）
	IsGroup      bool   `json:"is_group"`      // 是否群消息
	MediaURL     string `json:"media_url"`     // 媒体 URL（可选）
	RawData      string `json:"raw_data"`      // 原始 payload（可选）
	ReceivedAt   int64  `json:"received_at"`   // 毫秒时间戳
}

// ProcessIncomingMessage 多渠道统一入口
func (e *SalesEngine) ProcessIncomingMessage(ctx context.Context, msg *ChannelMessage) (*SalesResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}
	if msg.Content == "" && msg.MediaURL == "" {
		// 没有有效内容的事件型消息直接跳过
		return &SalesResponse{}, nil
	}

	// 渠道特定清洗
	content, sessionID, customerID := e.normalizeChannelMessage(context.Background(), msg)

	// 构造 SalesRequest
	req := &SalesRequest{
		SessionID:   sessionID,
		CustomerID:  customerID,
		OneID:       customerID,
		UserMessage: content,
		Platform:    msg.Channel,
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}

	// 平台相关的人设调整
	if msg.Channel == "feishu" {
		req.Config.Persona = "你是飞书上的企业助手，回复简洁专业。"
	} else if msg.Channel == "telegram" {
		req.Config.Persona = "你是 Telegram 上的销售助手，语气亲切。"
	}

	return e.Handle(ctx, req)
}

// normalizeChannelMessage 渠道特定清洗 → 通用字段
func (e *SalesEngine) normalizeChannelMessage(ctx context.Context, msg *ChannelMessage) (content, sessionID, customerID string) {
	// 内容
	if msg.MediaURL != "" {
		switch msg.MsgType {
		case "image":
			content = "[图片] " + msg.Content
		case "voice":
			content = "[语音] " + msg.Content
		case "video":
			content = "[视频] " + msg.Content
		case "file":
			content = "[文件] " + msg.Content
		default:
			content = msg.Content
		}
	} else {
		content = msg.Content
	}
	// 会话 ID
	if msg.ChatID != "" {
		sessionID = msg.Channel + ":" + msg.ChatID
	} else {
		sessionID = msg.Channel + ":" + msg.AccountID + ":" + msg.ExternalUser
	}
	// 客户 ID（OneID）
	customerID = msg.Channel + ":" + msg.ExternalUser
	return
}

// stageToJourneyStage SOP 阶段字符串 → JourneyStage
// 商业产品级：把 SOP 引擎的语义阶段映射到客户旅程的标准化阶段
// 便于话术库按客户实际所处阶段精准推荐
func stageToJourneyStage(stage string) JourneyStage {
	switch stage {
	case "churn_risk":
		return StageSleeping
	case "active":
		return StageContact
	case "default":
		return StageLead
	default:
		return StageLead
	}
}

// recordFeedback 记录反馈学习快照（SalesEngine 主链路第 9 步）
// ----------------------------------------------------------------------------
// 商业产品级 AI 自我进化闭环：
//
//	每次 Handle 结束都把"本次决策快照"喂给 FeedbackLearner，包括
//	intent/confidence/SOP/AIReply/是否转人工/token/耗时。
//	CustomerAccept 默认 false（生成时尚未知客户是否接受），
//	后续 SmartCSOrchestrator 在客户下一条消息或人工接管时更新。
//
// 设计原则：
//   - feedbackLearner 为 nil 时静默跳过（不破坏现有链路）
//   - 所有 return 路径都经过 defer，确保不遗漏
//   - 记录失败不影响主流程（仅 log）
func (e *SalesEngine) recordFeedback(ctx context.Context, req *SalesRequest, resp *SalesResponse) {
	if e.feedbackLearner == nil {
		return
	}
	if req == nil || resp == nil {
		return
	}
	record := &FeedbackRecord{
		SessionID:      req.SessionID,
		CustomerID:     req.CustomerID,
		AIReply:        resp.Reply,
		Transferred:    resp.TransferredToHuman,
		TransferReason: resp.TransferReason,
		Tokens:         resp.CostTokens,
		LatencyMs:      resp.LatencyMs,
	}
	if resp.Intent != nil {
		record.IntentType = resp.Intent.IntentType
		record.Confidence = resp.Intent.Confidence
	}
	if resp.MatchedSOP != nil {
		record.SOPName = resp.MatchedSOP.Name
	}
	if err := e.feedbackLearner.RecordFeedback(ctx, record); err != nil {
		// 记录失败不影响主流程，仅打 log 便于排查
		logger.Errorf("[SalesEngine] feedback learner record failed: %v", err)
	}
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step:   "9_feedback_learn",
		Status: "ok",
		Detail: fmt.Sprintf("intent=%s conf=%.2f sop=%s transferred=%v tokens=%d",
			record.IntentType, record.Confidence, record.SOPName, record.Transferred, record.Tokens),
	})
}
