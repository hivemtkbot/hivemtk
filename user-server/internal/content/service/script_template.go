package service

import (
	"encoding/json"
	"marketing/internal/content/model"
	"marketing/internal/content/repository"
	"strings"
)

// ScriptTemplateService 话术模板服务
type ScriptTemplateService struct {
	templateRepo  *repository.ScriptTemplateRepository
	categoryRepo  *repository.ScriptCategoryRepository
	recommendRepo *repository.ScriptRecommendRepository
}

// NewScriptTemplateService 创建话术模板服务实例
func NewScriptTemplateService() *ScriptTemplateService {
	return &ScriptTemplateService{
		templateRepo:  repository.NewScriptTemplateRepository(),
		categoryRepo:  repository.NewScriptCategoryRepository(),
		recommendRepo: repository.NewScriptRecommendRepository(),
	}
}

// CreateScriptTemplateRequest 创建话术模板请求
type CreateScriptTemplateRequest struct {
	Category  string   `json:"category" binding:"required"`
	Title     string   `json:"title" binding:"required"`
	Content   string   `json:"content" binding:"required"`
	Variables []string `json:"variables"`
	Tags      string   `json:"tags"`
	IsPublic  bool     `json:"is_public"`
}

// CreateTemplate 创建话术模板
func (s *ScriptTemplateService) CreateTemplate(createdBy uint, req *CreateScriptTemplateRequest) (*model.ScriptTemplate, error) {
	variables, _ := json.Marshal(req.Variables)

	template := &model.ScriptTemplate{
		Category:  req.Category,
		Title:     req.Title,
		Content:   req.Content,
		Variables: string(variables),
		Tags:      req.Tags,
		IsPublic:  req.IsPublic,
		CreatedBy: createdBy,
	}

	if err := s.templateRepo.Create(template); err != nil {
		return nil, err
	}

	return template, nil
}

// GetTemplateList 获取话术模板列表
func (s *ScriptTemplateService) GetTemplateList(category string, page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	return s.templateRepo.GetAll(category, page, pageSize)
}

// GetTemplateByID 获取话术模板详情
func (s *ScriptTemplateService) GetTemplateByID(id uint) (*model.ScriptTemplate, error) {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return template, nil
}

// UpdateTemplateRequest 更新话术模板请求
type UpdateTemplateRequest struct {
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Variables []string `json:"variables"`
	Tags      string   `json:"tags"`
	IsPublic  bool     `json:"is_public"`
}

// UpdateTemplate 更新话术模板
func (s *ScriptTemplateService) UpdateTemplate(id uint, req *UpdateTemplateRequest) (*model.ScriptTemplate, error) {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Category != "" {
		template.Category = req.Category
	}
	if req.Title != "" {
		template.Title = req.Title
	}
	if req.Content != "" {
		template.Content = req.Content
	}
	if req.Variables != nil {
		variables, _ := json.Marshal(req.Variables)
		template.Variables = string(variables)
	}
	template.Tags = req.Tags
	template.IsPublic = req.IsPublic

	if err := s.templateRepo.Update(template); err != nil {
		return nil, err
	}

	return template, nil
}

// DeleteTemplate 删除话术模板
func (s *ScriptTemplateService) DeleteTemplate(id uint) error {
	template, err := s.templateRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = template

	return s.templateRepo.Delete(id)
}

// GetCategories 获取话术分类
func (s *ScriptTemplateService) GetCategories() ([]*model.ScriptCategory, error) {
	return s.categoryRepo.GetAll()
}

// CreateCategory 创建话术分类
func (s *ScriptTemplateService) CreateCategory(name string, parentID uint) (*model.ScriptCategory, error) {
	category := &model.ScriptCategory{
		Name:     name,
		ParentID: parentID,
	}

	if err := s.categoryRepo.Create(category); err != nil {
		return nil, err
	}

	return category, nil
}

// SearchTemplates 搜索话术
func (s *ScriptTemplateService) SearchTemplates(keyword string, page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	return s.templateRepo.SearchTemplates(keyword, page, pageSize)
}

// UseTemplate 使用话术（增加使用次数）
func (s *ScriptTemplateService) UseTemplate(id uint) error {
	return s.templateRepo.IncrementUsage(id)
}

// GetPublicTemplates 获取公开话术模板
func (s *ScriptTemplateService) GetPublicTemplates(page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	return s.templateRepo.GetPublicTemplates(page, pageSize)
}

// RecommendScript 推荐话术
func (s *ScriptTemplateService) RecommendScript(sessionID, message string) ([]*model.ScriptTemplate, error) {
	// 简单关键词匹配推荐
	templates, _, _ := s.templateRepo.SearchTemplates("", 1, 100)

	// 计算相关性得分
	type scoredTemplate struct {
		template *model.ScriptTemplate
		score    float64
	}

	var scored []scoredTemplate
	for _, t := range templates {
		score := s.calculateRelevance(message, t)
		if score > 0.3 {
			scored = append(scored, scoredTemplate{template: t, score: score})
		}
	}

	// 按得分排序
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[i].score < scored[j].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// 返回前 5 个推荐
	var result []*model.ScriptTemplate
	for i := 0; i < len(scored) && i < 5; i++ {
		result = append(result, scored[i].template)
	}

	return result, nil
}

// calculateRelevance 计算相关性得分
func (s *ScriptTemplateService) calculateRelevance(message string, template *model.ScriptTemplate) float64 {
	message = strings.ToLower(message)
	content := strings.ToLower(template.Content)
	title := strings.ToLower(template.Title)

	// 简单匹配：消息中包含话术关键词
	matchCount := 0
	keywords := strings.Fields(content)
	for _, keyword := range keywords {
		if len(keyword) > 2 && strings.Contains(message, keyword) {
			matchCount++
		}
	}

	if len(keywords) == 0 {
		return 0
	}

	score := float64(matchCount) / float64(len(keywords))

	// 标题匹配加权
	if strings.Contains(message, title) {
		score += 0.3
	}

	return score
}
