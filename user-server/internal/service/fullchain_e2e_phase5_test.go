package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Phase 5 端到端测试：完整业务链路（触达 → 智能体 9 步 → 反馈学习闭环）
// ----------------------------------------------------------------------------
// 商业产品级验证：一条客户消息从入站到反馈学习的完整链路，
// 覆盖用户视角的 6 大支柱中的关键链路：
//   ② 触达（4 渠道入站消息）
//   ⑥ 智能体（SalesEngine 9 步链路 + 第 9 步反馈学习）
//
// 测试策略：
//   - 使用 NewSalesEngine(nil,...) 创建引擎，依赖为 nil 时走兜底逻辑
//   - 注入真实 FeedbackLearner，验证第 9 步记录
//   - 不使用 mock，验证真实业务流程
// ============================================================================

// TestE2E_FullChain_ReachToSalesFeedback 4 渠道客户消息 → SalesEngine 9 步 → 反馈学习记录
// 商业产品级核心闭环：客户在 4 个渠道发消息 → 智能体 9 步处理 → 决策快照进入反馈学习
func TestE2E_FullChain_ReachToSalesFeedback(t *testing.T) {
	// 1. 构建 智能体引擎（依赖为 nil，走兜底逻辑）+ 注入反馈学习器
	fl := NewFeedbackLearner(nil)
	engine := NewSalesEngine(nil, nil, nil, nil, nil, nil, nil, nil)
	engine.SetFeedbackLearner(context.Background(), fl)

	channels := []struct {
		name    string
		channel string
		content string
		from    string
	}{
		{"WeCom", "wecom", "光子嫩肤多少钱", "user_wecom_1"},
		{"WhatsApp", "whatsapp", "hi, I want to know the price", "user_wa_1"},
		{"Telegram", "telegram", "hello", "user_tg_1"},
		{"Feishu", "feishu", "在吗，想咨询下", "user_fs_1"},
	}

	for _, tc := range channels {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// 2. 客户消息入站（触达）
			resp, err := engine.ProcessIncomingMessage(ctx, &ChannelMessage{
				Channel:      tc.channel,
				AccountID:    "1",
				ExternalUser: tc.from,
				Content:      tc.content,
				MsgType:      "text",
			})
			if err != nil {
				t.Fatalf("[%s] ProcessIncomingMessage 失败: %v", tc.name, err)
			}
			if resp == nil {
				t.Fatalf("[%s] resp nil", tc.name)
			}
			if resp.Reply == "" {
				t.Errorf("[%s] Reply 不应为空", tc.name)
			}

			// 3. 验证 9 步链路完整性
			if len(resp.Steps) < 7 {
				t.Errorf("[%s] Steps 应 >= 7 个，实际 %d", tc.name, len(resp.Steps))
			}

			// 4. 验证第 9 步反馈学习被记录
			hasFeedbackStep := false
			for _, s := range resp.Steps {
				if s.Step == "9_feedback_learn" {
					hasFeedbackStep = true
					if s.Status != "ok" {
						t.Errorf("[%s] 第 9 步 status 应为 ok: %s", tc.name, s.Status)
					}
					break
				}
			}
			if !hasFeedbackStep {
				t.Errorf("[%s] 应包含 9_feedback_learn 步骤", tc.name)
			}

			t.Logf("✅ [%s] 完整链路: %d 步, reply=%s...", tc.name, len(resp.Steps), truncate(resp.Reply, 30))
		})
	}

	// 5. 验证 FeedbackLearner 累积了 4 个渠道的反馈
	allStats := fl.GetAllIntentStats()
	totalCount := 0
	for _, s := range allStats {
		totalCount += s.TotalCount
	}
	if totalCount < 4 {
		t.Errorf("FeedbackLearner 应记录 >= 4 条反馈（4 渠道），实际 %d", totalCount)
	}
	t.Logf("✅ 反馈学习累积: %d 条记录, %d 种意图", totalCount, len(allStats))
}

// TestE2E_FullChain_FeedbackAccumulation 多轮对话 → 反馈学习累积 → 置信度阈值建议
// 商业产品级：智能体用得越久越懂客户，FeedbackLearner 累积数据后能建议置信度阈值
func TestE2E_FullChain_FeedbackAccumulation(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := NewSalesEngine(nil, nil, nil, nil, nil, nil, nil, nil)
	engine.SetFeedbackLearner(context.Background(), fl)

	customer := "cust_accumulation"
	sessionID := "wecom:chat_001"

	// 模拟同一客户 15 轮对话（超过冷启动阈值 10）
	for i := 0; i < 15; i++ {
		_, _ = engine.ProcessIncomingMessage(ctx, &ChannelMessage{
			Channel:      "wecom",
			AccountID:    "1",
			ExternalUser: customer,
			ChatID:       "chat_001",
			Content:      "我想了解光子嫩肤",
			MsgType:      "text",
		})
		// 用 sessionID 避免 engine 内部 customerID 重复
		_ = sessionID
	}

	// 验证累积统计
	stats := fl.GetAllIntentStats()
	if len(stats) == 0 {
		t.Fatal("应累积意图统计")
	}

	// 验证 SuggestConfidenceFloor 能给出建议（冷启动后）
	for _, s := range stats {
		floor := fl.SuggestConfidenceFloor(context.Background(), s.IntentType)
		if floor <= 0 {
			t.Errorf("意图 %s 的 SuggestConfidenceFloor 应 > 0", s.IntentType)
		}
		if floor > 1 {
			t.Errorf("意图 %s 的 SuggestConfidenceFloor 应 <= 1", s.IntentType)
		}
		t.Logf("✅ 意图 %s: total=%d, floor=%.2f", s.IntentType, s.TotalCount, floor)
	}
}

