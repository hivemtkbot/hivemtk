package model

import "encoding/json"

// RichCardType 卡片类型
type RichCardType string

const (
	CardTypeProduct RichCardType = "product" // 商品卡
	CardTypeOrder   RichCardType = "order"   // 订单卡
	CardTypePromo   RichCardType = "promo"   // 优惠/活动卡
	CardTypeGeneric RichCardType = "generic" // 通用卡
)

// CardButton 卡片动作按钮
type CardButton struct {
	Text   string `json:"text"`            // 按钮文案
	URL    string `json:"url,omitempty"`   // 跳转链接（外链/小程序等）
	Action string `json:"action,omitempty"` // 前端自定义动作标识（如 "copy", "buy"）
}

// RichCard 会话内结构化富卡片（商品卡/订单卡/优惠卡/通用卡）。
// 由智能体在对话中通过工具（card.show）产出，随回复一并下发到 web_embed / Telegram 等渠道。
// 在会话消息中以 JSON 形式存于 SessionMessage.CardData。
type RichCard struct {
	Type        RichCardType        `json:"type"`                   // product/order/promo/generic
	Title       string              `json:"title"`                  // 主标题（商品名/订单号/活动名）
	Subtitle    string              `json:"subtitle,omitempty"`     // 副标题
	Description string              `json:"description,omitempty"`  // 描述/卖点
	ImageURL    string              `json:"image_url,omitempty"`    // 主图（商品图/活动海报）
	ThumbURL    string              `json:"thumb_url,omitempty"`    // 缩略图
	Fields      map[string]string   `json:"fields,omitempty"`       // 结构化键值对（价格/规格/物流状态等）
	Buttons     []CardButton        `json:"buttons,omitempty"`      // 动作按钮
}

// MarshalRichCard 将 RichCard 序列化为 JSON 字符串（存库/透传用）
func MarshalRichCard(c *RichCard) (string, error) {
	if c == nil {
		return "", nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalRichCard 从 JSON 字符串解析 RichCard
func UnmarshalRichCard(s string) (*RichCard, error) {
	if s == "" {
		return nil, nil
	}
	var c RichCard
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	return &c, nil
}
