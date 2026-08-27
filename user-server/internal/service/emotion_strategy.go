package service

// P1g 情感分层响应策略（竞品吸收：小冠AI，见 AI_CORE_COMPETITIVE_ANALYSIS.md）
// 愤怒→补偿+高级客服（转人工）；焦虑→进度可视化（不转人工，注入回复策略提示）；
// 满意→裂变引导。替代原先"危机关键词一刀切转人工"的无差别行为。

// EmotionType 客户情绪类型
type EmotionType string

const (
	EmotionNeutral   EmotionType = "neutral"
	EmotionAnger     EmotionType = "anger"
	EmotionAnxiety   EmotionType = "anxiety"
	EmotionSatisfied EmotionType = "satisfied"
)

// emotionKeywords 情绪关键词库：
// Anger 与 Anxiety 两表对原 NLPKeywords.Urgent 全集做互斥拆分，
// 保证旧 isUrgentOrComplaint=true 的内容必然落入二者之一（行为不产生第三态漂移）。
var emotionKeywords = struct {
	Anger     []string
	Anxiety   []string
	Satisfied []string
}{
	Anger: []string{
		"投诉", "举报", "曝光", "315", "消协", "工商局",
		"骗子", "假货", "垃圾", "再也不买",
		"退钱", "退款", "赔钱", "赔偿",
	},
	Anxiety: []string{
		"紧急", "着急", "马上", "立刻", "赶紧", "快点",
	},
	Satisfied: []string{
		"很满意", "太好了", "非常好", "靠谱", "五星好评", "推荐朋友",
	},
}

// ClassifyEmotion 情感分类，优先级：愤怒 > 焦虑 > 满意 > 中性。
// 复用 N-2 否定窗口匹配（如"不用退款了""不用着急"均不触发对应情绪层）。
func ClassifyEmotion(content string) EmotionType {
	if matchKeywords(content, emotionKeywords.Anger) {
		return EmotionAnger
	}
	if matchKeywords(content, emotionKeywords.Anxiety) {
		return EmotionAnxiety
	}
	if matchKeywords(content, emotionKeywords.Satisfied) {
		return EmotionSatisfied
	}
	return EmotionNeutral
}

// EmotionStrategy 情感对应的编排策略
type EmotionStrategy struct {
	// TransferToHuman 是否升级人工（仅愤怒层）
	TransferToHuman bool
	// TransferReason 转人工原因标签（TransferToHuman=true 时非空）
	TransferReason string
	// ReplyHint 注入销售引擎 prompt 的回复策略提示（焦虑=进度可视化 / 满意=裂变引导）
	ReplyHint string
}

// StrategyForEmotion 返回情感对应的编排策略
func StrategyForEmotion(e EmotionType) EmotionStrategy {
	switch e {
	case EmotionAnger:
		return EmotionStrategy{
			TransferToHuman: true,
			TransferReason:  "检测到强烈不满或投诉情绪，转高级客服优先补偿处理",
		}
	case EmotionAnxiety:
		return EmotionStrategy{
			ReplyHint: "客户处于焦虑等待状态：回复必须明确当前处理进度、下一步动作与预计完成时间，避免空泛安抚。",
		}
	case EmotionSatisfied:
		return EmotionStrategy{
			ReplyHint: "客户表现出满意情绪：可自然邀请其推荐给朋友或留下评价，语气克制、点到为止不强推。",
		}
	default:
		return EmotionStrategy{}
	}
}
