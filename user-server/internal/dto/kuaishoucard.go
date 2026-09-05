package dto

// KuaishouCardCreateRequest 创建快手卡片请求
type KuaishouCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"omitempty,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	IsActive     bool   `json:"is_active"`
}

// KuaishouCardUpdateRequest 更新快手卡片请求
type KuaishouCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`
	Title        string `json:"title" binding:"required,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"omitempty,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	LikeCount    int    `json:"like_count"`
	ShareCount   int    `json:"share_count"`
	ViewCount    int    `json:"view_count"`
	IsActive     bool   `json:"is_active"`
}

// KuaishouCardResponse 快手卡片响应
type KuaishouCardResponse struct {
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

// KuaishouCardListRequest 快手卡片列表请求
type KuaishouCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"keyword"`
	IsActive *bool  `form:"is_active"`
}

// KuaishouCardListResponse 快手卡片列表响应
type KuaishouCardListResponse struct {
	List  []KuaishouCardResponse `json:"list"`
	Total int64                  `json:"total"`
}

// KuaishouCardViewRequest 访问快手卡片请求
type KuaishouCardViewRequest struct {
	ID uint `json:"id" binding:"required"`
}

// KuaishouCardViewResponse 访问快手卡片响应
type KuaishouCardViewResponse struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	LinkURL     string `json:"link_url"`
	Tags        string `json:"tags"`
	LikeCount   int    `json:"like_count"`
	ShareCount  int    `json:"share_count"`
	ViewCount   int    `json:"view_count"`
}
