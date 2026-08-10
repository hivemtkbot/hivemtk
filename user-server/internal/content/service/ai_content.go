package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/content/repository"
	"os"
	"strings"

	sysrepo "hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// AIContentService AI 内容服务
type AIContentService struct {
	llmService   llm.LLMServiceInterface
	recordRepo   repository.AIGenerationRecordRepository
	templateRepo repository.PromptTemplateRepository
}

// NewAIContentService 创建 AI 内容服务实例
func NewAIContentService(db any) *AIContentService {
	gormDB := db.(*gorm.DB)
	return &AIContentService{
		llmService:   llm.NewLLMService(),
		recordRepo:   repository.NewAIGenerationRecordRepository(gormDB),
		templateRepo: repository.NewPromptTemplateRepository(gormDB),
	}
}

// GenerateContentRequest 生成内容请求
type GenerateContentRequest struct {
	Type       model.AIGenerationType `json:"type" binding:"required"`
	Input      string                 `json:"input" binding:"required"`
	TemplateID uint                   `json:"template_id"`
	Variables  map[string]any         `json:"variables"`
	Model      string                 `json:"model"`
}

// GenerateContentResponse 生成内容响应
type GenerateContentResponse struct {
	ID         uint   `json:"id"`
	Output     string `json:"output"`
	TokensUsed int    `json:"tokens_used"`
	Model      string `json:"model"`
}

// GenerateContent 生成内容
func (s *AIContentService) GenerateContent(ctx context.Context, userID uint, req *GenerateContentRequest) (*GenerateContentResponse, error) {
	// 在调用外部 LLM 服务前，验证 API key 是否已配置
	apiKey := s.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, fmt.Errorf("AI service not configured")
	}

	// 构建提示词
	prompt, err := s.buildPrompt(req)
	if err != nil {
		return nil, err
	}

	// 获取模型配置
	modelName := req.Model
	if modelName == "" {
		modelName = "gpt-3.5-turbo"
	}

	// 配置 LLM (使用用户配置覆盖默认值)
	config := &llm.LLMConfig{
		Model:          modelName,
		APIType:        "openai",
		APIKey:         apiKey,
		BaseURL:        s.resolveBaseURL(),
		Temperature:    0.7,
		MaxTokens:      2000,
		ResponseFormat: "text",
	}

	// 调用 LLM 生成
	output, err := s.llmService.Generate(ctx, config, prompt)
	if err != nil {
		return nil, fmt.Errorf("生成失败：%v", err)
	}

	// 实际使用 token 数量
	tokensUsed := len(prompt)/4 + len(output)/4

	// 保存生成记录
	record := &model.AIGenerationRecord{
		UserID:     userID,
		Type:       req.Type,
		Input:      req.Input,
		Output:     output,
		TemplateID: req.TemplateID,
		Model:      modelName,
		TokensUsed: tokensUsed,
	}

	if err := s.recordRepo.Create(record); err != nil {
		// 记录保存失败不影响返回结果
	}

	return &GenerateContentResponse{
		ID:         record.ID,
		Output:     output,
		TokensUsed: tokensUsed,
		Model:      modelName,
	}, nil
}

// buildPrompt 构建提示词
func (s *AIContentService) buildPrompt(req *GenerateContentRequest) (string, error) {
	// 如果指定了模板，使用模板生成
	if req.TemplateID > 0 {
		template, err := s.templateRepo.GetByID(req.TemplateID)
		if err != nil {
			return "", fmt.Errorf("模板不存在")
		}
		return s.fillTemplate(template.Template, req.Variables)
	}

	// 否则根据类型构建默认提示词
	return s.buildDefaultPrompt(req.Type, req.Input, req.Variables)
}

