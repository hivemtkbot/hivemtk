package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hivemtk-user/internal/identity"
)

// UnifiedID Prefix constants
//
// 2026-08-16 严肃化：13 渠道全覆盖。每加新渠道必须同步加 prefix + 字段。
const (
	unifiedIDPrefixPhone        = "phone:"
	unifiedIDPrefixEmail        = "email:"
	unifiedIDPrefixTelegram     = "telegram:"
	unifiedIDPrefixWhatsApp     = "whatsapp:"
	unifiedIDPrefixWechat       = "wechat:" // 微信公众号
	unifiedIDPrefixFeishu       = "feishu:"
	unifiedIDPrefixWeCom        = "wecom:" // 企业微信 external_userid
	unifiedIDPrefixDouyin       = "douyin:"
	unifiedIDPrefixTikTok       = "tiktok:"
	unifiedIDPrefixKuaishou     = "kuaishou:"
	unifiedIDPrefixXiaohongshu  = "xiaohongshu:"
	unifiedIDPrefixXianyu       = "xianyu:"
	unifiedIDPrefixSMS          = "sms:"     // 短信即手机号
	unifiedIDPrefixEmailContact = "email_c:" // email 也作为联系方式
)

// Customer 客户模型 - CDP 统一客户数据
//
// 13 渠道完整身份字段：每个渠道一个 OpenID/UserID 字段。
// UnifiedID 生成时按优先级选取一个非空字段作为唯一键。
// 任何渠道的入站消息都能通过 OneID 反查到客户。
type Customer struct {
	ID        string `gorm:"type:varchar(36);primaryKey" json:"id"`
	UnifiedID string `gorm:"type:varchar(128);uniqueIndex" json:"unified_id"`
	Name      string `gorm:"type:varchar(100);index" json:"name"`

	// 强标识（用作 OneID 主键候选）
	Phone     string `gorm:"type:varchar(20);index" json:"phone"`
	PhoneHash string `gorm:"type:varchar(64);index;column:phone_hash" json:"-"`
	Email     string `gorm:"type:varchar(100);index" json:"email"`

	// 13 渠道 OpenID / UserID（每个渠道独立字段，可与 phone/email 并存）
	//
	// 2026-08-17 严肃化：gorm column tag 指向 DB 实际列名
	// （历史 model 用 `whatsapp_phone/wecom_external_id/tiktok_open_id`，
	//  DB 实际是 `whats_app_phone/we_com_external_id/tik_tok_open_id`，
	//  缺 column tag 导致 GORM 读写全部为 NULL，本节添加显式 mapping 修复）
	TelegramChatID   int64  `gorm:"type:bigint;index;column:telegram_chat_id" json:"telegram_chat_id"`
	TelegramUsername string `gorm:"type:varchar(100);index;column:telegram_username" json:"telegram_username"`
	WhatsAppPhone    string `gorm:"type:varchar(20);index;column:whats_app_phone" json:"whatsapp_phone"`
	WechatOpenID     string `gorm:"type:varchar(64);index;column:wechat_open_id" json:"wechat_open_id"`
	FeishuOpenID     string `gorm:"type:varchar(64);index;column:feishu_open_id" json:"feishu_open_id"`
	WeComExternalID  string `gorm:"type:varchar(64);index;column:we_com_external_id" json:"wecom_external_id"`
	DouyinOpenID     string `gorm:"type:varchar(64);index;column:douyin_open_id" json:"douyin_open_id"`
	TikTokOpenID     string `gorm:"type:varchar(64);index;column:tik_tok_open_id" json:"tiktok_open_id"`
	KuaishouOpenID   string `gorm:"type:varchar(64);index;column:kuaishou_open_id" json:"kuaishou_open_id"`
	XiaohongshuID    string `gorm:"type:varchar(64);index;column:xiaohongshu_id" json:"xiaohongshu_id"`
	XianyuID         string `gorm:"type:varchar(64);index;column:xianyu_id" json:"xianyu_id"`

	// 标签 + 评分
	Tags      string `gorm:"type:text" json:"tags"`
	RFMScore  int    `gorm:"default:0" json:"rfm_score"`
	ChurnRisk string `gorm:"type:varchar(20);default:'low'" json:"churn_risk"`

	// S-4 专属坐席（2026-08-26）：客户绑定的专属人工坐席（agent_statuses.agent_id，可空）。
	// 会话分配时优先路由给该坐席（在线且有容量），否则回退现有分配算法。
	OwnerAgentID *uint `gorm:"index;column:owner_agent_id" json:"owner_agent_id,omitempty"`

	// 时间
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName 返回表名
func (Customer) TableName() string {
	return "customers"
}

// BeforeCreate 创建前钩子 - 自动生成 ID、OneID 和 phone_hash
func (c *Customer) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}

	if c.UnifiedID == "" {
		c.UnifiedID = GenerateCustomerUnifiedID(c)
	}

	if c.Phone != "" && c.PhoneHash == "" {
		c.PhoneHash = hashPhone(c.Phone)
	}

	return nil
}

