package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 商业产品级 销售动作触发器（Sales Action Trigger）
// ----------------------------------------------------------------------------
// 商业市场需求（按真实用户使用场景）：
//   销售每天接触 50+ 客户，每条对话涉及多个连锁动作：
//     1. AI 识别"询价" → 自动打"价格敏感"标签 + 推进到"意向"阶段
//     2. AI 识别"准备购买" → 推进到"报价" + 自动生成跟进
//     3. AI 提取到"产品+价格" → 自动生成订单意向
//     4. 销售完成跟进 → 推进到下一阶段
//     5. 订单创建 → 推进到"成交"+ 触发售后 SOP
//
// 闭门造车 vs 实际场景的差距：
//   原实现：每个组件独立，无联动。销售需要手动操作 5 个不同页面才能完成"客户
//   从陌生到成交"的流程。必然遗漏。
//   修复：动作触发器统一编排 5 个组件，按真实业务流自动联动。
// ============================================================================

// SalesActionTrigger 销售动作触发器
// 把 AI 谈单 / 跟进完成 / 订单创建 三个核心事件
// 自动分发到 标签 / 旅程 / 跟进 / 订单 / 仪表盘 五个下游组件
type SalesActionTrigger struct {
	mu sync.Mutex

	// 五个下游组件（依赖注入）
	tagger       *AITagger
	journey      *CustomerJourneyService
	followup     *FollowUpService
	extractor    *OrderIntentExtractor
	dashboard    *SalesDashboard
	draftService *OrderDraftService // P1-CLOSE-11 订单草稿

	// 默认销售（可被业务 owner 覆盖）
	defaultOwnerID string

	// 触发历史（用于审计 + 测试）
	history []TriggerRecord
}

// TriggerRecord 触发记录
type TriggerRecord struct {
	EventType  string          `json:"event_type"` // sales_response / followup_completed / order_created
	CustomerID string          `json:"customer_id"`
	Actions    []TriggerAction `json:"actions"` // 触发的具体动作
	OccurredAt time.Time       `json:"occurred_at"`
	Metadata   map[string]any  `json:"metadata,omitempty"`
}

// TriggerAction 单个触发动作
type TriggerAction struct {
	Action     string `json:"action"`     // tag_applied / stage_advanced / followup_scheduled / order_intent_extracted / dashboard_recorded
	Target     string `json:"target"`     // 目标对象（如"behavior:price_sensitive" / "interested"）
	Confidence int    `json:"confidence"` // 0-100
	Result     string `json:"result"`     // ok / skip / fail
	Message    string `json:"message,omitempty"`
}

// TriggerConfig 触发器配置
type TriggerConfig struct {
	DefaultOwnerID string
}

// NewSalesActionTrigger 创建销售动作触发器
func NewSalesActionTrigger(
	tagger *AITagger,
	journey *CustomerJourneyService,
	followup *FollowUpService,
	extractor *OrderIntentExtractor,
	dashboard *SalesDashboard,
	cfg *TriggerConfig,
) *SalesActionTrigger {
	if cfg == nil {
		cfg = &TriggerConfig{DefaultOwnerID: "system"}
	}
	if dashboard == nil {
		dashboard = NewSalesDashboard(journey)
	}
	return &SalesActionTrigger{
		tagger:         tagger,
		journey:        journey,
		followup:       followup,
		extractor:      extractor,
		dashboard:      dashboard,
		defaultOwnerID: cfg.DefaultOwnerID,
	}
}

// SetDraftService 注入订单草稿服务（P1-CLOSE-11）
func (t *SalesActionTrigger) SetDraftService(ctx context.Context, svc *OrderDraftService)  {
	t.draftService = svc
}