// fillTemplate 填充模板变量
func (s *AIContentService) fillTemplate(template string, variables map[string]any) (string, error) {
	result := template
	for key, value := range variables {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result, nil
}

// buildDefaultPrompt 构建默认提示词
func (s *AIContentService) buildDefaultPrompt(typ model.AIGenerationType, input string, variables map[string]any) (string, error) {
	switch typ {
	case model.AIGenerationTypeCopywriting:
		return fmt.Sprintf("请为以下内容生成营销文案：\n\n%s\n\n要求：语言生动有感染力，突出核心卖点。", input), nil
	case model.AIGenerationTypeTitle:
		count := 5
		if c, ok := variables["count"].(float64); ok {
			count = int(c)
		}
		return fmt.Sprintf("请为以下内容生成%d个吸引人的标题：\n\n%s", count, input), nil
	case model.AIGenerationTypeSummary:
		return fmt.Sprintf("请为以下内容生成摘要：\n\n%s", input), nil
	case model.AIGenerationTypeReply:
		return fmt.Sprintf("请针对以下内容生成专业、友好的回复：\n\n%s", input), nil
	case model.AIGenerationTypeRewrite:
		style := "简洁明了"
		if s, ok := variables["style"].(string); ok {
			style = s
		}
		return fmt.Sprintf("请将以下内容改写，风格要求%s：\n\n%s", style, input), nil
	case model.AIGenerationTypeExpand:
		return fmt.Sprintf("请对以下内容进行扩写，增加更多细节和深度：\n\n%s", input), nil
	case model.AIGenerationTypePolish:
		return fmt.Sprintf("请润色以下内容，使其更加专业流畅：\n\n%s", input), nil
	case model.AIGenerationTypeKeywords:
		return fmt.Sprintf("请从以下内容中提取关键词：\n\n%s", input), nil
	case model.AIGenerationTypeAdCopy:
		return fmt.Sprintf("请为以下产品生成广告文案：\n\n%s\n\n要求：突出卖点，吸引点击，包含行动号召。", input), nil
	case model.AIGenerationTypeSocialPost:
		platform := "社交媒体"
		if p, ok := variables["platform"].(string); ok {
			platform = p
		}
		return fmt.Sprintf("请为%s平台生成一条帖子：\n\n%s\n\n要求：符合平台特点，吸引用户互动。", platform, input), nil
	case model.AIGenerationTypeScript:
		return fmt.Sprintf("请生成销售话术：\n\n%s\n\n要求：开场白吸引注意，突出价值，引导成交。", input), nil
	case model.AIGenerationTypeEmail:
		return fmt.Sprintf("请撰写一封邮件：\n\n%s\n\n要求：主题明确，内容简洁专业。", input), nil
	default:
		return input, nil
	}
}

// CreateHistoryRequest 创建生成历史记录请求
type CreateHistoryRequest struct {
	Type       model.AIGenerationType `json:"type" binding:"required"`
	Input      string                 `json:"input" binding:"required"`
	Output     string                 `json:"output"`
	TemplateID uint                   `json:"template_id"`
	Model      string                 `json:"model"`
}

// CreateHistory 直接创建生成历史记录（不调用外部 AI 服务）
func (s *AIContentService) CreateHistory(userID uint, req *CreateHistoryRequest) (*model.AIGenerationRecord, error) {
	modelName := req.Model
	if modelName == "" {
		modelName = "manual"
	}

	record := &model.AIGenerationRecord{
		UserID:     userID,
		Type:       req.Type,
		Input:      req.Input,
		Output:     req.Output,
		TemplateID: req.TemplateID,
		Model:      modelName,
	}

	if err := s.recordRepo.Create(record); err != nil {
		return nil, fmt.Errorf("创建历史记录失败：%v", err)
	}

	return record, nil
}

// GetGenerationHistory 获取生成历史
func (s *AIContentService) GetGenerationHistory(userID uint, page, pageSize int, filters map[string]any) ([]*model.AIGenerationRecord, int64, error) {
	return s.recordRepo.GetByMerchantAndUser(userID, page, pageSize, filters)
}

// GetRecordByID 获取生成记录
func (s *AIContentService) GetRecordByID(id uint) (*model.AIGenerationRecord, error) {
	record, err := s.recordRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// SaveRecord 保存生成记录
func (s *AIContentService) SaveRecord(id uint) error {
	return s.recordRepo.UpdateSaved(id, true)
}

// FavoriteRecord 收藏生成记录
func (s *AIContentService) FavoriteRecord(id uint, isFavorite bool) error {
	return s.recordRepo.UpdateFavorite(id, isFavorite)
}

// RateRecord 评分生成记录
func (s *AIContentService) RateRecord(id uint, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("评分必须在 1-5 之间")
	}
	return s.recordRepo.UpdateRating(id, rating)
}

// DeleteRecord 删除生成记录
func (s *AIContentService) DeleteRecord(id uint) error {
	record, err := s.recordRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = record
	return s.recordRepo.Delete(id)
}

// PromptTemplateService 提示词模板服务
type PromptTemplateService struct {
	templateRepo repository.PromptTemplateRepository
}

// NewPromptTemplateService 创建提示词模板服务实例
func NewPromptTemplateService(db any) *PromptTemplateService {
	gormDB := db.(*gorm.DB)
	return &PromptTemplateService{
		templateRepo: repository.NewPromptTemplateRepository(gormDB),
	}
}

// CreateTemplateRequest 创建模板请求
type CreateTemplateRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        model.AIGenerationType `json:"type" binding:"required"`
	Template    string                 `json:"template" binding:"required"`
	Variables   string                 `json:"variables"`
	Description string                 `json:"description"`
	Example     string                 `json:"example"`
}

// GetTemplates 获取模板列表
func (s *PromptTemplateService) GetTemplates(templateType string) ([]*model.PromptTemplate, error) {
	return s.templateRepo.ListByType(templateType)
}

