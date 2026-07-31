package service

// knowledge_base.go 知识库业务服务层
//
// 五层架构归属: L4 业务编排层
// 设计依据: 2026-07-31 强 1对1 改造
//
// 业务规则:
//   - owner_type=private 时 owner_agent_id 必填, 校验失败拒绝创建
//   - owner_type=shared  时 owner_agent_id 必为空, 校验失败拒绝创建
//   - type 必须为 faq/rag/sop 三选一
//   - 删除知识库时, 应同步删除所有 agent_kb_bindings 引用 (业务级联)
//   - agent 删除时, 业务级联删除其作为 owner 的 private 知识库

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// KnowledgeBaseService 知识库业务服务
type KnowledgeBaseService struct {
	repo        *repository.KnowledgeBaseRepository
	bindingRepo *repository.AgentKBBindingRepository
	db          *gorm.DB
}

// IsValidKBType 校验知识库类型字段是否合法 (faq / rag / sop)
//
// 工具方法: 不依赖 repo/db, 业务层可直接调用. 抽离出 Repository 以避免
// 架构检查 (check-architecture.sh) 对"无 ctx 工具方法"的误报.
func IsValidKBType(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	return t == model.KnowledgeBaseTypeFAQ ||
		t == model.KnowledgeBaseTypeRAG ||
		t == model.KnowledgeBaseTypeSOP
}

// NewKnowledgeBaseService 创建知识库服务
func NewKnowledgeBaseService(db *gorm.DB) *KnowledgeBaseService {
	return &KnowledgeBaseService{
		repo:        repository.NewKnowledgeBaseRepository(db),
		bindingRepo: repository.NewAgentKBBindingRepository(db),
		db:          db,
	}
}

// SetRepositories 注入 repository (用于测试)
func (s *KnowledgeBaseService) SetRepositories(kbRepo *repository.KnowledgeBaseRepository, bindRepo *repository.AgentKBBindingRepository) {
	if kbRepo != nil {
		s.repo = kbRepo
	}
	if bindRepo != nil {
		s.bindingRepo = bindRepo
	}
}

// CreateKB 创建知识库
//
// 业务规则:
//   - name 必填
//   - type 必填且为 faq/rag/sop
//   - owner_type=private 时 owner_agent_id 必填
//   - owner_type=shared  时 owner_agent_id 必为空
func (s *KnowledgeBaseService) CreateKB(ctx context.Context, kb *model.KnowledgeBase) error {
	if s.repo == nil {
		return errors.New("repo not initialized")
	}
	if kb == nil {
		return errors.New("kb is nil")
	}
	kb.Name = strings.TrimSpace(kb.Name)
	if kb.Name == "" {
		return errors.New("name 不能为空")
	}
	kb.Type = strings.ToLower(strings.TrimSpace(kb.Type))
	if !IsValidKBType(kb.Type) {
		return fmt.Errorf("type 非法: %s (faq/rag/sop)", kb.Type)
	}
	kb.OwnerType = strings.ToLower(strings.TrimSpace(kb.OwnerType))
	if kb.OwnerType == "" {
		kb.OwnerType = model.KnowledgeBaseOwnerPrivate
	}
	switch kb.OwnerType {
	case model.KnowledgeBaseOwnerPrivate:
		if kb.OwnerAgentID == nil || *kb.OwnerAgentID == 0 {
			return errors.New("owner_type=private 时 owner_agent_id 必填")
		}
	case model.KnowledgeBaseOwnerShared:
		if kb.OwnerAgentID != nil && *kb.OwnerAgentID != 0 {
			return errors.New("owner_type=shared 时 owner_agent_id 必为空")
		}
		// shared 时显式置空, 避免入库残留
		kb.OwnerAgentID = nil
	default:
		return fmt.Errorf("owner_type 非法: %s (private/shared)", kb.OwnerType)
	}
	if kb.Enabled == nil {
		t := true
		kb.Enabled = &t
	}
	return s.repo.Create(ctx, kb)
}

// GetKB 按 ID 查询
func (s *KnowledgeBaseService) GetKB(ctx context.Context, id uint) (*model.KnowledgeBase, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.GetByID(ctx, id)
}

