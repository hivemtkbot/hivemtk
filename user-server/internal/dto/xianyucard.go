package dto

// XianyuCardCreateRequest 创建闲鱼卡片请求
type XianyuCardCreateRequest struct {
	Title        string `json:"title" binding:"required,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"required,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	IsActive     bool   `json:"is_active"`
}

// XianyuCardUpdateRequest 更新闲鱼卡片请求
type XianyuCardUpdateRequest struct {
	ID           uint   `json:"id" binding:"omitempty"`
	Title        string `json:"title" binding:"required,max=255"`
	Description  string `json:"description" binding:"omitempty,max=500"`
	ImageURL     string `json:"image_url" binding:"required,url"`
	RedirectURL  string `json:"redirect_url"`
	DomainPoolID uint   `json:"domain_pool_id"`
	Tags         string `json:"tags" binding:"omitempty,max=255"`
	LikeCount    int    `json:"like_count"`
	ShareCount   int    `json:"share_count"`
	ViewCount    int    `json:"view_count"`
	IsActive     bool   `json:"is_active"`
}

// XianyuCardListRequest 闲鱼卡片列表请求
type XianyuCardListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"keyword"`
	IsActive *bool  `form:"is_active"`
}

// XianyuCardResponse 闲鱼卡片响应
type XianyuCardResponse struct {
	ID           uint   `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
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
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// XianyuCardListResponse 闲鱼卡片列表响应
type XianyuCardListResponse struct {
	List      []XianyuCardResponse `json:"list"`
	Total     int64                `json:"total"`
	Page      int                  `json:"page"`
	PageSize  int                  `json:"page_size"`
	TotalPage int                  `json:"total_page"`
}
