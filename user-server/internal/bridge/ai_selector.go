package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"marketing/internal/aiagent/llm"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// AI 动态选择器端点（解耦硬编码选择器）
//
// 前端插件把「脱敏 DOM 结构快照」POST 上来 → 后端用 LLM 生成标准 SelectorSpec
// （消息列表/消息项/文本/输入框/发送按钮/自他标记 → 以及「可执行 JS 抽取器」）→ 返回给插件缓存执行。
//
// 这样「选择器」不再是前端写死的常量，而是「云端 LLM 生成的运行时产物」：
//   抖音网页改版 → 只需 LLM 重新识别 → 前端无需发版即可恢复抓取。
// LLM key 在本服务（本地 Mac，经 frp 穿透），不进前端、不经公网明文。
// 插件端：命中优先用缓存，连续 0 命中/无缓存时自动重新请求（自愈）。
//
// 2026-08-04 升级：主路径改为 LLM 直接返回「可执行 JS 抽取器」(spec.extractor)，
// 经 new Function 沙箱式执行，彻底不再依赖固定选择器 schema，最大限度兼容平台改版。
// ============================================================================

const selectorSpecVersion = 3

// extractorBlocklist 服务端静态校验黑名单：拒绝会外泄数据/联网/读写本地存储的抽取器，
// 防止 LLM 生成的 JS 在用户浏览器里做危险操作（前端运行期还会再校验一次，防御纵深）。
var extractorBlocklist = []string{
	"fetch(", "xmlhttprequest", "websocket(", "import(", "eval(", "new function",
	"localStorage", "sessionstorage", "document.cookie", "window.open(",
	"location.href", "location.assign", "location.replace", "chrome.storage",
	"postmessage", "navigator.sendbeacon", "indexeddb",
}

