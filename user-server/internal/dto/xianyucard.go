package dto

// XianyuCardCreateRequest 创建闲鱼卡片请求
type XianyuCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"required,url"`        // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签，逗号分隔
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// XianyuCardUpdateRequest 更新闲鱼卡片请求
type XianyuCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`                  // 卡片ID
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"required,url"`        // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签，逗号分隔
	LikeCount    int    `json:"like_count"`                              // 点赞数
	ShareCount   int    `json:"share_count"`                             // 分享数
	ViewCount    int    `json:"view_count"`                              // 浏览数
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// XianyuCardListRequest 闲鱼卡片列表请求
type XianyuCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
	Keyword  string `form:"keyword"`                                     // 关键词搜索
	IsActive *bool  `form:"is_active"`                                   // 是否激活筛选
}

// XianyuCardResponse 闲鱼卡片响应
type XianyuCardResponse struct {
	ID           uint   `json:"id"`             // 卡片ID
	Title        string `json:"title"`          // 卡片标题
	Description  string `json:"description"`    // 卡片描述
	ImageURL     string `json:"image_url"`      // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`   // 跳转链接
	DomainPoolID *uint  `json:"domain_pool_id"` // 域名池ID
	ShortLinkURL string `json:"short_link_url"` // 短链URL
	ShortCode    string `json:"short_code"`     // 短码
	Tags         string `json:"tags"`           // 标签，逗号分隔
	LikeCount    int    `json:"like_count"`     // 点赞数
	ShareCount   int    `json:"share_count"`    // 分享数
	ViewCount    int    `json:"view_count"`     // 浏览数
	IsActive     bool   `json:"is_active"`      // 是否激活
	CreatedAt    string `json:"created_at"`     // 创建时间
	UpdatedAt    string `json:"updated_at"`     // 更新时间
}

// XianyuCardListResponse 闲鱼卡片列表响应
type XianyuCardListResponse struct {
	List      []XianyuCardResponse `json:"list"`       // 卡片列表
	Total     int64                `json:"total"`      // 总数
	Page      int                  `json:"page"`       // 当前页
	PageSize  int                  `json:"page_size"`  // 每页数量
	TotalPage int                  `json:"total_page"` // 总页数
}