// TriggerAfterSales AI 谈单响应后触发（核心入口）
// 商业产品级业务流：
//  1. 自动打标签（行为 / 兴趣 / 阶段）
//  2. 自动推进客户旅程（基于意图）
//  3. 自动提取订单意向（从客户消息 + AI 回复）
//  4. 自动安排跟进（基于阶段）
//  5. 记录到销售仪表盘
func (t *SalesActionTrigger) TriggerAfterSales(ctx context.Context, customerID, ownerID string, resp *SalesResponse) *TriggerRecord {
	if customerID == "" || resp == nil {
		return nil
	}
	if ownerID == "" {
		ownerID = t.defaultOwnerID
	}
	rec := &TriggerRecord{
		EventType:  "sales_response",
		CustomerID: customerID,
		Actions:    make([]TriggerAction, 0, 8),
		OccurredAt: time.Now(),
	}

	// === 1. 自动打标签 ===
	if t.tagger != nil {
		tags := t.tagger.TagFromSalesResponse(ctx, customerID, resp)
		for _, tag := range tags {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action:     "tag_applied",
				Target:     tag.Tag,
				Confidence: int(tag.Confidence * 100),
				Result:     "ok",
				Message:    fmt.Sprintf("source=%s category=%s", tag.Source, tag.Category),
			})
		}
		if len(tags) == 0 {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action: "tag_applied", Result: "skip", Message: "无标签可打",
			})
		}
	}

	// === 2. 自动推进客户旅程 ===
	if t.journey != nil {
		newStage := t.advanceJourneyByIntent(ctx, customerID, resp)
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "stage_advanced", Target: string(newStage), Result: "ok",
			Message: fmt.Sprintf("intent=%s", safeIntent(resp)),
		})
	}

	// === 3. 自动提取订单意向 → 自动生成订单草稿 ===
	// P1-CLOSE-11 关键升级：不再只是记录"提取到意图"的动作，
	// 而是真正生成"待确认草稿"，销售一键确认即可下单
	if t.extractor != nil {
		// 优先从记忆中的客户信息提取，最后兜底用 reply
		textToExtract := resp.Reply
		if resp.Memory != nil {
			if resp.Memory.Demand != "" {
				textToExtract = resp.Memory.Demand + " " + textToExtract
			}
			if resp.Memory.Budget != "" {
				textToExtract = resp.Memory.Budget + " " + textToExtract
			}
		}
		intents := t.extractor.ExtractFromText(ctx, customerID, textToExtract)
		for _, in := range intents {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action:     "order_intent_extracted",
				Target:     in.ProductName,
				Confidence: int(in.Confidence * 100),
				Result:     "ok",
				Message:    fmt.Sprintf("amount=%.2f qty=%d", in.TotalAmount, in.Quantity),
			})
			// 3.1 关键：自动创建草稿（P1-CLOSE-11）
			if t.draftService != nil {
				draft := t.draftService.CreateFromIntent(ctx, &in, ownerID)
				if draft != nil {
					rec.Actions = append(rec.Actions, TriggerAction{
						Action:     "order_draft_created",
						Target:     draft.ID,
						Confidence: int(draft.Confidence * 100),
						Result:     "ok",
						Message: fmt.Sprintf("product=%s amount=%.2f expires_at=%s",
							draft.ProductName, draft.TotalAmount, draft.ExpiresAt.Format("2006-01-02")),
					})
				}
			}
		}
		if len(intents) == 0 {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action: "order_intent_extracted", Result: "skip", Message: "无产品/价格信号",
			})
		}
	}

	// === 4. 自动安排跟进（仅当旅程到达关键阶段或转人工时） ===
	if t.followup != nil {
		shouldSchedule := false
		scheduleType := ReminderFirstContact
		scheduleIn := 1 * time.Hour
		priority := PriorityNormal

		if resp.TransferredToHuman {
			// 转人工 → 销售必须在 30 分钟内接管
			shouldSchedule = true
			scheduleType = ReminderFirstContact
			scheduleIn = 30 * time.Minute
			priority = PriorityUrgent
		} else if resp.Intent != nil {
			switch resp.Intent.IntentType {
			case IntentPurchase:
				// 客户已表示想买 → 1 小时内逼单
				shouldSchedule = true
				scheduleType = ReminderQuoteFollowup
				scheduleIn = 1 * time.Hour
				priority = PriorityHigh
			case IntentPriceInquiry, IntentAskProduct:
				// 客户在咨询 → 2 小时内提供报价
				shouldSchedule = true
				scheduleType = ReminderQuoteFollowup
				scheduleIn = 2 * time.Hour
				priority = PriorityHigh
			case IntentObjectionPrice:
				// 价格异议 → 4 小时内异议处理
				shouldSchedule = true
				scheduleType = ReminderFirstContact
				scheduleIn = 4 * time.Hour
				priority = PriorityHigh
			}
		}

		if shouldSchedule {
			r, err := t.followup.Schedule(ctx, customerID, ownerID, scheduleType, scheduleIn, &ScheduleOptions{
				Title:       "AI 触发跟进: " + string(scheduleType),
				Description: "AI 识别客户" + safeIntent(resp) + "后自动安排",
				Priority:    priority,
				AutoHandle:  false,
			})
			if err == nil && r != nil {
				rec.Actions = append(rec.Actions, TriggerAction{
					Action: "followup_scheduled", Target: r.ID, Result: "ok",
					Message: fmt.Sprintf("type=%s due_in=%v", scheduleType, scheduleIn),
				})
			}
		} else {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action: "followup_scheduled", Result: "skip", Message: "当前意图无需新跟进",
			})
		}
	}

	// === 5. 记录到销售仪表盘 ===
	if t.dashboard != nil {
		t.dashboard.RecordAIDeal(ctx, AIDealEvent{
			CustomerID:  customerID,
			OwnerID:     ownerID,
			Intent:      safeIntent(resp),
			Replied:     resp.Reply != "" && !resp.TransferredToHuman,
			Transferred: resp.TransferredToHuman,
			CostTokens:  resp.CostTokens,
			LatencyMs:   resp.LatencyMs,
			OccurredAt:  time.Now(),
		})
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "dashboard_recorded", Target: "ai_deal", Result: "ok",
		})
	}

	// 记录到历史
	t.mu.Lock()
	t.history = append(t.history, *rec)
	if len(t.history) > 1000 {
		t.history = t.history[len(t.history)-1000:]
	}
	t.mu.Unlock()

	return rec
}

