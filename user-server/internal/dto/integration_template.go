package dto

// IntegrationTemplateResponse 对接模板响应
type IntegrationTemplateResponse struct {
	ID         uint64 `json:"id"`
	Code       string `json:"code"`
	Platform   string `json:"platform"`
	Category   string `json:"category"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	APIBase    string `json:"api_base"`
	AuthType   string `json:"auth_type"`
	AuthConfig string `json:"auth_config"`
	DocURL     string `json:"doc_url"`
	FieldMaps  string `json:"field_maps"`
	Endpoints  string `json:"endpoints"`
	BuiltIn    bool   `json:"is_built_in"`
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// IntegrationTemplateListResponse 列表响应
type IntegrationTemplateListResponse struct {
	List     []*IntegrationTemplateResponse `json:"list"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
}

// IntegrationTemplateCreateRequest 创建模板请求
type IntegrationTemplateCreateRequest struct {
	Code       string `json:"code" binding:"required"`
	Platform   string `json:"platform" binding:"required"`
	Category   string `json:"category"`
	Name       string `json:"name" binding:"required"`
	Version    string `json:"version"`
	APIBase    string `json:"api_base"`
	AuthType   string `json:"auth_type"`
	AuthConfig string `json:"auth_config"`
	DocURL     string `json:"doc_url"`
	FieldMaps  string `json:"field_maps"`
	Endpoints  string `json:"endpoints"`
	Enabled    *bool  `json:"enabled"`
	Remark     string `json:"remark"`
}

// IntegrationTemplateUpdateRequest 更新模板请求
type IntegrationTemplateUpdateRequest struct {
	Category   string `json:"category"`
	Name       string `json:"name"`
	APIBase    string `json:"api_base"`
	AuthConfig string `json:"auth_config"`
	DocURL     string `json:"doc_url"`
	FieldMaps  string `json:"field_maps"`
	Endpoints  string `json:"endpoints"`
	Enabled    *bool  `json:"enabled"`
	Remark     string `json:"remark"`
}
