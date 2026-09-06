package dto

// DouyinCardCreateRequest 创建抖音卡片请求
type DouyinCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`
	Description  string `json:"description" binding:"max=500"`
	Content      string `json:"content" binding:"max=1000"`
	ImageURL     string `json:"image_url" binding:"omitempty,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"max=255"`
	IsActive     bool   `json:"is_active"`
	TemplateID   uint   `json:"template_id"`
	Status       string `json:"status"`
}

// DouyinCardUpdateRequest 更新抖音卡片请求
type DouyinCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`
	Title        string `json:"title" binding:"omitempty,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	Content      string `json:"content" binding:"omitempty,max=1000"`
	ImageURL     string `json:"image_url" binding:"omitempty,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	LikeCount    int    `json:"like_count"`
	ShareCount   int    `json:"share_count"`
	ViewCount    int    `json:"view_count"`
	IsActive     bool   `json:"is_active"`
	Status       string `json:"status"`
}

// DouyinCardListRequest 抖音卡片列表请求
type DouyinCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"keyword"`
	IsActive *bool  `form:"is_active"`
}

// DouyinCardResponse 抖音卡片响应
type DouyinCardResponse struct {
	ID           uint   `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Content      string `json:"content"`
	ImageURL     string `json:"image_url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID *uint  `json:"domain_pool_id"`
	ShortLinkURL string `json:"short_link_url"`
	ShortCode    string `json:"short_code"`
	Tags         string `json:"tags"`
	LikeCount    int    `json:"like_count"`
	ShareCount   int    `json:"share_count"`
	ViewCount    int    `json:"view_count"`
	IsActive     bool   `json:"is_active"`
	Status       string `json:"status"`
	TemplateID   uint   `json:"template_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// DouyinCardListResponse 抖音卡片列表响应
type DouyinCardListResponse struct {
	List  []DouyinCardResponse `json:"list"`
	Total int64                `json:"total"`
}

// DouyinCardViewRequest 访问抖音卡片请求
type DouyinCardViewRequest struct {
	ID uint `json:"id" binding:"required"`
}

// DouyinCardViewResponse 访问抖音卡片响应
type DouyinCardViewResponse struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	RedirectURL string `json:"redirect_url"`
	Tags        string `json:"tags"`
	LikeCount   int    `json:"like_count"`
	ShareCount  int    `json:"share_count"`
	ViewCount   int    `json:"view_count"`
}
