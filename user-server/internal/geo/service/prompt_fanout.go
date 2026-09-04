package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PromptFanoutService Prompt 扇出研究服务
//
// 对标 Otterly Query Fan-out / AI Mode 扇出能力：给定种子关键词，
// 用 LLM 生成 AI 引擎真实会扇出的问法变体（疑问/对比/推荐/FAQ/负面五类），
// 可选对每个变体跑真实探针验证品牌可见性。
type PromptFanoutService struct {
	llm      *LLMAdapter
	probeSvc *ProbeService
}

// NewPromptFanoutService 创建扇出研究服务
func NewPromptFanoutService(llm *LLMAdapter, probeSvc *ProbeService) *PromptFanoutService {
	return &PromptFanoutService{llm: llm, probeSvc: probeSvc}
}

// FanoutRequest 扇出研究请求
type FanoutRequest struct {
	Seed       string `json:"seed" binding:"required"` // 种子关键词/品牌词
	Intent     string `json:"intent"`                  // 业务意图备注，辅助 LLM 生成更贴切的问法
	WithProbes bool   `json:"with_probes"`             // 是否对每个变体跑真实探针
}

// FanoutVariant 单个扇出变体
type FanoutVariant struct {
	Category   string  `json:"category"`              // direct / compare / recommend / faq / negative
	Query      string  `json:"query"`                 // 变体问法
	BrandHit   *bool   `json:"brand_hit,omitempty"`   // with_probes=true 时回填：品牌是否被提及
	Engine     string  `json:"engine,omitempty"`      // 探针命中引擎
	ProbeError string  `json:"probe_error,omitempty"` // 单变体探针失败原因（不中断整批）
}

// FanoutResult 扇出研究结果
type FanoutResult struct {
	Seed       string          `json:"seed"`
	Variants   []FanoutVariant `json:"variants"`
	ProbeCount int             `json:"probe_count"` // 实际执行的探针次数
	Model      string          `json:"model"`       // 扇出生成使用的模型
}

const promptFanoutSystem = `你是 AI 搜索优化（GEO）专家。用户给出一个种子关键词，你需要模拟真实用户在
ChatGPT / Perplexity / 文心一言 等 AI 搜索引擎中会问出的问题变体（query fan-out）。
严格只输出一个 JSON 对象，不要输出任何其他文字。格式：
{"variants":[{"category":"direct|compare|recommend|faq|negative","query":"..."}]}
要求：
1. 生成 8-12 个变体，覆盖全部 5 个 category（direct 直接询问 / compare 竞品对比 / recommend 求推荐 / faq 长尾疑问 / negative 负面考察如"XX 靠谱吗 有什么坑"）
2. query 必须是用户真实口吻的完整问句，不要罗列关键词
3. 不要生成重复或仅有语气词差异的问法`

// Fanout 生成扇出变体，可选真实探针验证
func (s *PromptFanoutService) Fanout(ctx context.Context, req *FanoutRequest) (*FanoutResult, error) {
	seed := strings.TrimSpace(req.Seed)
	if seed == "" {
		return nil, fmt.Errorf("seed 不能为空")
	}

	prompt := fmt.Sprintf("种子关键词：%s", seed)
	if intent := strings.TrimSpace(req.Intent); intent != "" {
		prompt += fmt.Sprintf("\n业务意图：%s", intent)
	}
	prompt += "\n请生成扇出变体 JSON。"

	resp, err := s.llm.GenerateJSON(ctx, promptFanoutSystem, prompt, 2000)
	if err != nil {
		return nil, fmt.Errorf("扇出变体生成失败: %w", err)
	}

	variants, err := parseFanoutVariants(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("扇出变体解析失败: %w", err)
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("LLM 未返回有效变体")
	}

	result := &FanoutResult{Seed: seed, Variants: variants, Model: resp.Model}

	if req.WithProbes && s.probeSvc != nil {
		for i := range result.Variants {
			v := &result.Variants[i]
			runs, _ := s.probeSvc.ProbeAllEngines(ctx, v.Query)
			if len(runs) == 0 {
				v.ProbeError = "探针失败或无可用引擎"
				continue
			}
			// 任一引擎提及品牌即算命中；优先记录第一个提及的引擎
			hit := false
			for _, r := range runs {
				if r.BrandMentioned {
					hit = true
					v.Engine = r.Engine
					break
				}
			}
			v.BrandHit = &hit
			result.ProbeCount += len(runs)
		}
	}
	return result, nil
}

type fanoutLLMResponse struct {
	Variants []struct {
		Category string `json:"category"`
		Query    string `json:"query"`
	} `json:"variants"`
}

// parseFanoutVariants 解析 LLM 返回（宽松：容错 category 空值与包裹文本）
// 注意 extractJSONObject 的贪婪正则会从裸数组里抓出首个内层对象，
// 因此对象路径解析不到 variants 时必须回退数组路径。
func parseFanoutVariants(content string) ([]FanoutVariant, error) {
	tryObj := extractJSONObject(content)
	if tryObj != "" {
		var parsed fanoutLLMResponse
		if err := json.Unmarshal([]byte(tryObj), &parsed); err == nil && len(parsed.Variants) > 0 {
			return normalizeFanoutVariants(parsed.Variants), nil
		}
	}
	tryArr := extractJSONArray(content)
	if tryArr == "" {
		return nil, fmt.Errorf("响应中无 JSON")
	}
	var arrParsed fanoutLLMResponse
	if err := json.Unmarshal([]byte(`{"variants":`+tryArr+`}`), &arrParsed); err != nil {
		return nil, err
	}
	if len(arrParsed.Variants) == 0 {
		return nil, fmt.Errorf("响应中无有效变体")
	}
	return normalizeFanoutVariants(arrParsed.Variants), nil
}

func normalizeFanoutVariants(vs []struct {
	Category string `json:"category"`
	Query    string `json:"query"`
}) []FanoutVariant {
	validCats := map[string]bool{"direct": true, "compare": true, "recommend": true, "faq": true, "negative": true}
	out := make([]FanoutVariant, 0, len(vs))
	seen := map[string]bool{}
	for _, v := range vs {
		q := strings.TrimSpace(v.Query)
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		cat := strings.ToLower(strings.TrimSpace(v.Category))
		if !validCats[cat] {
			cat = "direct"
		}
		out = append(out, FanoutVariant{Category: cat, Query: q})
	}
	return out
}
