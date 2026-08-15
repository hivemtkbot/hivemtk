package dto

// TikTokCardCreateRequest 创建 TikTok 卡片请求
type TikTokCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`        
	Description  string `json:"description" binding:"omitempty,max=500"` 
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       
	RedirectURL  string `json:"redirect_url"`                            
	DomainPoolID uint   `json:"domain_pool_id"`                          
	Tags         string `json:"tags" binding:"omitempty,max=255"`        
	IsActive     bool   `json:"is_active"`                               
}

// TikTokCardUpdateRequest 更新 TikTok 卡片请求
type TikTokCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`                  
	Title        string `json:"title" binding:"required,max=255"`        
	Description  string `json:"description" binding:"omitempty,max=500"` 
	ImageURL     string `json:"image_url" binding:"omitempty,url"`       
	RedirectURL  string `json:"redirect_url"`                            
	DomainPoolID uint   `json:"domain_pool_id"`                          
	Tags         string `json:"tags" binding:"omitempty,max=255"`        
	ViewCount    int    `json:"view_count"`                              
	LikeCount    int    `json:"like_count"`                              
	ShareCount   int    `json:"share_count"`                             
	IsActive     bool   `json:"is_active"`                               
}

// TikTokCardResponse TikTok 卡片响应
type TikTokCardResponse struct {
	ID           uint   `json:"id"`             
	Title        string `json:"title"`          
	Description  string `json:"description"`    
	ImageURL     string `json:"image_url"`      
	RedirectURL  string `json:"redirect_url"`   
	DomainPoolID *uint  `json:"domain_pool_id"` 
	ShortLinkURL string `json:"short_link_url"` 
	ShortCode    string `json:"short_code"`     
	Tags         string `json:"tags"`           
	ViewCount    int    `json:"view_count"`     
	LikeCount    int    `json:"like_count"`     
	ShareCount   int    `json:"share_count"`    
	IsActive     bool   `json:"is_active"`      
	CreatedAt    string `json:"created_at"`     
	UpdatedAt    string `json:"updated_at"`     
}

// TikTokCardListRequest TikTok 卡片列表请求
type TikTokCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`              
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"` 
	Keyword  string `form:"keyword"`                                     
	IsActive *bool  `form:"is_active"`                                   
}

// TikTokCardListResponse TikTok 卡片列表响应
type TikTokCardListResponse struct {
	List  []TikTokCardResponse `json:"list"`  
	Total int64                `json:"total"` 
}

// TikTokCardStatsOverallResponse 总体统计响应
type TikTokCardStatsOverallResponse struct {
	TotalCards     int64                    `json:"total_cards"`     
	ActiveCards    int64                    `json:"active_cards"`    
	TotalViews     int64                    `json:"total_views"`     
	PopularCards   []TikTokCardResponse     `json:"popular_cards"`   
	DailyStats     []TikTokCardDailyStat    `json:"daily_stats"`     
	RecentActivity []TikTokCardActivityItem `json:"recent_activity"` 
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

