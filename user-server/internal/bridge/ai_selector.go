package bridge

import (
	"net/http"

	"github.com/gin-gonic/gin"
)




// SelectorSpec 是 LLM 返回的标准选择器结构（与前端 selector-ai.js 协议一致）
type SelectorSpec struct {
	Version     int      `json:"version"`
	Channel     string   `json:"channel"`
	Domain      string   `json:"domain"`
	GeneratedAt int64    `json:"generated_at"`
	MessageList []string `json:"message_list"` 
	MessageItem []string `json:"message_item"` 
	Text        []string `json:"text"`         
	InputBox    []string `json:"input_box"`    
	SendButton  []string `json:"send_button"`  
	SelfMarker  []string `json:"self_marker"`  
	OtherMarker []string `json:"other_marker"` 
	Extractor string `json:"extractor,omitempty"`
	Notes     string `json:"notes,omitempty"`
}



type aiSelectorsResponse struct {
	Enabled bool          `json:"enabled"`
	Source  string        `json:"source"` 
	Spec    *SelectorSpec `json:"spec,omitempty"`
	Reason  string        `json:"reason,omitempty"`
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

















