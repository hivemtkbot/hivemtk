package dto

// XiaohongshuCardCreateRequest 创建小红书卡片请求
type XiaohongshuCardCreateRequest struct {
	Title        string `json:"title" binding:"omitempty,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"required,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID *uint  `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	IsActive     bool   `json:"is_active"`
}

// XiaohongshuCardUpdateRequest 更新小红书卡片请求
type XiaohongshuCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`
	Title        string `json:"title" binding:"omitempty,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"required,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID *uint  `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	ViewCount    int    `json:"view_count"`
	IsActive     bool   `json:"is_active"`
}

// XiaohongshuCardListRequest 小红书卡片列表请求
type XiaohongshuCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"keyword"`
	IsActive *bool  `form:"is_active"`
}

// XiaohongshuCardResponse 小红书卡片响应
type XiaohongshuCardResponse struct {
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
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// XiaohongshuCardListResponse 小红书卡片列表响应
type XiaohongshuCardListResponse struct {
	List  []XiaohongshuCardResponse `json:"list"`
	Total int64                     `json:"total"`
}
