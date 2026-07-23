package agent_runtime

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ============================================================================
// 业务结算提取器（资产包模式文档 §六 强制业务结算协议）
//
// 协议：
//   {
//     "intent": "faq / lead_capture / human_transfer",
//     "captured_data": {"whatsapp": "...", "email": "...", "product": "...", "quantity": "..."}
//   }
//
// 实现策略：
//  1. 优先解析末尾 ```json ... ``` 代码块（标准格式）
//  2. 退化解析末尾 {"intent":...} 裸 JSON 段（兼容模型未遵守协议时仍输出）
//  3. 都找不到时返回 nil, nil（业务降级，按 FAQ 处理）
// ============================================================================

// BusinessIntentResult 业务结算结果（资产包模式 §六 协议）
//
// 字段：
//   - Intent       业务结算标签（faq / lead_capture / human_transfer）
//   - CapturedData 捕获到的业务字段（统一字符串化，便于上层直接落库）
//   - RawJSON      原始 JSON 字符串（用于审计/回放/调试）
type BusinessIntentResult struct {
	Intent       string            `json:"intent"`
	CapturedData map[string]string `json:"captured_data"`
	RawJSON      string            `json:"raw_json,omitempty"`
}

// DefaultIntentExtractor 默认业务结算提取器
//
// 不依赖任何外部状态，纯函数式实现，可单测
type DefaultIntentExtractor struct{}

// Extract 从 LLM 响应文本中提取业务结算 JSON 块
func (DefaultIntentExtractor) Extract(reply string) (*BusinessIntentResult, error) {
	if reply == "" {
		return nil, nil
	}

	// 1. 优先解析 ```json ... ``` 代码块
	if ir := extractFromCodeBlock(reply); ir != nil {
		return ir, nil
	}

	// 2. 退化解析裸 JSON 段 {"intent":...}
	if ir := extractFromBareJSON(reply); ir != nil {
		return ir, nil
	}

	return nil, nil
}

// intentJSONBlockRE 匹配末尾 ```json ... ``` 代码块
//
// (?s) 让 . 匹配换行；非贪婪 [\\s\\S]*? 防止跨块吞噬
var intentJSONBlockRE = regexp.MustCompile("(?s)```(?:json|JSON)?\\s*(\\{[\\s\\S]*?\\})\\s*```")

// intentBareRE 匹配裸 {"intent": ...} JSON 段（最外层大括号匹配）
var intentBareStartRE = regexp.MustCompile(`\{"intent"\s*:`)

// extractFromCodeBlock 从 ```json {...} ``` 代码块提取
func extractFromCodeBlock(reply string) *BusinessIntentResult {
	matches := intentJSONBlockRE.FindAllStringSubmatch(reply, -1)
	if len(matches) == 0 {
		return nil
	}
	// 取最后一个匹配（业务结算 JSON 总在末尾）
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return nil
	}
	rawJSON := strings.TrimSpace(last[1])
	return parseIntentJSON(rawJSON)
}

// extractFromBareJSON 从裸 {"intent":...} JSON 段提取
//
// 找最后一个 {"intent" 起始位置，按大括号深度匹配到结尾
func extractFromBareJSON(reply string) *BusinessIntentResult {
	idxs := intentBareStartRE.FindAllStringIndex(reply, -1)
	if len(idxs) == 0 {
		return nil
	}
	lastStart := idxs[len(idxs)-1][0]
	// 从 lastStart 起按大括号深度匹配到结尾
	depth := 0
	end := -1
	for i := lastStart; i < len(reply); i++ {
		switch reply[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}
	rawJSON := reply[lastStart : end+1]
	return parseIntentJSON(rawJSON)
}

// parseIntentJSON 解析 JSON 字符串为 BusinessIntentResult
//
// 容错：
//   - JSON 解析失败返回 nil（不报错，避免日志噪声）
//   - intent 字段允许缺失（默认 faq）
//   - captured_data 字段允许缺失（默认空 map）
//   - 非预期字段值类型不强校验（模型可能输出 number 而非 string）
func parseIntentJSON(raw string) *BusinessIntentResult {
	var dec struct {
		Intent       string            `json:"intent"`
		CapturedData map[string]any    `json:"captured_data"`
	}
	if err := json.Unmarshal([]byte(raw), &dec); err != nil {
		return nil
	}
	ir := &BusinessIntentResult{
		Intent:       dec.Intent,
		CapturedData: make(map[string]string, len(dec.CapturedData)),
		RawJSON:      raw,
	}
	if ir.Intent == "" {
		ir.Intent = "faq"
	}
	for k, v := range dec.CapturedData {
		switch val := v.(type) {
		case string:
			ir.CapturedData[k] = val
		case float64:
			ir.CapturedData[k] = strings.TrimRight(strings.TrimRight(
				strings.TrimSpace(formatBusinessFloat(val)), "0"), ".")
		case bool:
			if val {
				ir.CapturedData[k] = "true"
			} else {
				ir.CapturedData[k] = "false"
			}
		case nil:
			// 保留空字符串便于上层判断
			ir.CapturedData[k] = ""
		default:
			// 复杂类型（数组/对象）序列化为字符串
			if b, err := json.Marshal(val); err == nil {
				ir.CapturedData[k] = string(b)
			}
		}
	}
	return ir
}

// formatBusinessFloat 格式化 float 为字符串（避免科学计数法）
//
// 注：刻意加 Business 前缀避免与 alignment_stage.go 已有的 formatFloat 冲突
func formatBusinessFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