// advanceJourneyByIntent 基于意图自动推进客户旅程
// 商业产品级业务流：
//
//	陌生 → 留资：客户表示愿意加微
//	留资 → 初步接触：客户已回复 AI 多轮
//	初步接触 → 意向：客户咨询产品/价格
//	意向 → 报价：AI 提供了价格方案
//	报价 → 成交：客户明确购买（IntentPurchase）
//	任意 → 流失：客户投诉/明确流失
func (t *SalesActionTrigger) advanceJourneyByIntent(ctx context.Context, customerID string, resp *SalesResponse) JourneyStage {
	if resp == nil {
		return ""
	}
	state := t.journey.GetState(ctx, customerID)
	current := state.CurrentStage
	if current == "" {
		current = StageStranger
	}
	intent := ""
	if resp.Intent != nil {
		intent = resp.Intent.IntentType
	}

	// 计算目标阶段
	var target JourneyStage
	switch intent {
	case IntentPurchase:
		// 准备购买 → 推进到"意向"或"报价"（取决于当前）
		if current == StageStranger || current == StageLead {
			target = StageInterested
		} else if current == StageContact {
			target = StageQuoted
		} else {
			target = current
		}
	case IntentPriceInquiry, IntentAskProduct, IntentObjectionPrice:
		// 咨询 → 推进到"意向"
		switch current {
		case StageStranger:
			target = StageLead
		case StageLead, StageContact:
			target = StageInterested
		default:
			target = current
		}
	case IntentGreeting, IntentSocial:
		// 闲聊 → 至少从"陌生"到"留资"
		if current == StageStranger {
			target = StageLead
		} else {
			target = current
		}
	case IntentChurn, IntentComplaint:
		// 流失/投诉 → 推进到"留资"标记+告警，不进入流失（仍需销售挽留）
		// 实际转入由销售人工操作
		target = current
	default:
		// 未知意图不推进
		target = current
	}

	// 目标 = 当前 → 跳过
	if target == current {
		return current
	}

	// 推进旅程
	_, err := t.journey.Transition(ctx, customerID, target, "ai_chat", "ai",
		"AI 意图自动推进: "+intent, map[string]any{
			"intent":      intent,
			"confidence":  safeConf(resp),
			"transferred": resp.TransferredToHuman,
		})
	if err != nil {
		return current
	}
	return target
}