// hashPhone 对手机号做哈希（P0-05 隐私合规）
// v7 审计修复：统一委托 identity.PhoneHash（盐化），与 service 层派生/查询算法一致。
// 原无盐 sha256 与 identity.PhoneHash(盐化) 不一致，导致 phone_hash 写读两套哈希值、查询必失效。
func hashPhone(phone string) string {
	return identity.PhoneHash(phone)
}

// GenerateCustomerUnifiedID 根据优先级生成统一 ID（包级函数）
//
// 优先级：Phone > Email > WhatsApp > Telegram > 微信 > 飞书 > 企微 > 抖音 > TikTok > 快手 > 小红书 > 闲鱼
//
// 同一客户多渠道身份会共享一个 OneID；多账号绑定通过 CustomerChannels 副表。
func GenerateCustomerUnifiedID(c *Customer) string {
	if c.Phone != "" {
		// v3 审计 P0-2：改用盐化哈希派生，unified_id 不再携带明文手机号
		return identity.UnifiedIDFromPhone(c.Phone)
	}
	if c.Email != "" {
		return unifiedIDPrefixEmail + c.Email
	}
	if c.WhatsAppPhone != "" {
		return unifiedIDPrefixWhatsApp + c.WhatsAppPhone
	}
	if c.TelegramChatID != 0 {
		return unifiedIDPrefixTelegram + int64ToStr(c.TelegramChatID)
	}
	if c.WechatOpenID != "" {
		return unifiedIDPrefixWechat + c.WechatOpenID
	}
	if c.FeishuOpenID != "" {
		return unifiedIDPrefixFeishu + c.FeishuOpenID
	}
	if c.WeComExternalID != "" {
		return unifiedIDPrefixWeCom + c.WeComExternalID
	}
	if c.DouyinOpenID != "" {
		return unifiedIDPrefixDouyin + c.DouyinOpenID
	}
	if c.TikTokOpenID != "" {
		return unifiedIDPrefixTikTok + c.TikTokOpenID
	}
	if c.KuaishouOpenID != "" {
		return unifiedIDPrefixKuaishou + c.KuaishouOpenID
	}
	if c.XiaohongshuID != "" {
		return unifiedIDPrefixXiaohongshu + c.XiaohongshuID
	}
	if c.XianyuID != "" {
		return unifiedIDPrefixXianyu + c.XianyuID
	}
	return uuid.New().String()
}

func int64ToStr(v int64) string {
	b, _ := json.Marshal(v)
	s, _ := json.Marshal(string(b))
	return string(s[1 : len(s)-1])
}

// GetCustomerTags 获取标签数组（包级函数）
func GetCustomerTags(c *Customer) []string {
	if c.Tags == "" {
		return []string{}
	}
	var tags []string
	err := json.Unmarshal([]byte(c.Tags), &tags)
	if err != nil {
		return []string{}
	}
	return tags
}

// SetCustomerTags 设置标签数组（包级函数）
func SetCustomerTags(c *Customer, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	c.Tags = string(data)
	return nil
}