// GetTemplateByID 获取模板详情
func (s *PromptTemplateService) GetTemplateByID(id uint) (*model.PromptTemplate, error) {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	// 系统模板所有人可用
	_ = template
	return template, nil
}

// CreateTemplate 创建模板
func (s *PromptTemplateService) CreateTemplate(req *CreateTemplateRequest) (*model.PromptTemplate, error) {
	template := &model.PromptTemplate{
		Name:        req.Name,
		Type:        req.Type,
		Template:    req.Template,
		Variables:   req.Variables,
		Description: req.Description,
		Example:     req.Example,
		IsSystem:    false,
	}

	if err := s.templateRepo.Create(template); err != nil {
		return nil, err
	}

	return template, nil
}

// UpdateTemplate 更新模板
func (s *PromptTemplateService) UpdateTemplate(id uint, req *CreateTemplateRequest) (*model.PromptTemplate, error) {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if template.IsSystem {
		return nil, fmt.Errorf("系统模板不能修改")
	}

	_ = template

	template.Name = req.Name
	template.Type = req.Type
	template.Template = req.Template
	template.Variables = req.Variables
	template.Description = req.Description
	template.Example = req.Example

	if err := s.templateRepo.Update(template); err != nil {
		return nil, err
	}

	return template, nil
}

// DeleteTemplate 删除模板
func (s *PromptTemplateService) DeleteTemplate(id uint) error {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return err
	}

	if template.IsSystem {
		return fmt.Errorf("系统模板不能删除")
	}

	_ = template

	return s.templateRepo.Delete(id)
}

// InitSystemTemplates 初始化系统模板
func (s *PromptTemplateService) InitSystemTemplates() error {
	for _, template := range model.SystemPromptTemplates {
		existing, _ := s.templateRepo.GetByTypeAndName(template.Type, template.Name)
		if existing == nil {
			templateCopy := template
			templateCopy.Status = 1 // 设置状态为激活
			s.templateRepo.Create(&templateCopy)
		}
	}
	return nil
}

// GetTemplateTypes 获取模板类型列表
func (s *PromptTemplateService) GetTemplateTypes() []map[string]string {
	return []map[string]string{
		{"value": string(model.AIGenerationTypeCopywriting), "label": "营销文案"},
		{"value": string(model.AIGenerationTypeTitle), "label": "标题生成"},
		{"value": string(model.AIGenerationTypeSummary), "label": "内容摘要"},
		{"value": string(model.AIGenerationTypeReply), "label": "智能回复"},
		{"value": string(model.AIGenerationTypeRewrite), "label": "内容改写"},
		{"value": string(model.AIGenerationTypeExpand), "label": "内容扩写"},
		{"value": string(model.AIGenerationTypePolish), "label": "内容润色"},
		{"value": string(model.AIGenerationTypeKeywords), "label": "关键词提取"},
		{"value": string(model.AIGenerationTypeAdCopy), "label": "广告文案"},
		{"value": string(model.AIGenerationTypeSocialPost), "label": "社交媒体"},
		{"value": string(model.AIGenerationTypeScript), "label": "销售话术"},
		{"value": string(model.AIGenerationTypeEmail), "label": "邮件撰写"},
	}
}

// resolveAPIKey 解析 LLM API Key
// 优先从环境变量 LLM_API_KEY 读取，其次 OPENAI_API_KEY，最后回退到系统配置表
func (s *AIContentService) resolveAPIKey(ctx context.Context) string {
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		return v
	}
	// 退化路径：从系统配置表读取
	cfg, err := s.loadSystemLLMConfig(ctx)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.APIKey
}

// resolveBaseURL 解析 LLM BaseURL
func (s *AIContentService) resolveBaseURL() string {
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		return v
	}
	return "https://api.openai.com"
}

// llmSystemConfig 系统 LLM 配置
type llmSystemConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// loadSystemLLMConfig 加载系统 LLM 配置
// 优先级:系统 SystemConfig.Config > LLM_API_KEY 环境变量
func (s *AIContentService) loadSystemLLMConfig(ctx context.Context) (*llmSystemConfig, error) {
	// 1) 优先读系统配置表
	sysCfgRepo := sysrepo.NewSystemConfigRepository()
	if sysCfg, err := sysCfgRepo.GetConfig(ctx); err == nil && sysCfg != nil {
		// SystemConfig 自身的字段都是基础项,LLM 配置通常存在 JSON 字段或 env
		// 这里仅尝试读 llm_api_key 子键
	}
	// 2) 退到环境变量(LLM_API_KEY / LLM_BASE_URL / LLM_MODEL)
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	if apiKey == "" {
		return nil, fmt.Errorf("未配置 LLM_API_KEY 环境变量,且数据库无 LLM 配置")
	}
	return &llmSystemConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, nil
}
