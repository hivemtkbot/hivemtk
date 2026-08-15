package model

import (
	"time"
)

// AIGenerationType AI生成类型
type AIGenerationType string

const (
	AIGenerationTypeCopywriting AIGenerationType = "copywriting" 
	AIGenerationTypeTitle       AIGenerationType = "title"       
	AIGenerationTypeSummary     AIGenerationType = "summary"     
	AIGenerationTypeReply       AIGenerationType = "reply"       
	AIGenerationTypeTranslation AIGenerationType = "translation" 
	AIGenerationTypeRewrite     AIGenerationType = "rewrite"     
	AIGenerationTypeExpand      AIGenerationType = "expand"      
	AIGenerationTypePolish      AIGenerationType = "polish"      
	AIGenerationTypeKeywords    AIGenerationType = "keywords"    
	AIGenerationTypeDescription AIGenerationType = "description" 
	AIGenerationTypeAdCopy      AIGenerationType = "ad_copy"     
	AIGenerationTypeSocialPost  AIGenerationType = "social_post" 
	AIGenerationTypeEmail       AIGenerationType = "email"       
	AIGenerationTypeScript      AIGenerationType = "script"      
)

// AIGenerationRecord AI生成记录
type AIGenerationRecord struct {
	ID         uint             `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint             `gorm:"index;not null" json:"user_id"`
	Type       AIGenerationType `gorm:"type:varchar(20);not null" json:"type"`
	Input      string           `gorm:"type:text" json:"input"`
	Output     string           `gorm:"type:text" json:"output"`
	TemplateID uint             `gorm:"index" json:"template_id"`
	Model      string           `gorm:"type:varchar(50)" json:"model"`
	TokensUsed int              `json:"tokens_used"`
	IsSaved    bool             `gorm:"default:false" json:"is_saved"`
	IsFavorite bool             `gorm:"default:false" json:"is_favorite"`
	Rating     int              `json:"rating"` 
	CreatedAt  time.Time        `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (AIGenerationRecord) TableName() string {
	return "ai_generation_records"
}

// PromptTemplate 提示词模板
type PromptTemplate struct {
	ID          uint             `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string           `gorm:"type:varchar(100);not null" json:"name"`
	Type        AIGenerationType `gorm:"type:varchar(20);not null" json:"type"`
	Template    string           `gorm:"type:text;not null" json:"template"`
	Variables   string           `gorm:"type:text" json:"variables"` 
	Description string           `gorm:"type:varchar(255)" json:"description"`
	Example     string           `gorm:"type:text" json:"example"` 
	IsSystem    bool             `gorm:"default:false" json:"is_system"`
	Status      int              `gorm:"default:1" json:"status"`
	UseCount    int              `gorm:"default:0" json:"use_count"`
	CreatedAt   time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (PromptTemplate) TableName() string {
	return "prompt_templates"
}

// SystemPromptTemplates 系统预定义模板
var SystemPromptTemplates = []PromptTemplate{
	{
		Name:        "营销文案生成",
		Type:        AIGenerationTypeCopywriting,
		Template:    "请为以下产品/服务生成一段吸引人的营销文案：\n\n产品/服务：{{product}}\n目标受众：{{audience}}\n核心卖点：{{selling_points}}\n\n要求：\n1. 突出产品优势\n2. 语言生动有感染力\n3. 字数控制在{{word_count}}字左右",
		Variables:   `[{"name":"product","type":"string","required":true},{"name":"audience","type":"string","required":true},{"name":"selling_points","type":"string","required":true},{"name":"word_count","type":"number","default":200}]`,
		Description: "根据产品信息和目标受众生成营销文案",
		IsSystem:    true,
	},
	{
		Name:        "标题生成",
		Type:        AIGenerationTypeTitle,
		Template:    "请为以下内容生成{{count}}个吸引人的标题：\n\n内容摘要：{{content}}\n风格：{{style}}\n\n要求：\n1. 标题要有吸引力\n2. 长度适中\n3. 符合{{style}}风格",
		Variables:   `[{"name":"content","type":"string","required":true},{"name":"count","type":"number","default":5},{"name":"style","type":"string","default":"专业"}]`,
		Description: "根据内容生成多个标题选项",
		IsSystem:    true,
	},
	{
		Name:        "客服回复生成",
		Type:        AIGenerationTypeReply,
		Template:    "作为客服，请针对以下客户问题生成专业、友好的回复：\n\n客户问题：{{question}}\n产品信息：{{product_info}}\n\n要求：\n1. 语气友好专业\n2. 解决客户问题\n3. 必要时提供解决方案",
		Variables:   `[{"name":"question","type":"string","required":true},{"name":"product_info","type":"string","required":false}]`,
		Description: "生成专业的客服回复",
		IsSystem:    true,
	},
	{
		Name:        "内容改写",
		Type:        AIGenerationTypeRewrite,
		Template:    "请将以下内容改写，要求：\n\n原文：{{content}}\n改写风格：{{style}}\n改写目的：{{purpose}}\n\n注意：\n1. 保持原意不变\n2. 语言更加{{style}}\n3. 适合{{purpose}}场景",
		Variables:   `[{"name":"content","type":"string","required":true},{"name":"style","type":"string","default":"简洁明了"},{"name":"purpose","type":"string","default":"社交媒体发布"}]`,
		Description: "按指定风格改写内容",
		IsSystem:    true,
	},
	{
		Name:        "社交媒体帖子",
		Type:        AIGenerationTypeSocialPost,
		Template:    "请为{{platform}}平台生成一条帖子：\n\n主题：{{topic}}\n目标：{{goal}}\n\n要求：\n1. 符合{{platform}}平台特点\n2. 吸引用户互动\n3. 包含合适的表情符号\n4. 字数控制在{{word_count}}字以内",
		Variables:   `[{"name":"platform","type":"string","required":true},{"name":"topic","type":"string","required":true},{"name":"goal","type":"string","default":"增加品牌曝光"},{"name":"word_count","type":"number","default":200}]`,
		Description: "生成社交媒体平台帖子",
		IsSystem:    true,
	},
	{
		Name:        "广告文案",
		Type:        AIGenerationTypeAdCopy,
		Template:    "请为以下产品生成广告文案：\n\n产品名称：{{product_name}}\n产品特点：{{features}}\n目标人群：{{target_audience}}\n投放平台：{{platform}}\n\n要求：\n1. 突出产品核心卖点\n2. 吸引目标用户点击\n3. 包含行动号召\n4. 字数{{word_count}}字左右",
		Variables:   `[{"name":"product_name","type":"string","required":true},{"name":"features","type":"string","required":true},{"name":"target_audience","type":"string","required":true},{"name":"platform","type":"string","default":"微信"},{"name":"word_count","type":"number","default":100}]`,
		Description: "生成广告投放文案",
		IsSystem:    true,
	},
	{
		Name:        "销售话术",
		Type:        AIGenerationTypeScript,
		Template:    "请生成一段销售话术：\n\n产品/服务：{{product}}\n客户类型：{{customer_type}}\n销售场景：{{scenario}}\n\n要求：\n1. 开场白吸引注意\n2. 突出产品价值\n3. 处理常见异议\n4. 引导成交",
		Variables:   `[{"name":"product","type":"string","required":true},{"name":"customer_type","type":"string","default":"潜在客户"},{"name":"scenario","type":"string","default":"电话销售"}]`,
		Description: "生成销售场景话术",
		IsSystem:    true,
	},
	{
		Name:        "邮件撰写",
		Type:        AIGenerationTypeEmail,
		Template:    "请撰写一封邮件：\n\n邮件类型：{{email_type}}\n收件人：{{recipient}}\n主要内容：{{content}}\n语气：{{tone}}\n\n要求：\n1. 邮件主题明确\n2. 内容简洁专业\n3. 语气{{tone}}",
		Variables:   `[{"name":"email_type","type":"string","required":true},{"name":"recipient","type":"string","required":true},{"name":"content","type":"string","required":true},{"name":"tone","type":"string","default":"正式专业"}]`,
		Description: "生成各类邮件内容",
		IsSystem:    true,
	},
}

