package service

import "strings"

// NLPKeywords 统一 NLP 关键词库
// 整合 chat_visitor.transferKeywords / veto_rule.explicitKeywords / smart_cs_orchestrator.urgentKeywords
// 避免三套独立关键词列表不一致的问题
var NLPKeywords = struct {
	Transfer []string
	Explicit []string
	Urgent   []string
}{
	Transfer: []string{
		"转人工", "转接人工", "人工客服", "真人客服", "找人工",
		"找客服", "人工服务", "真人", "找人",
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

// negationWindow N-2 否定窗口：命中位置前 6 字符内出现否定词则视为否定语境，不命中。
// 依据 MASTER_COMPETITIVE_DECISIONS.md M3/N-2："不用转人工"不应误触发转人工。
const negationWindow = 6

// negationWords 否定词表（N-2）
var negationWords = []string{"不用", "别", "无需", "不需要", "不想"}

// hasNegationBefore 检查 lower 中 idx（字节偏移）之前的否定窗口（前 6 个 rune）是否含否定词
func hasNegationBefore(lower string, idx int) bool {
	if idx <= 0 {
		return false
	}
	prefix := []rune(lower[:idx])
	lo := len(prefix) - negationWindow
	if lo < 0 {
		lo = 0
	}
	window := string(prefix[lo:])
	for _, n := range negationWords {
		if strings.Contains(window, n) {
			return true
		}
	}
	return false
}

// matchKeywords 关键词匹配 + 否定窗口检查（N-2）：
// 任一关键词的任一次出现，其前 6 字符不含否定词即视为命中；
// 所有出现均处于否定语境（如"不用转人工"）则不命中。
func matchKeywords(content string, kws []string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, kw := range kws {
		kwLower := strings.ToLower(kw)
		off := 0
		for {
			i := strings.Index(lower[off:], kwLower)
			if i < 0 {
				break
			}
			abs := off + i
			if !hasNegationBefore(lower, abs) {
				return true
			}
			off = abs + len(kwLower)
			if off >= len(lower) {
				break
			}
		}
	}
	return false
}

// MatchTransferKeywords 检查消息是否命中转人工关键词（含 N-2 否定窗口过滤）
func MatchTransferKeywords(content string) bool {
	return matchKeywords(content, NLPKeywords.Transfer)
}

// MatchExplicitKeywords 检查消息是否命中显式转人工关键词（veto 规则，含 N-2 否定窗口过滤）
func MatchExplicitKeywords(content string) bool {
	return matchKeywords(content, NLPKeywords.Explicit)
}

// MatchUrgentKeywords 检查消息是否命中紧急/投诉关键词（含 N-2 否定窗口过滤）
func MatchUrgentKeywords(content string) bool {
	return matchKeywords(content, NLPKeywords.Urgent)
}
