package service

import (
	"context"
	"strings"

	tracelearning "hivemtk-user/internal/service/trace_learning"
)

// L-2 经验沉淀读取侧：SalesEngine 组 prompt 时注入本行业 top3 错误模式洞察。
//
// 行业来源由装配层注入（main.go 调用 SetLearningInsightIndustryProvider，
// 如从资产包/系统配置解析本商户行业）；未注入或行业为空时不注入。
// 洞察数据来自 learning_insights 表（trace_learning 批处理沉淀），无数据时静默跳过。

// salesInsightLimit 注入 prompt 的洞察条数上限
const salesInsightLimit = 3

// insightIndustryFn 本商户行业标签解析器（装配层注入）
var insightIndustryFn func() string

// SetLearningInsightIndustryProvider 注入行业标签解析器（main.go 装配时调用）
func SetLearningInsightIndustryProvider(fn func() string) {
	insightIndustryFn = fn
}

// appendLearningInsights 将行业错误模式洞察追加到 prompt（无数据/未配置行业则不注入）
func (e *SalesEngine) appendLearningInsights(sb *strings.Builder) {
	if e.db == nil || insightIndustryFn == nil {
		return
	}
	industry := insightIndustryFn()
	if industry == "" {
		return
	}
	insights, err := tracelearning.TopInsights(context.Background(), e.db, industry, salesInsightLimit)
	if err != nil || len(insights) == 0 {
		return
	}
	sb.WriteString("\n【历史错误模式（回复时务必避免）】:\n")
	for _, t := range insights {
		sb.WriteString("- ")
		sb.WriteString(t)
		sb.WriteString("\n")
	}
}