// TriggerAfterFollowUp 跟进完成时触发
// 商业产品级业务流：
//
//	销售在跟进待办里点击"已完成"
//	  → 记录互动到客户旅程
//	  → 若客户表示"已购买" → 推进到"成交" + 触发售后 SOP
//	  → 若客户表示"无意向" → 推进到"流失"
//	  → 否则 → 推进到下一阶段（按销售操作）
//	  → 记录到销售仪表盘
func (t *SalesActionTrigger) TriggerAfterFollowUp(ctx context.Context, reminderID, customerID, ownerID, result string) *TriggerRecord {
	rec := &TriggerRecord{
		EventType:  "followup_completed",
		CustomerID: customerID,
		Actions:    make([]TriggerAction, 0, 4),
		OccurredAt: time.Now(),
	}

	// 1. 推进客户旅程（记录互动 + 根据 result 推进阶段）
	if t.journey != nil {
		t.journey.Touch(ctx, customerID, "followup")
		target := t.advanceJourneyByFollowUpResult(ctx, customerID, result)
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "stage_advanced", Target: string(target), Result: "ok",
			Message: "followup result: " + result,
		})
	}

	// 2. 记录到销售仪表盘
	if t.dashboard != nil {
		t.dashboard.RecordFollowUp(ctx, FollowUpEvent{
			CustomerID: customerID,
			OwnerID:    ownerID,
			Channel:    "manual",
			IsAI:       false,
			Result:     result,
			OccurredAt: time.Now(),
		})
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "dashboard_recorded", Target: "followup", Result: "ok",
		})
	}

	// 3. 成交 → 自动触发售后跟进
	if result == "converted" && t.followup != nil {
		r, err := t.followup.Schedule(ctx, customerID, ownerID, ReminderAfterSaleCare, 7*24*time.Hour, &ScheduleOptions{
			Title:       "售后回访",
			Description: "成交 7 天后自动安排",
			Priority:    PriorityNormal,
			AutoHandle:  true,
		})
		if err == nil && r != nil {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action: "followup_scheduled", Target: r.ID, Result: "ok",
				Message: "售后回访自动安排",
			})
		}
	}

	// 4. 推进到成交 → 把客户旅程也推到"成交"（不依赖销售手动）
	if result == "converted" && t.journey != nil {
		_, _ = t.journey.Transition(ctx, customerID, StageWon, "followup", ownerID,
			"销售跟进成交", nil)
	}

	// 5. 成交 → 在仪表盘记录订单（销售跟进→成单）
	// 关键：原实现只推进旅程+记录跟进，仪表盘的销售业绩(订单数/金额)为 0
	// 修复：跟进结果=converted 时，自动补一条订单事件（金额/产品从上下文推断）
	if result == "converted" && t.dashboard != nil {
		amount, productName := t.inferOrderFromJourney(ctx, customerID)
		t.dashboard.RecordOrder(ctx, OrderEvent{
			OrderID:     "followup-" + reminderID,
			CustomerID:  customerID,
			OwnerID:     ownerID,
			Amount:      amount,
			ProductName: productName,
			IsAIHandled: false,
			OrderedAt:   time.Now(),
		})
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "order_recorded", Target: "from_followup", Result: "ok",
			Message: fmt.Sprintf("amount=%.2f product=%s", amount, productName),
		})
	}

	// 记录历史
	t.mu.Lock()
	t.history = append(t.history, *rec)
	if len(t.history) > 1000 {
		t.history = t.history[len(t.history)-1000:]
	}
	t.mu.Unlock()

	return rec
}

// advanceJourneyByFollowUpResult 基于跟进结果推进旅程
func (t *SalesActionTrigger) advanceJourneyByFollowUpResult(ctx context.Context, customerID, result string) JourneyStage {
	state := t.journey.GetState(ctx, customerID)
	current := state.CurrentStage
	if current == "" {
		current = StageStranger
	}
	var target JourneyStage
	switch result {
	case "converted":
		// 成交：推到 Won
		target = StageWon
	case "lost", "rejected":
		// 流失：推到 Lost
		target = StageLost
	case "no_reply", "objection":
		// 无回复/异议：保持当前阶段（让 SOP 继续）
		target = current
	default:
		target = current
	}
	if target != current {
		_, _ = t.journey.Transition(ctx, customerID, target, "followup", "sales",
			"跟进结果驱动: "+result, map[string]any{"result": result})
	}
	return target
}

