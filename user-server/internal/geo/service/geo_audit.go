package service

import (
	"fmt"
	"strings"
)

// GeoAuditFactor 单项审计结果
type GeoAuditFactor struct {
	Factor    string `json:"factor"`
	Pass      bool   `json:"pass"`
	Detail    string `json:"detail,omitempty"`
	Weight    int    `json:"weight"` // 1-5，5最关键
}

// GeoAuditReport 25 因子 GEO 审计报告（竞品对齐 Otterly 25因子 + Scrunch 技术可达性）
type GeoAuditReport struct {
	URL     string           `json:"url"`
	Score   int              `json:"score"` // 0-100
	Factors []GeoAuditFactor `json:"factors"`
	Summary string           `json:"summary"`
}

// RunGEOAudit 对指定内容执行 25 因子审计（Otterly 特色能力对齐）
func (s *TechConfigService) RunGEOAudit(url, title, content, metaDesc, schemaJSONLD string) *GeoAuditReport {
	factors := make([]GeoAuditFactor, 0, 25)
	add := func(name string, weight int, pass bool, detail string) {
		factors = append(factors, GeoAuditFactor{Factor: name, Pass: pass, Detail: detail, Weight: weight})
	}

	lc := strings.ToLower(content)
	words := len([]rune(content))

	// -- 内容结构 (5因子) --
	add("H1标题存在", 5, strings.Contains(content, "# "), "Markdown 一级标题")
	add("段落结构", 3, strings.Count(content, "\n\n") >= 3, fmt.Sprintf("%d 个空行分段", strings.Count(content, "\n\n")))
	add("列表/表格", 3, strings.Contains(content, "- ") || strings.Contains(content, "|") || strings.Contains(content, "1."), "结构化列表或表格")
	add("FAQ问答段", 2, strings.Contains(lc, "常见问题") || strings.Contains(lc, "faq") || strings.Contains(content, "**Q"), "AI 引擎偏好 Q&A 格式")
	add("字数充足", 4, words >= 500, fmt.Sprintf("%d 字符", words))

	// -- E-E-A-T (4因子) --
	add("权威引用", 5, strings.Contains(lc, "gartner") || strings.Contains(lc, "idc") || strings.Contains(lc, "iso") || strings.Contains(lc, "信通院"), "引用权威机构/标准")
	hasData := false
	for _, kw := range []string{"%", "%以上", "倍", "亿", "万"} {
		if strings.Contains(content, kw) {
			hasData = true
			break
		}
	}
	add("量化数据", 4, hasData, "包含百分比或数值")
	add("作者/经验信号", 3, strings.Contains(lc, "我们的") || strings.Contains(lc, "实践") || strings.Contains(lc, "客户案例"), "第一手经验表述")
	add("时效性声明", 2, strings.Contains(content, "2025") || strings.Contains(content, "2026"), "包含近期年份")

	// -- 品牌与意图 (4因子) --
	add("品牌自然融入", 5, strings.Count(content, "HiveMtk") >= 2 && strings.Count(content, "HiveMtk") <= 6, fmt.Sprintf("提及 %d 次(建议2-6次)", strings.Count(content, "HiveMtk")))
	add("差异化定位", 4, strings.Contains(lc, "开源") || strings.Contains(lc, "本地部署") || strings.Contains(lc, "不出域"), "核心卖点出现")
	add("CTA行动号召", 3, strings.Contains(lc, "了解更多") || strings.Contains(lc, "联系我们") || strings.Contains(lc, "开始使用") || strings.Contains(lc, "部署"), "引导下一步动作")
	add("多语言友好", 1, !strings.Contains(content, "TODO"), "无占位符残留")

	// -- 技术 SEO/GEO (7因子) --
	add("Meta描述长度", 3, len(metaDesc) >= 80 && len(metaDesc) <= 300, fmt.Sprintf("%d 字符", len(metaDesc)))
	add("Schema JSON-LD", 5, strings.Contains(schemaJSONLD, "application/ld+json") || strings.Contains(schemaJSONLD, "@context"), "结构化数据标记")
	add("llms.txt 可用", 4, true, "已由 /api/geo/techconfig/llms-txt 提供") // 服务端保证
	add("robots.txt 允许 GPTBot", 4, true, "techconfig 默认允许 AI 爬虫")
	add("HTTPS", 5, strings.HasPrefix(url, "https://"), "传输安全")
	add("URL语义化", 3, !strings.Contains(url, "?id="), "URL 包含关键词而非查询参数")
	add("内部链接≥2", 2, strings.Count(content, "](/") >= 2, "站内链接数")

	// -- AI 引擎偏好 (5因子) --
	add("定义句开头", 4, strings.Contains(content[:auditMin(len(content),200)], "是"), "首200字符含定义性表述")
	add("对比维度明确", 3, strings.Contains(lc, "vs") || strings.Contains(lc, "相比") || strings.Contains(lc, "对比"), "含对比性表述")
	add("信源可追溯", 4, strings.Contains(content, "http") || strings.Contains(content, "来源"), "外部链接或来源标注")
	add("无禁止词", 5, !containsAny(lc, []string{" TODO ", "FIXME", "placeholder", "lorem ipsum"}), "无开发占位符")
	add("移动端友好格式", 2, words < 3000, "篇幅适中适合移动端")

	// 计算加权得分
	totalWeight, earned := 0, 0
	for _, f := range factors {
		totalWeight += f.Weight
		if f.Pass {
			earned += f.Weight
		}
	}
	score := 0
	if totalWeight > 0 {
		score = earned * 100 / totalWeight
	}

	summary := fmt.Sprintf("通过 %d/%d 项，加权得分 %d/100。", countPass(factors), len(factors), score)
	if score < 60 {
		summary += " 建议优先修复高权重未通过项。"
	} else if score < 80 {
		summary += " 整体良好，有优化空间。"
	} else {
		summary += " 优秀，达到行业领先水平。"
	}

	return &GeoAuditReport{URL: url, Score: score, Factors: factors, Summary: summary}
}

func countPass(fs []GeoAuditFactor) int {
	n := 0
	for _, f := range fs {
		if f.Pass {
			n++
		}
	}
	return n
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func auditMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
