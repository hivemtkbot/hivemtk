package dto

// KuaishouCardCreateRequest 创建快手卡片请求
type KuaishouCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签，逗号分隔
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// KuaishouCardUpdateRequest 更新快手卡片请求
type KuaishouCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`                  // 卡片ID
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签，逗号分隔
	LikeCount    int    `json:"like_count"`                              // 点赞数
	ShareCount   int    `json:"share_count"`                             // 分享数
	ViewCount    int    `json:"view_count"`                              // 浏览数
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// KuaishouCardResponse 快手卡片响应
type KuaishouCardResponse struct {
	ID           uint   `json:"id"`             // 卡片ID
	Title        string `json:"title"`          // 卡片标题
	Description  string `json:"description"`    // 卡片描述
	ImageURL     string `json:"image_url"`      // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`   // 跳转链接
	DomainPoolID *uint  `json:"domain_pool_id"` // 域名池ID
	ShortLinkURL string `json:"short_link_url"` // 短链URL
	ShortCode    string `json:"short_code"`     // 短链代码
	Tags         string `json:"tags"`           // 标签，逗号分隔
	ViewCount    int    `json:"view_count"`     // 浏览数
	LikeCount    int    `json:"like_count"`     // 点赞数
	ShareCount   int    `json:"share_count"`    // 分享数
	IsActive     bool   `json:"is_active"`      // 是否激活
	CreatedAt    string `json:"created_at"`     // 创建时间
	UpdatedAt    string `json:"updated_at"`     // 更新时间
}

// KuaishouCardListRequest 快手卡片列表请求
type KuaishouCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
	Keyword  string `form:"keyword"`                                     // 关键词搜索
	IsActive *bool  `form:"is_active"`                                   // 是否激活筛选
}

// KuaishouCardListResponse 快手卡片列表响应
type KuaishouCardListResponse struct {
	List  []KuaishouCardResponse `json:"list"`  // 卡片列表
	Total int64                  `json:"total"` // 总数
}

// KuaishouCardViewRequest 访问快手卡片请求
type KuaishouCardViewRequest struct {
	ID uint `json:"id" binding:"required"` // 卡片ID
}

// KuaishouCardViewResponse 访问快手卡片响应
type KuaishouCardViewResponse struct {
	ID          uint   `json:"id"`          // 卡片ID
	Title       string `json:"title"`       // 卡片标题
	Description string `json:"description"` // 卡片描述
	ImageURL    string `json:"image_url"`   // 卡片图片URL
	LinkURL     string `json:"link_url"`    // 跳转链接
	Tags        string `json:"tags"`        // 标签，逗号分隔
	LikeCount   int    `json:"like_count"`  // 点赞数
	ShareCount  int    `json:"share_count"` // 分享数
	ViewCount   int    `json:"view_count"`  // 浏览数
}
