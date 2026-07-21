package humanize

// rule_scorer_bench_test.go P3 RuleScorer 性能基准
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十六章 §16.5
//
// 性能目标：
//   - RuleScorer 全量评估 < 1ms（5 维同步执行）
//   - 边界情况（500字长文本、含 AI 痕迹词）应保持稳定 < 5ms
//
// 运行方式：
//
//	go test -bench=BenchmarkRuleScorer -benchmem -benchtime=3s ./internal/service/humanize/
//
// 监控指标：
//   - P3 监控 humanize_score_value_count (counter)
//   - 内部 latency 记录在 zerolog 业务日志

import (
	"context"
	"strings"
	"testing"

	"marketing/internal/dto"
)

// 真实业务 AI 回复样本（用于 benchmark）
var benchReplies = []struct {
	name    string
	reply   string
	intent  string
	message string
}{
	{
		name:    "短自然回复",
		reply:   "嗯嗯，可以的，您看这个套餐比较适合您~",
		intent:  "pricing_inquiry",
		message: "多少钱",
	},
	{
		name:    "中等业务回复",
		reply:   "好的，我帮您看下哈，根据您刚才说的需求，其实我们的标准版就够用了，月付 199，一年下来比月付省 400 多。要不我先帮您开个试用？",
		intent:  "pricing_inquiry",
		message: "你们这个套餐多少钱",
	},
	{
		name:    "长回复+AI痕迹",
		reply:   "首先，我理解您的担忧。其次，作为一款致力于提供卓越服务的解决方案，我们的产品具备以下核心优势：第一，全方位的功能覆盖；第二，专业的技术支持；第三，值得信赖的品牌保证。综上所述，我们坚信本产品能够完美满足您的需求。建议您立即注册体验，开启智能化之旅。",
		intent:  "objection_price",
		message: "感觉有点贵啊",
	},
	{
		name:    "500字长文",
		reply:   strings.Repeat("嗯好的，我帮您看下哈。这个功能我们之前也帮很多客户配置过，", 10) + "您放心。",
		intent:  "feature_demo",
		message: "能详细讲下这个功能怎么用吗",
	},
	{
		name:    "纯机器腔",
		reply:   "根据您的查询，我已检索到相关信息。该产品具备以下特性：1. 高效；2. 稳定；3. 安全。请问您是否需要进行进一步的咨询？",
		intent:  "general",
		message: "你好",
	},
	{
		name:    "含emoji和语气词",
		reply:   "哈哈，没问题哒～ 您看这样行不？先试用 3 天，合适再续费，OK 的哈？😊",
		intent:  "trial_request",
		message: "想先试试",
	},
}

// BenchmarkRuleScorer_RuleOnly 基准：纯 RuleScorer 全量评估
func BenchmarkRuleScorer_RuleOnly(b *testing.B) {
	s := NewRuleScorer()
	ctx := context.Background()
	// 取一个代表性样本
	sample := benchReplies[1]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := &dto.HumanizeEvalInput{
			AIReply:         sample.reply,
			Intent:          sample.intent,
			CustomerMessage: sample.message,
		}
		_, err := s.Evaluate(ctx, input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRuleScorer_AllSamples 基准：跨多种样本文本测试
func BenchmarkRuleScorer_AllSamples(b *testing.B) {
	s := NewRuleScorer()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, sample := range benchReplies {
			input := &dto.HumanizeEvalInput{
				AIReply:         sample.reply,
				Intent:          sample.intent,
				CustomerMessage: sample.message,
			}
			_, _ = s.Evaluate(ctx, input)
		}
	}
}

// BenchmarkRuleScorer_Parallel 基准：并发场景（模拟 100 QPS）
func BenchmarkRuleScorer_Parallel(b *testing.B) {
	s := NewRuleScorer()
	ctx := context.Background()
	sample := benchReplies[1]

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			input := &dto.HumanizeEvalInput{
				AIReply:         sample.reply,
				Intent:          sample.intent,
				CustomerMessage: sample.message,
			}
			_, _ = s.Evaluate(ctx, input)
		}
	})
}
