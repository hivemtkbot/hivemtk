package dto

// TikTokCardCreateRequest 创建 TikTok 卡片请求
type TikTokCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// TikTokCardUpdateRequest 更新 TikTok 卡片请求
type TikTokCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`                  // 卡片ID
	Title        string `json:"title" binding:"required,max=255"`        // 卡片标题
	Description  string `json:"description" binding:"omitempty,max=500"` // 卡片描述
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`                            // 跳转链接
	DomainPoolID uint   `json:"domain_pool_id"`                          // 域名池ID
	Tags         string `json:"tags" binding:"omitempty,max=255"`        // 标签
	ViewCount    int    `json:"view_count"`                              // 浏览数
	LikeCount    int    `json:"like_count"`                              // 点赞数
	ShareCount   int    `json:"share_count"`                             // 分享数
	IsActive     bool   `json:"is_active"`                               // 是否激活
}

// TikTokCardResponse TikTok 卡片响应
type TikTokCardResponse struct {
	ID           uint   `json:"id"`             // 卡片ID
	Title        string `json:"title"`          // 卡片标题
	Description  string `json:"description"`    // 卡片描述
	ImageURL     string `json:"image_url"`      // 卡片图片URL
	RedirectURL  string `json:"redirect_url"`   // 跳转链接
	DomainPoolID *uint  `json:"domain_pool_id"` // 域名池ID
	ShortLinkURL string `json:"short_link_url"` // 短链URL
	ShortCode    string `json:"short_code"`     // 短链代码
	Tags         string `json:"tags"`           // 标签
	ViewCount    int    `json:"view_count"`     // 浏览数
	LikeCount    int    `json:"like_count"`     // 点赞数
	ShareCount   int    `json:"share_count"`    // 分享数
	IsActive     bool   `json:"is_active"`      // 是否激活
	CreatedAt    string `json:"created_at"`     // 创建时间
	UpdatedAt    string `json:"updated_at"`     // 更新时间
}

// TikTokCardListRequest TikTok 卡片列表请求
type TikTokCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
	Keyword  string `form:"keyword"`                                     // 关键词搜索
	IsActive *bool  `form:"is_active"`                                   // 是否激活筛选
}

// TikTokCardListResponse TikTok 卡片列表响应
type TikTokCardListResponse struct {
	List  []TikTokCardResponse `json:"list"`  // 卡片列表
	Total int64                `json:"total"` // 总数
}

// TikTokCardStatsOverallResponse 总体统计响应
type TikTokCardStatsOverallResponse struct {
	TotalCards     int64                    `json:"total_cards"`     // 总卡片数
	ActiveCards    int64                    `json:"active_cards"`    // 激活卡片数
	TotalViews     int64                    `json:"total_views"`     // 总浏览数
	PopularCards   []TikTokCardResponse     `json:"popular_cards"`   // 热门卡片
	DailyStats     []TikTokCardDailyStat    `json:"daily_stats"`     // 每日统计
	RecentActivity []TikTokCardActivityItem `json:"recent_activity"` // 最近活动
}

// TikTokCardDailyStat 每日统计
type TikTokCardDailyStat struct {
	Date      string `json:"date"`
	ViewCount int64  `json:"view_count"`
}

// TikTokCardActivityItem 活动记录
type TikTokCardActivityItem struct {
	CardTitle string `json:"card_title"`
	Action    string `json:"action"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

// TikTokCardStatsDetailResponse 单卡片统计详情
type TikTokCardStatsDetailResponse struct {
	CardID         uint                     `json:"card_id"`
	Title          string                   `json:"title"`
	ViewCount      int                      `json:"view_count"`
	DailyStats     []TikTokCardDailyStat    `json:"daily_stats"`
	RecentActivity []TikTokCardActivityItem `json:"recent_activity"`
}
