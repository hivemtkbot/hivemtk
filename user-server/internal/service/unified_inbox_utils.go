package service

import (
	"regexp"
	"strings"
)


var (
	phonePattern = regexp.MustCompile(`(?:^|[^0-9])((?:\+?86[-\s]?)?1[3-9]\d{9})(?:[^0-9]|$)`)
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
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
	if m := phonePattern.FindStringSubmatch(text); len(m) > 1 {
		phone = m[1]
	}
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
	priorityOrder := []string{
		IntentComplaint,    
		IntentPurchase,     
		IntentPriceInquiry, 
		IntentAskProduct,   
		IntentChurn,        
	}
	for _, intent := range priorityOrder {
		for _, kw := range intentKeywords[intent] {
			if strings.Contains(textLower, strings.ToLower(kw)) {
				return intent
			}
		}
	}
	return IntentGreeting 
}

