package model

import "encoding/json"

// RichCardType 卡片类型
type RichCardType string

const (
	CardTypeProduct RichCardType = "product"
	CardTypeOrder   RichCardType = "order"
	CardTypePromo   RichCardType = "promo"
	CardTypeGeneric RichCardType = "generic"
)

// CardButton 卡片动作按钮
type CardButton struct {
	Text   string `json:"text"`
	URL    string `json:"url,omitempty"`
	Action string `json:"action,omitempty"`
}

// RichCard 会话内结构化富卡片（商品卡/订单卡/优惠卡/通用卡）。
// 由智能体在对话中通过工具（card.show）产出，随回复一并下发到 web_embed / Telegram 等渠道。
// 在会话消息中以 JSON 形式存于 SessionMessage.CardData。
type RichCard struct {
	Type        RichCardType      `json:"type"`
	Title       string            `json:"title"`
	Subtitle    string            `json:"subtitle,omitempty"`
	Description string            `json:"description,omitempty"`
	ImageURL    string            `json:"image_url,omitempty"`
	ThumbURL    string            `json:"thumb_url,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	Buttons     []CardButton      `json:"buttons,omitempty"`
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
