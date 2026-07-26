// Package dto 提供 AssetBundle（资产包）的 API 传输对象。
//
// 方向9：资产包模式 - 严格的 API 入参/出参契约
// 文档依据：docs/企业级架构优化/资产包模式.md §一/§六
//
// 关键设计：
//  1. 开发者模式：直接双向绑定 messages 数组（Playground）
//  2. 商户模式：低代码表单（促销活动/优惠比例/常见问答卡片）→ 后端翻译成 messages
//  3. DTO 统一管理这两套视图，让 Service 保持业务内聚
package dto

import (
	"marketing/internal/model"
)

// AssetBundleCreateRequest 创建资产包请求
type AssetBundleCreateRequest struct {
	AssetID     string                     `json:"asset_id" binding:"required"`
	Title       string                     `json:"title" binding:"required"`
	Description string                     `json:"description"`
	Author      string                     `json:"author"`
	Version     string                     `json:"version"`
	Scope       model.AssetBundleScope     `json:"scope"`
	Industry    string                     `json:"industry"`
	Language    string                     `json:"language"`
	Tags        []string                   `json:"tags"`
	Messages    []model.AssetBundleMessage `json:"messages" binding:"required"`
}

// AssetBundleUpdateRequest 更新资产包请求
type AssetBundleUpdateRequest struct {
	ID          int64                      `json:"id" binding:"required"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	Author      string                     `json:"author"`
	Version     string                     `json:"version"`
	Scope       model.AssetBundleScope     `json:"scope"`
	Industry    string                     `json:"industry"`
	Language    string                     `json:"language"`
	Tags        []string                   `json:"tags"`
	Messages    []model.AssetBundleMessage `json:"messages"`
	Status      model.AssetBundleStatus    `json:"status"`
	ChangeNote  string                     `json:"change_note"`
}

// AssetBundleListRequest 列表查询请求
type AssetBundleListRequest struct {
	Keyword  string                  `json:"keyword" form:"keyword"`
	Author   string                  `json:"author" form:"author"`
	Industry string                  `json:"industry" form:"industry"`
	Language string                  `json:"language" form:"language"`
	Scope    model.AssetBundleScope  `json:"scope" form:"scope"`
	Status   model.AssetBundleStatus `json:"status" form:"status"`
	Tags     []string                `json:"tags" form:"tags"`
	Page     int                     `json:"page" form:"page"`
	Size     int                     `json:"size" form:"size"`
}

// AssetBundleResponse 资产包响应（基础）
type AssetBundleResponse struct {
	*model.AssetBundle
}

// AssetBundleListResponse 列表响应
type AssetBundleListResponse struct {
	List  []*model.AssetBundle `json:"list"`
	Total int64                `json:"total"`
	Page  int                  `json:"page"`
	Size  int                  `json:"size"`
}

// ============================================================================
// 商户低代码模式 DTO（方向9 §六）
// ============================================================================

// MerchantFormSaveRequest 商户低代码表单保存请求
//
// 文档：方向9 §六
// 商户在前端看到的不是 messages 数组，而是表单（活动名称/优惠比例/常见问答卡片）
// 后端把表单反向翻译成标准 messages 数组
type MerchantFormSaveRequest struct {
	AssetID string `json:"asset_id" binding:"required"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	// 基础经营策略（注入到 system 末尾）
	ShopName       string `json:"shop_name"`
	CampaignName   string `json:"campaign_name"`
	DiscountPct    string `json:"discount_pct"`
	SupportContact string `json:"support_contact"`
	// 6 维拟人门禁
	CrisisThreshold string   `json:"crisis_threshold"` // 危机感触发阈值
	ToneLevel       string   `json:"tone_level"`       // 语气词等级
	CensorshipLevel string   `json:"censorship_level"` // 反审查尺度
	EnabledIntents  []string `json:"enabled_intents"`
	// 常见问答卡片（商户自定义的 Few-Shots）
	QACards []MerchantQACard `json:"qa_cards"`
	// 乐高卡片
	CardConfig MerchantCardConfig `json:"card_config"`
	// 模板包（开发者原始 messages；商户可在此基础上覆盖）
	TemplateAssetID string `json:"template_asset_id"`
}