// TriggerAfterOrder 订单创建后触发
// 商业产品级业务流：
//
//	订单创建 → 客户旅程推到"成交"
//	        → 触发售后 SOP（7 天后回访）
//	        → 记录到销售仪表盘
//	        → 重置 RFM 分数
//
// isAIHandled（可选）：true=AI 独立成单（销售未介入）；false/未传=人工成单
func (t *SalesActionTrigger) TriggerAfterOrder(ctx context.Context, orderID, customerID, ownerID string, amount float64, productName string, isAIHandled ...bool) *TriggerRecord {
	aiHandled := false
	if len(isAIHandled) > 0 {
		aiHandled = isAIHandled[0]
	}
	rec := &TriggerRecord{
		EventType:  "order_created",
		CustomerID: customerID,
		Actions:    make([]TriggerAction, 0, 4),
		OccurredAt: time.Now(),
	}

	// 1. 推进到"成交"
	if t.journey != nil {
		_, _ = t.journey.Transition(ctx, customerID, StageWon, "order", ownerID,
			"订单创建自动推进: "+productName, map[string]any{
				"order_id": orderID, "amount": amount, "ai_handled": aiHandled,
			})
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "stage_advanced", Target: string(StageWon), Result: "ok",
		})
	}

	// 2. 触发售后跟进（7 天后）
	if t.followup != nil {
		r, err := t.followup.Schedule(ctx, customerID, ownerID, ReminderAfterSaleCare, 7*24*time.Hour, &ScheduleOptions{
			Title:       "售后回访: " + productName,
			Description: "订单 " + orderID + " 创建后自动安排",
			Priority:    PriorityNormal,
			AutoHandle:  true,
		})
		if err == nil && r != nil {
			rec.Actions = append(rec.Actions, TriggerAction{
				Action: "followup_scheduled", Target: r.ID, Result: "ok",
			})
		}
	}

	// 3. 记录到销售仪表盘
	if t.dashboard != nil {
		t.dashboard.RecordOrder(context.Background(), OrderEvent{
			OrderID:     orderID,
			CustomerID:  customerID,
			OwnerID:     ownerID,
			Amount:      amount,
			ProductName: productName,
			IsAIHandled: aiHandled,
			OrderedAt:   time.Now(),
		})
		rec.Actions = append(rec.Actions, TriggerAction{
			Action: "dashboard_recorded", Target: "order", Result: "ok",
		})
	}

	// 记录历史
	t.mu.Lock()
	t.history = append(t.history, *rec)
	if len(t.history) > 1000 {
		t.history = t.history[len(t.history)-1000:]
	}
	t.mu.Unlock()

	return rec
}

// GetHistory 获取触发历史（用于审计 + 测试）
func (t *SalesActionTrigger) GetHistory(ctx context.Context, customerID string, limit int)  []TriggerRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	hist := make([]TriggerRecord, 0)
	for i := len(t.history) - 1; i >= 0; i-- {
		if customerID == "" || t.history[i].CustomerID == customerID {
			hist = append(hist, t.history[i])
			if limit > 0 && len(hist) >= limit {
				break
			}
		}
	}
	return hist
}

// inferOrderFromJourney 从客户旅程上下文推断订单金额与产品
// 关键：销售跟进时只知道"客户说想买"，但没说金额/产品。
// 修复：从旅程的最近事件或自动标签中推断，作为仪表盘订单事件的兜底数据。
func (t *SalesActionTrigger) inferOrderFromJourney(ctx context.Context, customerID string) (float64, string) {
	if t.journey == nil {
		return 0, ""
	}
	state := t.journey.GetState(ctx, customerID)
	amount := 0.0
	product := "未指定产品"
	// 从最近事件中提取金额/产品（如果有）
	for i := len(state.StageHistory) - 1; i >= 0; i-- {
		ev := state.StageHistory[i]
		if ev.Metadata != nil {
			if v, ok := ev.Metadata["amount"].(float64); ok && v > 0 {
				amount = v
			}
			if v, ok := ev.Metadata["product_name"].(string); ok && v != "" {
				product = v
			}
		}
	}
	return amount, product
}

// ===== 工具函数 =====

func safeIntent(resp *SalesResponse) string {
	if resp == nil || resp.Intent == nil {
		return "unknown"
	}
	if resp.Intent.IntentName != "" {
		return resp.Intent.IntentName
	}
	return resp.Intent.IntentType
}

func safeConf(resp *SalesResponse) float64 {
	if resp == nil || resp.Intent == nil {
		return 0
	}
	return resp.Intent.Confidence
}

func init() {
	// 保证不引入额外 import 报错
	_ = strings.TrimSpace
}
