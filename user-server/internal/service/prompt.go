package service

import (
	"context"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// PromptService Prompt 版本管理 + A/B 实验业务层
type PromptService struct {
	repo *repository.PromptRepo
}

// NewPromptService 构造
func NewPromptService() *PromptService {
	return &PromptService{repo: repository.NewPromptRepo()}
}

// NewPromptServiceWithRepo 注入 repo（测试用）
func NewPromptServiceWithRepo(repo *repository.PromptRepo) *PromptService {
	return &PromptService{repo: repo}
}

// ListVersions 获取某个 SOP Node / Prompt ID 的所有历史版本
func (s *PromptService) ListVersions(ctx context.Context, idStr string, sopID uint, sopNodeID string, status string) ([]model.PromptCandidate, error) {
	return s.repo.ListVersions(ctx, idStr, sopID, sopNodeID, status)
}

// PublishRequest 发布新版本请求结构
type PublishRequest struct {
	SystemPrompt       string
	UserPromptTemplate string
	SOPNodeID          string
	SOPID              uint
	ImprovementNotes   string
	Variables          string
	ParentID           uint
}

// Publish 发布新版本（从 draft → active，自动把同 sop_node_id 的旧版本降为 retired）
func (s *PromptService) Publish(ctx context.Context, req PublishRequest) (*model.PromptCandidate, error) {
	// 把当前 active 的同 node 版本降为 retired
	if req.SOPNodeID != "" {
		if err := s.repo.RetireActiveBySOPNode(ctx, req.SOPNodeID); err != nil {
			return nil, err
		}
	}

	newVersion := &model.PromptCandidate{
		SOPNodeID:          req.SOPNodeID,
		SOPID:              req.SOPID,
		SystemPrompt:       req.SystemPrompt,
		UserPromptTemplate: req.UserPromptTemplate,
		ImprovementNotes:   req.ImprovementNotes,
		Status:             model.PromptCandidateStatusActive,
		ParentID:           req.ParentID,
	}
	if err := s.repo.CreateCandidate(ctx, newVersion); err != nil {
		return nil, err
	}
	return newVersion, nil
}

// ListABTests 获取所有 Prompt A/B 实验列表
func (s *PromptService) ListABTests(ctx context.Context, status string) ([]model.PromptABTest, error) {
	return s.repo.ListABTests(ctx, status)
}