// MerchantQACard 常见问答卡片
type MerchantQACard struct {
	ID          string `json:"id"`           // 卡片 ID（前端生成）
	Trigger     string `json:"trigger"`      // 触发关键词/场景描述
	UserExample string `json:"user_example"` // Few-Shot 中 user 的示例
	Reply       string `json:"reply"`        // Few-Shot 中 assistant 的回复
	Order       int    `json:"order"`        // 显示顺序
}

// MerchantCardConfig 乐高卡片配置
type MerchantCardConfig struct {
	IntentType   string               `json:"intent_type"` // button_card / coupon / handoff
	ProductImage string               `json:"product_image"`
	Buttons      []MerchantCardButton `json:"buttons"`
}

// MerchantCardButton 卡片按钮
type MerchantCardButton struct {
	Title   string `json:"title"`
	Action  string `json:"action"` // open_url / call_api
	URL     string `json:"url,omitempty"`
	APIName string `json:"api_name,omitempty"` // 当 action=call_api
	APIArgs string `json:"api_args,omitempty"` // JSON 字符串
	Order   int    `json:"order"`
}

// MerchantFormParseResponse 解析 messages 数组后的商户表单响应
//
// 文档：方向9 §六
// 前端用正则解析 messages[0].content 提取参数
// 解析结果返回给前端用于回显
type MerchantFormParseResponse struct {
	ShopName       string             `json:"shop_name"`
	CampaignName   string             `json:"campaign_name"`
	DiscountPct    string             `json:"discount_pct"`
	SupportContact string             `json:"support_contact"`
	QACards        []MerchantQACard   `json:"qa_cards"`
	CardConfig     MerchantCardConfig `json:"card_config"`
	EnabledIntents []string           `json:"enabled_intents"`
	// 6 维拟人门禁指标阀门（保存时写入 system 快照，编辑回显时还原）
	CrisisThreshold string `json:"crisis_threshold"`
	ToneLevel       string `json:"tone_level"`
	CensorshipLevel string `json:"censorship_level"`
}

// WeaveRequest Weave 算法请求
type WeaveRequest struct {
	AssetID      string                     `json:"asset_id" binding:"required"`
	UserQuery    string                     `json:"user_query" binding:"required"`
	RAGDocs      []RAGDocumentDTO           `json:"rag_docs"`
	ChatHistory  []model.AssetBundleMessage `json:"chat_history"`
	MerchantVars map[string]string          `json:"merchant_vars"`
	Options      *WeaveOptionsDTO           `json:"options"`
	// 沙箱/预览模式：开发者 Playground 本地试运行置 true，跳过热插拔门禁与用量累加
	Sandbox bool `json:"sandbox"`
}

// WeaveResponse Weave 响应
type WeaveResponse struct {
	Messages     []model.AssetBundleMessage `json:"messages"`
	ResultLength int                        `json:"result_length"`
	Stats        WeaveStats                 `json:"stats"`
}

// WeaveStats 织布统计
type WeaveStats struct {
	AssetMessages    int `json:"asset_messages"`
	RAGMessages      int `json:"rag_messages"`
	HistoryMessages  int `json:"history_messages"`
	FinalTotal       int `json:"final_total"`
	StrippedFewShots int `json:"stripped_few_shots"`
}

// RAGDocumentDTO RAG 文档传输对象
type RAGDocumentDTO struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
}

// WeaveOptionsDTO Weave 策略 DTO
type WeaveOptionsDTO struct {
	RAGPosition         string `json:"rag_position"`
	MaxHistoryMessages  int    `json:"max_history_messages"`
	StripFewShotJSON    bool   `json:"strip_few_shot_json"`
	IncludeMerchantVars bool   `json:"include_merchant_vars"`
}
