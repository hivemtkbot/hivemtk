package service

import "strings"

// NLPKeywords 统一 NLP 关键词库
// 整合 chat_visitor.transferKeywords / veto_rule.explicitKeywords / smart_cs_orchestrator.urgentKeywords
// 避免三套独立关键词列表不一致的问题
var NLPKeywords = struct {
	Transfer  []string // 触发转人工
	Explicit  []string // 显式转人工（veto 规则）
	Urgent    []string // 紧急/投诉
}{
	Transfer: []string{
		"人工", "真人", "转人工", "找人", "客服",
		"operator", "human", "agent",
		"转接人工", "人工客服", "真人客服", "找人工",
	},
	Explicit: []string{
		"转人工", "人工客服", "找人工", "真人客服", "转接人工",
		"real agent", "human agent", "transfer to human",
	},
	Urgent: []string{
		"投诉", "举报", "曝光", "315", "消协", "工商局",
		"紧急", "着急", "马上", "立刻", "赶紧", "快点",
		"骗子", "假货", "垃圾", "再也不买",
		"退钱", "退款", "赔钱", "赔偿",
	},
}

// MatchTransferKeywords 检查消息是否命中转人工关键词
func MatchTransferKeywords(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, kw := range NLPKeywords.Transfer {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// MatchExplicitKeywords 检查消息是否命中显式转人工关键词（veto 规则）
func MatchExplicitKeywords(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, kw := range NLPKeywords.Explicit {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// MatchUrgentKeywords 检查消息是否命中紧急/投诉关键词
func MatchUrgentKeywords(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, kw := range NLPKeywords.Urgent {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
