package service

import (
	"regexp"
	"strings"
)

// ============================================================================
// 商业产品级 收件箱工具函数
// ----------------------------------------------------------------------------
// 真实场景：客户在私信里随手写"我的微信 xxx" / "加我手机 13800138000" / "邮箱 foo@bar.com"
// 必须能从非结构化文本中提取出来，OneID 才能正确合并。
// ============================================================================

var (
	// 提取国内手机号：1[3-9]\d{9}
	// 关键：中文/英文/标点都可能出现在手机号前后，要全部允许
	phonePattern = regexp.MustCompile(`(?:^|[^0-9])((?:\+?86[-\s]?)?1[3-9]\d{9})(?:[^0-9]|$)`)
	// 提取邮箱
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	// 简单意图识别
	intentKeywords = map[string][]string{
		IntentPriceInquiry: {"价格", "多少钱", "怎么卖", "报价", "price", "cost", "费用", "价位"},
		IntentAskProduct:   {"有什么", "推荐", "适合", "哪款", "怎么选", "product", "recommend"},
		IntentPurchase:     {"想买", "下单", "购买", "付款", "buy", "purchase", "order", "我要"},
		IntentComplaint:    {"投诉", "退款", "差评", "垃圾", "complaint", "refund", "scam"},
		IntentChurn:        {"不想要了", "取消", "退订", "cancel", "unsubscribe"},
	}
)

// extractContactFromText 从消息文本中提取手机号/邮箱
func extractContactFromText(text string) (phone, email string) {
	if text == "" {
		return "", ""
	}
	// 提取手机号
	if m := phonePattern.FindStringSubmatch(text); len(m) > 1 {
		phone = m[1]
	}
	// 提取邮箱
	if m := emailPattern.FindStringSubmatch(text); len(m) > 0 {
		email = m[0]
	}
	return phone, email
}

// detectIntentFromText 简单意图识别（基于关键词）
// 真实场景：LLM 推理耗时长/有成本，先用关键词做快速识别，复杂情况交给 LLM
func detectIntentFromText(text string) string {
	if text == "" {
		return IntentGreeting
	}
	textLower := strings.ToLower(text)
	// 优先识别高价值意图
	priorityOrder := []string{
		IntentComplaint,    // 投诉优先（避免错过风险）
		IntentPurchase,     // 购买（高价值）
		IntentPriceInquiry, // 询价
		IntentAskProduct,   // 咨询
		IntentChurn,        // 流失
	}
	for _, intent := range priorityOrder {
		for _, kw := range intentKeywords[intent] {
			if strings.Contains(textLower, strings.ToLower(kw)) {
				return intent
			}
		}
	}
	return IntentGreeting // 默认闲聊
}