// SelectorSpec 是 LLM 返回的标准选择器结构（与前端 selector-ai.js 协议一致）
type SelectorSpec struct {
	Version     int      `json:"version"`
	Channel     string   `json:"channel"`
	Domain      string   `json:"domain"`
	GeneratedAt int64    `json:"generated_at"`
	MessageList []string `json:"message_list"` // 消息列表容器
	MessageItem []string `json:"message_item"` // 单条消息气泡（选择器兜底）
	Text        []string `json:"text"`         // 气泡内文本节点（选择器兜底）
	InputBox    []string `json:"input_box"`    // 输入框（contenteditable/textarea）
	SendButton  []string `json:"send_button"`  // 发送按钮
	SelfMarker  []string `json:"self_marker"`  // 标记「自己/发出」的气泡 class 或选择器
	OtherMarker []string `json:"other_marker"` // 标记「客户/收到」的气泡 class 或选择器
	// Extractor 是「可执行 JS 抽取器」源码：函数 (doc, win) => ({ messages, input_box, send_button })。
	// 这是**主路径**，彻底不依赖固定选择器 schema —— 即使平台 DOM 大改，LLM 重新生成即可恢复，
	// 前端经 new Function 沙箱式执行（仅读 DOM，无网络/存储）。无 LLM 或抽取器不可用时回退到上面选择器。
	Extractor string `json:"extractor,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

type aiSelectorsRequest struct {
	Channel     string `json:"channel"`
	Domain      string `json:"domain"`
	DomSnapshot string `json:"dom_snapshot"`
	SpecVersion int    `json:"spec_version"`
}

type aiSelectorsResponse struct {
	Enabled bool          `json:"enabled"`
	Source  string        `json:"source"` // "llm" | "cache" | "fallback"
	Spec    *SelectorSpec `json:"spec,omitempty"`
	Reason  string        `json:"reason,omitempty"`
}

// —— 服务端缓存：相同 (channel,domain,布局) 命中则直接返回，避免重复打 LLM ——
type specCacheEntry struct {
	spec  *SelectorSpec
	expAt time.Time
}

var (
	specCacheMu sync.Mutex
	specCache   = map[string]specCacheEntry{}
)

func specCacheKey(channel, domain, snapshot string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(channel + "|" + domain + "|" + snapshot))
	return fmt.Sprintf("%s|%d", channel, h.Sum64())
}

func specCacheGet(key string) (*SelectorSpec, bool) {
	specCacheMu.Lock()
	defer specCacheMu.Unlock()
	e, ok := specCache[key]
	if !ok || time.Now().After(e.expAt) {
		return nil, false
	}
	return e.spec, true
}

func specCachePut(key string, spec *SelectorSpec) {
	specCacheMu.Lock()
	defer specCacheMu.Unlock()
	specCache[key] = specCacheEntry{spec: spec, expAt: time.Now().Add(24 * time.Hour)}
}

// AISelectors 处理 POST /api/bridge/ai-selectors
//
// DEPRECATED（2026-08-04）：LLM 选择器架构已完全移除，改为纯 CSS 选择器 + UI 配置面板。
// API 保留兼容（不返回 404），始终返回 enabled=false，前端回退到本地规则引擎。
func AISelectors(c *gin.Context) {
	c.JSON(http.StatusOK, aiSelectorsResponse{
		Enabled: false,
		Source:  "fallback",
		Reason:  "deprecated: LLM selector generation removed, use UI config panel instead",
	})
}

// resolveLLMConfig 解析 LLM 配置，实现「全自动」启用：
//   - 运维可用 BRIDGE_DIAGNOSE_* 显式覆盖（最高优先）；
//   - 否则复用系统全局 Dispatcher 中已启用、带真实密钥的「云端厂商」，
//     跳过本地推理（默认 default，指向可能未运行的本地 8207 / host.docker.internal）。
//
// 返回 nil 表示无任何可用 LLM → 调用方应回退规则引擎（不抛错）。
func resolveLLMConfig() *llm.LLMConfig {
	// 1) 显式覆盖（最高优先级）
	if ak := os.Getenv("BRIDGE_DIAGNOSE_API_KEY"); ak != "" {
		return &llm.LLMConfig{
			APIKey:      ak,
			BaseURL:     getenvDefault("BRIDGE_DIAGNOSE_BASE_URL", "https://api.deepseek.com/v1"),
			Model:       getenvDefault("BRIDGE_DIAGNOSE_MODEL", "deepseek-chat"),
			MaxTokens:   8192,
			Temperature: 0.1,
		}
	}
	// 2) 复用系统全局 Dispatcher 中已启用且带真实密钥的云端厂商
	if d := llm.GetGlobalDispatcher(); d != nil {
		for _, p := range d.GetProviderList() {
			if !p.Enabled || p.APIKey == "" || p.BaseURL == "" {
				continue
			}
			if p.Name == "default" || isLocalhostURL(p.BaseURL) {
				continue // 跳过本地推理（默认 default，可能未运行）
			}
			return &llm.LLMConfig{
				APIKey:      p.APIKey,
				BaseURL:     p.BaseURL,
				Model:       p.Model,
				APIType:     p.APIType,
				MaxTokens:   8192,
				Temperature: 0.1,
			}
		}
	}
	return nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// isLocalhostURL 判断 BaseURL 是否指向本地/容器内地址（应跳过，避免打到未运行的本地推理）
func isLocalhostURL(u string) bool {
	for _, host := range []string{"127.0.0.1", "localhost", "0.0.0.0", "host.docker.internal", "::1"} {
		if strings.Contains(u, host) {
			return true
		}
	}
	return false
}

func generateSelectorSpec(ctx context.Context, req aiSelectorsRequest, cfg *llm.LLMConfig) (*SelectorSpec, error) {
	// 截断超大快照：推理模型会把 token 预算花在 reasoning 上，快照越长推理越久，
	// 容易把 max_tokens 耗尽导致最终答案(content)被截断。6000 字符对选择器识别已足够。
	snapshot := req.DomSnapshot
	const maxSnap = 6000
	if len(snapshot) > maxSnap {
		snapshot = snapshot[:maxSnap] + "\n...(snapshot truncated)"
	}
	prompt := buildSelectorPrompt(req, snapshot)

	svc := llm.NewLLMService()
	raw, err := svc.Generate(ctx, cfg, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	spec, err := parseSelectorSpec(raw, req)
	if err != nil {
		return nil, err
	}
	// 主路径是 extractor（可执行 JS 抽取器）；选择器仅作兜底。
	// 二者至少要有其一：若都没有，说明 LLM 没识别出任何可用结构。
	if len(spec.MessageItem) == 0 && spec.Extractor == "" {
		return nil, fmt.Errorf("llm returned neither message_item selectors nor an extractor")
	}
	return spec, nil
}

func buildSelectorPrompt(req aiSelectorsRequest, snapshot string) string {
	return fmt.Sprintf(`你是一个网页 DOM 结构分析器。下面是某 IM 网页（渠道=%q，域名=%q）的脱敏 DOM 快照（仅保留 tag/class/id/role/data-e2e，已剔除文本/图片/链接）。

只输出一个 JSON（不要解释、不要 markdown 代码块）：
{
  "extractor": "JS函数源码：function(doc, win){ return { conversation: {isGroup:boolean, groupId?:string, groupName:string}, messages:[{id,text,self:boolean,senderName,timestamp,msgType,mediaUrl,isGroup,groupId,groupName}], input_box:'sel', send_button:'sel' }; }",
  "message_list": ["容器选择器(兜底,可空)"],
  "message_item": ["消息气泡选择器(兜底,可空)"],
  "text": ["文本选择器(兜底,可空)"],
  "input_box": ["输入框选择器(兜底,可空)"],
  "send_button": ["发送按钮选择器(兜底,可空)"],
  "self_marker": ["自方标记(兜底,可空)"],
  "other_marker": ["对方标记(兜底,可空)"],
  "notes": "简短说明"
}

要求：
1. extractor 是【主路径】，务必给出。它是浏览器里运行的函数 function(doc,win){...}，【只能读取 DOM】，严禁 fetch/XMLHttpRequest/WebSocket/localStorage/sessionStorage/cookie/eval/页面跳转。messages 每项 {text:字符串, self:布尔(true=我方坐席,false=客户访客), senderName?:该轮发送者昵称(群聊必填), timestamp?:数字, msgType?:("text"|"image"|"video"|"file"), mediaUrl?:字符串, isGroup?:布尔(是否群聊消息), groupId?:字符串, groupName?:字符串}。self 用气泡 class(如 self/own/mine)或位置(自己靠右)判断。若为群聊：每条消息需尽力给出 senderName（@提及/昵称结构内的成员名），并给出 groupName；一对一私聊 isGroup=false。选择器优先 [class*="关键词"]。
2. 其余字段是【兜底选择器】，仅当 extractor 不可用前端才用；能确定就给，不确定填空数组。
3. 所有选择器必须基于快照真实存在的 class/属性，不要编造。
4. 直接给纯 JSON，不要任何说明文字。

DOM 快照：
%s`, req.Channel, req.Domain, snapshot)
}

// parseSelectorSpec 从 LLM 原始输出中提取并校验 SelectorSpec
func parseSelectorSpec(raw string, req aiSelectorsRequest) (*SelectorSpec, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in llm output")
	}
	jsonStr := raw[start : end+1]

	var partial struct {
		Extractor   string   `json:"extractor"`
		MessageList []string `json:"message_list"`
		MessageItem []string `json:"message_item"`
		Text        []string `json:"text"`
		InputBox    []string `json:"input_box"`
		SendButton  []string `json:"send_button"`
		SelfMarker  []string `json:"self_marker"`
		OtherMarker []string `json:"other_marker"`
		Notes       string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &partial); err != nil {
		return nil, fmt.Errorf("unmarshal selector spec: %w", err)
	}

	// 抽取器：有则必须安全，否则丢弃（不阻断，前端会回退到选择器）
	extractor := ""
	if partial.Extractor != "" {
		if err := sanitizeExtractor(partial.Extractor); err != nil {
			log.Printf("[bridge-ai] extractor failed safety check, dropped: %v", err)
		} else {
			extractor = partial.Extractor
		}
	}

	spec := &SelectorSpec{
		Version:     selectorSpecVersion,
		Channel:     req.Channel,
		Domain:      req.Domain,
		GeneratedAt: time.Now().Unix(),
		MessageList: dedupe(partial.MessageList),
		MessageItem: dedupe(partial.MessageItem),
		Text:        dedupe(partial.Text),
		InputBox:    dedupe(partial.InputBox),
		SendButton:  dedupe(partial.SendButton),
		SelfMarker:  dedupe(partial.SelfMarker),
		OtherMarker: dedupe(partial.OtherMarker),
		Extractor:   extractor,
		Notes:       partial.Notes,
	}
	return spec, nil
}

// sanitizeExtractor 对 LLM 返回的抽取器源码做静态安全校验：
// 拒绝任何会联网、读写本地存储、执行动态代码、跳转页面等的危险写法。
func sanitizeExtractor(code string) error {
	low := strings.ToLower(code)
	for _, bad := range extractorBlocklist {
		// 双向小写化：块名单为驼峰（localStorage/sessionStorage），低码比对需归一
		if strings.Contains(low, strings.ToLower(bad)) {
			return fmt.Errorf("forbidden token in extractor: %q", bad)
		}
	}
	// 必须长得像一个函数体：普通函数（function...）或箭头函数（=>）均可，
	// 与前端 selector-ai.js sanitizeExtractorCode 校验口径保持一致（避免服务端
	// 拒绝 LLM 返回的箭头函数、前端却放行的双向不一致）。
	if !strings.Contains(low, "function") && !strings.Contains(low, "=>") {
		return fmt.Errorf("extractor must be a function body (function or arrow)")
	}
	return nil
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