// ListKBs 列表查询
func (s *KnowledgeBaseService) ListKBs(ctx context.Context, kbType, ownerType string, agentID uint, keyword string) ([]*model.KnowledgeBase, int64, error) {
	if s.repo == nil {
		return nil, 0, nil
	}
	filter := repository.KBListFilter{
		Type:      kbType,
		OwnerType: ownerType,
	}
	if agentID > 0 {
		aid := agentID
		filter.OwnerAgentID = &aid
	}
	kbs, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	// 转为指针切片, 保持服务层 API 稳定
	ptrs := make([]*model.KnowledgeBase, len(kbs))
	for i := range kbs {
		ptrs[i] = &kbs[i]
	}
	// 服务层再做一次 keyword 过滤 (List 层 keyword 暂未支持)
	if keyword != "" {
		like := strings.ToLower(strings.TrimSpace(keyword))
		filtered := make([]*model.KnowledgeBase, 0, len(ptrs))
		for _, kb := range ptrs {
			if strings.Contains(strings.ToLower(kb.Name), like) || strings.Contains(strings.ToLower(kb.Description), like) {
				filtered = append(filtered, kb)
			}
		}
		return filtered, total, nil
	}
	return ptrs, total, nil
}

// ListByType 按类型查询
func (s *KnowledgeBaseService) ListByType(ctx context.Context, kbType string) ([]*model.KnowledgeBase, error) {
	if s.repo == nil {
		return nil, nil
	}
	kbs, err := s.repo.ListByType(ctx, kbType, 0)
	if err != nil {
		return nil, err
	}
	out := make([]*model.KnowledgeBase, len(kbs))
	for i := range kbs {
		out[i] = &kbs[i]
	}
	return out, nil
}

// ListByAgent 查某智能体可用的知识库 (shared + agent 私有)
func (s *KnowledgeBaseService) ListByAgent(ctx context.Context, agentID uint) ([]*model.KnowledgeBase, error) {
	if s.repo == nil {
		return nil, nil
	}
	kbs, err := s.repo.ListByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.KnowledgeBase, len(kbs))
	for i := range kbs {
		out[i] = &kbs[i]
	}
	return out, nil
}

// UpdateKB 更新知识库
func (s *KnowledgeBaseService) UpdateKB(ctx context.Context, id uint, kb *model.KnowledgeBase) error {
	if s.repo == nil {
		return errors.New("repo not initialized")
	}
	if id == 0 {
		return errors.New("id 不能为空")
	}
	if kb.Name == "" {
		return errors.New("name 不能为空")
	}
	// type 与 owner_type 校验
	if kb.Type != "" && !IsValidKBType(kb.Type) {
		return fmt.Errorf("type 非法: %s", kb.Type)
	}
	if kb.OwnerType != "" {
		ot := strings.ToLower(strings.TrimSpace(kb.OwnerType))
		if ot == model.KnowledgeBaseOwnerPrivate {
			if kb.OwnerAgentID == nil || *kb.OwnerAgentID == 0 {
				return errors.New("owner_type=private 时 owner_agent_id 必填")
			}
		}
		if ot == model.KnowledgeBaseOwnerShared {
			if kb.OwnerAgentID != nil && *kb.OwnerAgentID != 0 {
				return errors.New("owner_type=shared 时 owner_agent_id 必为空")
			}
			kb.OwnerAgentID = nil
		}
	}
	return s.repo.Update(ctx, id, kb)
}

// DeleteKB 删除知识库 (业务级联: 同步删除所有 agent_kb_bindings)
func (s *KnowledgeBaseService) DeleteKB(ctx context.Context, id uint) error {
	if s.repo == nil {
		return errors.New("repo not initialized")
	}
	// 先删除所有绑定
	if err := s.bindingRepo.DeleteByKB(ctx, id); err != nil {
		return fmt.Errorf("删除知识库绑定失败: %w", err)
	}
	return s.repo.Delete(ctx, id)
}

// BindToAgent 入口 (供控制器调用), 内部委托 AgentKBBindingService
func (s *KnowledgeBaseService) BindToAgent(ctx context.Context, kbID, agentID uint) error {
	bindingSvc := NewAgentKBBindingServiceWithRepos(s.repo, s.bindingRepo, s.db)
	return bindingSvc.Bind(ctx, agentID, kbID, 0)
}

// UnbindFromAgent 从智能体解绑知识库
func (s *KnowledgeBaseService) UnbindFromAgent(ctx context.Context, kbID, agentID uint) error {
	if s.bindingRepo == nil {
		return nil
	}
	return s.bindingRepo.DeleteByAgentAndKB(ctx, agentID, kbID)
}
