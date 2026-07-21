package dto

// DouyinCardCreateRequest 创建抖音卡片请求
type DouyinCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`  // 卡片标题
	Description  string `json:"description" binding:"max=500"`     // 卡片描述
	Content      string `json:"content" binding:"max=1000"`        // 卡片内容（兼容旧版）
	ImageURL     string `json:"image_url" binding:"omitempty,url"` // 卡片图片 URL（可选）
	RedirectURL  string `json:"redirect_url"`                      // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                    // 域名池 ID
	Tags         string `json:"tags" binding:"max=255"`            // 标签，逗号分隔
	IsActive     bool   `json:"is_active"`                         // 是否激活
	TemplateID   uint   `json:"template_id"`                       // 模板 ID（兼容测试）
	Status       string `json:"status"`                            // 状态（兼容测试：draft/published）
}

// DouyinCardUpdateRequest 更新抖音卡片请求
type DouyinCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`                  // 卡片 ID（可选，从 URL 路径获取）
	Title        string `json:"title" binding:"omitempty,max=255"`       // 卡片标题（可选，支持部分更新）
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述（可选）
	Content      string `json:"content" binding:"omitempty,max=1000"`    // 卡片内容（兼容旧版，可选）
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       // 卡片图片 URL（可选）
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池 ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签，逗号分隔（可选）
	LikeCount    int    `json:"like_count"`                              // 点赞数
	ShareCount   int    `json:"share_count"`                             // 分享数
	ViewCount    int    `json:"view_count"`                              // 浏览数
	IsActive     bool   `json:"is_active"`                               // 是否激活
	Status       string `json:"status"`                                  // 状态（兼容测试：draft/published）
}

// DouyinCardListRequest 抖音卡片列表请求
type DouyinCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`              // 页码（可选，默认 1）
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量（可选，默认 10）
	Keyword  string `form:"keyword"`                                     // 关键词搜索
	IsActive *bool  `form:"is_active"`                                   // 是否激活筛选
}

// DouyinCardResponse 抖音卡片响应
type DouyinCardResponse struct {
	ID           uint   `json:"id"`             // 卡片 ID
	Title        string `json:"title"`          // 卡片标题
	Description  string `json:"description"`    // 卡片描述
	Content      string `json:"content"`        // 卡片内容
	ImageURL     string `json:"image_url"`      // 卡片图片 URL
	RedirectURL  string `json:"redirect_url"`   // 跳转链接
	DomainPoolID *uint  `json:"domain_pool_id"` // 域名池 ID
	ShortLinkURL string `json:"short_link_url"` // 短链 URL
	ShortCode    string `json:"short_code"`     // 短码
	Tags         string `json:"tags"`           // 标签，逗号分隔
	LikeCount    int    `json:"like_count"`     // 点赞数
	ShareCount   int    `json:"share_count"`    // 分享数
	ViewCount    int    `json:"view_count"`     // 浏览数
	IsActive     bool   `json:"is_active"`      // 是否激活
	Status       string `json:"status"`         // 状态
	TemplateID   uint   `json:"template_id"`    // 模板 ID
	CreatedAt    string `json:"created_at"`     // 创建时间
	UpdatedAt    string `json:"updated_at"`     // 更新时间
}

// DouyinCardListResponse 抖音卡片列表响应
type DouyinCardListResponse struct {
	List  []DouyinCardResponse `json:"list"`  // 卡片列表
	Total int64                `json:"total"` // 总数
}

// DouyinCardViewRequest 访问抖音卡片请求
type DouyinCardViewRequest struct {
	ID uint `json:"id" binding:"required"` // 卡片 ID
}

// DouyinCardViewResponse 访问抖音卡片响应
type DouyinCardViewResponse struct {
	ID          uint   `json:"id"`           // 卡片 ID
	Title       string `json:"title"`        // 卡片标题
	Description string `json:"description"`  // 卡片描述
	ImageURL    string `json:"image_url"`    // 卡片图片 URL
	RedirectURL string `json:"redirect_url"` // 跳转链接
	Tags        string `json:"tags"`         // 标签，逗号分隔
	LikeCount   int    `json:"like_count"`   // 点赞数
	ShareCount  int    `json:"share_count"`  // 分享数
	ViewCount   int    `json:"view_count"`   // 浏览数
}