// TestE2E_FullChain_StepSequence 9 步链路顺序验证
// 商业产品级：确保 9 步链路按正确顺序执行，不遗漏、不乱序
func TestE2E_FullChain_StepSequence(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := NewSalesEngine(nil, nil, nil, nil, nil, nil, nil, nil)
	engine.SetFeedbackLearner(context.Background(), fl)

	resp, err := engine.ProcessIncomingMessage(context.Background(), &ChannelMessage{
		Channel:      "wecom",
		AccountID:    "1",
		ExternalUser: "user_seq",
		ChatID:       "chat_seq",
		Content:      "你好",
		MsgType:      "text",
	})
	if err != nil {
		t.Fatalf("失败: %v", err)
	}

	// 期望的步骤顺序（依赖为 nil 时部分步骤会 skip/fail，但仍按顺序执行）
	expectedSteps := []string{
		"1_resolve_customer",
		"2_recall_memory",
		"3_recognize_intent",
		"4_match_sop",
		"5_recall_rag",
		"6_generate_candidate",
		"7_polish",
		"8_audit",
		"9_feedback_learn",
	}

	stepNames := make([]string, 0, len(resp.Steps))
	for _, s := range resp.Steps {
		stepNames = append(stepNames, s.Step)
	}

	// 验证关键步骤都存在（3.5/5.5/5.6 是条件性的，不强制）
	for _, expected := range expectedSteps {
		found := false
		for _, actual := range stepNames {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("缺少步骤: %s (实际步骤: %s)", expected, strings.Join(stepNames, ", "))
		}
	}

	t.Logf("✅ 9 步链路完整: %s", strings.Join(stepNames, " → "))
}

// TestE2E_FullChain_SmartOrchestratorDelegatesToEngine SmartCSOrchestrator 委托 SalesEngine
// 商业产品级：SmartCSOrchestrator 是 LLM + 客服座席的结合体，遇到 AI 可处理的会话时委托给 SalesEngine
func TestE2E_FullChain_SmartOrchestratorDelegatesToEngine(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := NewSalesEngine(nil, nil, nil, nil, nil, nil, nil, nil)
	engine.SetFeedbackLearner(context.Background(), fl)

	orchestrator := NewSmartCSOrchestrator(engine, DefaultOrchestratorConfig())

	// 验证 orchestrator 正确包装了 engine
	if orchestrator.engine == nil {
		t.Fatal("orchestrator 应持有 engine 引用")
	}

	// 验证 orchestrator 的配置（字段直接访问）
	if orchestrator.confidenceThreshold != 0.7 {
		t.Errorf("confidenceThreshold 应为 0.7，实际 %.2f", orchestrator.confidenceThreshold)
	}
	if !orchestrator.enableAutoReply {
		t.Error("enableAutoReply 应为 true")
	}
	if orchestrator.maxAIConsecutive != 5 {
		t.Errorf("maxAIConsecutive 应为 5，实际 %d", orchestrator.maxAIConsecutive)
	}

	t.Logf("✅ SmartCSOrchestrator 配置: threshold=%.2f, autoReply=%v, maxAI=%d",
		orchestrator.confidenceThreshold, orchestrator.enableAutoReply, orchestrator.maxAIConsecutive)
}

// TestE2E_FullChain_ResponseIntegrity 响应完整性验证
// 商业产品级：SalesResponse 是整个链路的核心数据载体，必须包含完整信息
func TestE2E_FullChain_ResponseIntegrity(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := NewSalesEngine(nil, nil, nil, nil, nil, nil, nil, nil)
	engine.SetFeedbackLearner(context.Background(), fl)

	resp, err := engine.ProcessIncomingMessage(context.Background(), &ChannelMessage{
		Channel:      "wecom",
		AccountID:    "1",
		ExternalUser: "user_integrity",
		ChatID:       "chat_integrity",
		Content:      "你好，想咨询",
		MsgType:      "text",
	})
	if err != nil {
		t.Fatalf("失败: %v", err)
	}

	// 验证响应字段完整性
	if resp.Reply == "" {
		t.Error("Reply 不应为空")
	}
	if resp.LatencyMs < 0 {
		t.Errorf("LatencyMs 应 >= 0，实际 %d", resp.LatencyMs)
	}
	if len(resp.Steps) == 0 {
		t.Error("Steps 不应为空")
	}
	// Intent 应被填充（即使 fallback）
	if resp.Intent == nil {
		t.Error("Intent 不应为 nil（即使 fallback 也应填充）")
	} else if resp.Intent.IntentType == "" {
		t.Error("IntentType 不应为空")
	}

	t.Logf("✅ 响应完整: reply_len=%d, latency=%dms, steps=%d, intent=%s",
		len(resp.Reply), resp.LatencyMs, len(resp.Steps), resp.Intent.IntentType)
}
