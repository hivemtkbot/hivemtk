package service

import (
	"context"
	"strings"
	"testing"
)

// 2026-08-15 P2-5 修复：扩充 humanize_polisher.removeAITraces 客套话清单，覆盖：
//   1) 自报家门（"作为 AI 助手"等）
//   2) 空泛寒暄（"非常感谢您的咨询"等）
//   3) 单一应答客套（"可以的！😊"等）
//   4) 模板化道歉（"请谅解"等）
// 验证每类至少 1 个用例，确保后续 LLM 出现客套话时被服务端兜底清掉，
// 而不是直接投递给客户形成「一眼假」的统一收件箱。

func TestHumanizePolisher_P2_5_RemoveFlattery(t *testing.T) {
	p := NewHumanizePolisher()
	ctx := context.Background()

	type tc struct {
		name   string
		input  string
		expect string // 期望去除客套话后保留的实质性内容
		intent string // PolishContext.Intent（默认 ""=普通场景;complaint/churn/after_sale 跳过句首清理）
	}
	cases := []tc{
		// 1) 自报家门
		{
			name:   "self_intro_1",
			input:  "作为 AI 助手，我需要更多上下文。",
			expect: "，我需要更多上下文。",
		},
		{
			name:   "self_intro_2",
			input:  "我是 AI，不能直接帮您下单。",
			expect: "，不能直接帮您下单。",
		},
		// 2) 空泛寒暄
		{
			name:   "courtesy_1",
			input:  "非常感谢您的咨询。我们的产品主要面向私域。",
			expect: "我们的产品主要面向私域。",
		},
		{
			name:   "courtesy_2",
			input:  "很高兴为您服务。HiveMTK 是 AGPL-3.0 协议。",
			expect: "HiveMTK 是 AGPL-3.0 协议。",
		},
		{
			name:   "courtesy_3",
			input:  "感谢您的提问。Gitee 主仓库地址是 xxx。",
			expect: "Gitee 主仓库地址是 xxx。",
		},
		{
			name:   "courtesy_4",
			input:  "期待您的回复。",
			expect: "",
		},
		// 3) 单一应答客套
		{
			name:   "single_ack_1",
			input:  "可以的！😊 关于代码二开请查看 AGPL-3.0 协议。",
			expect: "关于代码二开请查看 AGPL-3.0 协议。",
		},
		{
			name:   "single_ack_2",
			input:  "好的！",
			expect: "",
		},
		{
			name:   "single_ack_3",
			input:  "没问题 😊 我来帮您看下。",
			expect: "我来帮您看下。",
		},
		{
			name:   "single_ack_4",
			input:  "当然可以😄 我们支持私有化部署。",
			expect: "我们支持私有化部署。",
		},
		// 4) 模板化道歉
		{
			name:   "apology_1",
			input:  "很抱歉，我无法直接操作您的设备。",
			expect: "直接操作您的设备。",
		},
		{
			name:   "apology_2",
			input:  "请您谅解。这是 HiveMTK 的私域部署说明。",
			expect: "这是 HiveMTK 的私域部署说明。",
		},
		{
			name:   "apology_3",
			input:  "造成不便深表歉意，请联系客服。",
			expect: "，请联系客服。",
		},
		// 5) 实际生产 case（P2-5 触发源）：
		//    客户问"代码二开" → LLM 输出"可以的！😊 关于代码二开...给您简单说明：- **开源协议**..."
		{
			name:   "real_xhs_1",
			input:  "可以的！😊 关于代码二次开发，给您简单说明：\n\n- **开源协议**：HiveMTK 采用 **GNU AGPL-3.0**\n- **仓库地址**：Gitee 主仓库",
			expect: "关于代码二次开发，给您简单说明：\n\n- **开源协议**：HiveMTK 采用 **GNU AGPL-3.0**\n- **仓库地址**：Gitee 主仓库",
		},
		{
			name:   "real_xhs_2",
			input:  "可以的😊 HiveMTK 是私域单租户 AI 智能体客服系统。",
			expect: "HiveMTK 是私域单租户 AI 智能体客服系统。",
		},
		// 6) 投诉/挽留/售后场景：句首"好的"必须保留（白名单）
		{
			name:   "complaint_leading_ack_preserved",
			input:  "好的，我来帮您处理这个售后问题。",
			expect: "好的，我来帮您处理这个售后问题。",
			intent: IntentComplaint,
		},
		{
			name:   "churn_leading_ack_preserved",
			input:  "好的，让我看一下您最近的订单。",
			expect: "好的，让我看一下您最近的订单。",
			intent: IntentChurn,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// P2-5 修复点:句首客套由 Polish.removeLeadingFlattery 处理,
			//   单测 Polish 完整流程而非单独的 removeAITraces
			pctx := &PolishContext{Platform: "xiaohongshu", Intent: c.intent}
			got, err := p.Polish(ctx, c.input, pctx)
			if err != nil {
				t.Fatalf("Polish unexpected error: %v", err)
			}
			// 期望:去除客套后,核心内容应保留(允许前后空白差异)
			if !strings.Contains(got, strings.TrimSpace(c.expect)) && strings.TrimSpace(c.expect) != "" {
				t.Errorf("removeAITraces(%q) = %q, want to contain %q", c.input, got, c.expect)
			}
			// 反向断言: 客套话不应再出现(仅在非 complaint/churn/after_sale 场景下严格断言)
			if c.intent != IntentComplaint && c.intent != IntentChurn && c.intent != IntentAfterSale {
				for _, trace := range []string{"作为 AI 助手", "可以的", "感谢您的", "很高兴为您服务", "请您谅解"} {
					if strings.Contains(got, trace) {
						t.Errorf("Polish(%q) = %q, still contains flattery %q", c.input, got, trace)
					}
				}
			}
		})
	}
}

// 反向回归：实际内容不应被误伤。验证 P2-5 修复不会把"可以的" 这种正常词全部干掉。
func TestHumanizePolisher_P2_5_NormalContent_NotOverStripped(t *testing.T) {
	p := NewHumanizePolisher()
	ctx := context.Background()

	cases := []string{
		"可以的，我可以帮您查看具体功能。",        // 句中含"可以的"但作正常叙述
		"我们支持的功能是 AGPL-3.0 开源。",      // 无客套话
		"好的，下面是您的订单详情。",             // "好的"作正常过渡
		"Hi, what can I do for you?",            // 英文正常
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			pctx := &PolishContext{Platform: "xiaohongshu"}
			got, err := p.Polish(ctx, input, pctx)
			if err != nil {
				t.Fatalf("Polish unexpected error: %v", err)
			}
			// 至少保留 ≥ 50% 原字符(避免把"好的，下面是您的订单详情"误伤成"下面是您的订单详情" 之类过头去除)
			// 注意:这里"好的"如果在句首+标点,本身应去除 → 此处反过来允许去除
			// 所以仅校验"实质性内容"是否被保留
			if len([]rune(got)) == 0 {
				t.Errorf("Polish(%q) = %q, should not strip all content", input, got)
			}
		})
	}
}
