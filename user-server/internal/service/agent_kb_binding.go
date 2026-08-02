package service

// agent_kb_binding.go 智能体知识库绑定业务服务层
//
// 五层架构归属: L4 业务编排层
// 设计依据: 强 1对1 改造
//
// 业务规则:
//   - (agent_id, knowledge_base_id) 唯一; 重复 bind 自动覆盖 (先删后建)
//   - 同一智能体可绑多个不同类型 (faq/rag/sop) 的知识库
//   - 同一类型可绑多个知识库, 业务层按 priority DESC 排序
//   - BatchBind 接受一组 (agent_id, kb_id) 对, 失败时回滚 (事务)

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// AgentKBBindingService 智能体知识库绑定服务
type AgentKBBindingService struct {
	bindingRepo *repository.AgentKBBindingRepository
	kbRepo      *repository.KnowledgeBaseRepository
	db          *gorm.DB
}

// NewAgentKBBindingService 创建绑定服务
func NewAgentKBBindingService(db *gorm.DB) *AgentKBBindingService {
	return &AgentKBBindingService{
		bindingRepo: repository.NewAgentKBBindingRepository(db),
		kbRepo:      repository.NewKnowledgeBaseRepository(db),
		db:          db,
	}
}

// NewAgentKBBindingServiceWithRepos 通过 repo 注入 (供测试和知识库 service 内部调用)
func NewAgentKBBindingServiceWithRepos(
	kbRepo *repository.KnowledgeBaseRepository,
	bindRepo *repository.AgentKBBindingRepository,
	db *gorm.DB,
) *AgentKBBindingService {
	return &AgentKBBindingService{
		bindingRepo: bindRepo,
		kbRepo:      kbRepo,
		db:          db,
	}
}

// SetRepositories 注入 repository (用于测试)
func (s *AgentKBBindingService) SetRepositories(
	kbRepo *repository.KnowledgeBaseRepository,
	bindRepo *repository.AgentKBBindingRepository,
) {
	if kbRepo != nil {
		s.kbRepo = kbRepo
	}
	if bindRepo != nil {
		s.bindingRepo = bindRepo
	}
}

// Bind 单个绑定 (重复自动覆盖)
//
// 业务规则:
//   - agentID 必填
//   - kbID    必填且必须存在
//   - 同一 (agent_id, kb_id) 已存在则覆盖 priority / enabled
//   - 默认 enabled = true
func (s *AgentKBBindingService) Bind(ctx context.Context, agentID, kbID uint, priority int) error {
	if s.bindingRepo == nil {
		return errors.New("binding repo not initialized")
	}
	if agentID == 0 {
		return errors.New("agent_id 不能为空")
	}
	if kbID == 0 {
		return errors.New("knowledge_base_id 不能为空")
	}
	// 校验知识库存在
	if s.kbRepo != nil {
		kb, err := s.kbRepo.GetByID(ctx, kbID)
		if err != nil {
			return fmt.Errorf("知识库不存在: %w", err)
		}
		if kb == nil {
			return errors.New("知识库不存在")
		}
	}
	// 重复自动覆盖: 先删后建
	if err := s.bindingRepo.DeleteByAgentAndKB(ctx, agentID, kbID); err != nil {
		return fmt.Errorf("清除旧绑定失败: %w", err)
	}
	binding := &model.AgentKBBinding{
		AgentID:  agentID,
		KBID:     kbID,
		KBType:   model.KnowledgeBaseTypeFAQ, // 默认; CreateBinding 路径会显式覆盖
		Role:     model.AgentKBBindingRolePrimary,
		Priority: priority,
		Enabled:  boolPtr(true),
	}
	return s.bindingRepo.Create(ctx, binding)
}

// Unbind 单个解绑
func (s *AgentKBBindingService) Unbind(ctx context.Context, agentID, kbID uint) error {
	if s.bindingRepo == nil {
		return errors.New("binding repo not initialized")
	}
	return s.bindingRepo.DeleteByAgentAndKB(ctx, agentID, kbID)
}

// ListByAgent 查某智能体的所有绑定
func (s *AgentKBBindingService) ListByAgent(ctx context.Context, agentID uint) ([]*model.AgentKBBinding, error) {
	if s.bindingRepo == nil {
		return nil, nil
	}
	bindings, err := s.bindingRepo.ListByAgent(ctx, agentID, "")
	if err != nil {
		return nil, err
	}
	out := make([]*model.AgentKBBinding, len(bindings))
	for i := range bindings {
		b := bindings[i]
		out[i] = &b
	}
	return out, nil
}

// ListByKB 查某知识库被哪些智能体绑定
func (s *AgentKBBindingService) ListByKB(ctx context.Context, kbID uint) ([]*model.AgentKBBinding, error) {
	if s.bindingRepo == nil {
		return nil, nil
	}
	bindings, err := s.bindingRepo.ListByKB(ctx, kbID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AgentKBBinding, len(bindings))
	for i := range bindings {
		b := bindings[i]
		out[i] = &b
	}
	return out, nil
}

// BatchBindItem 批量绑定参数项
type BatchBindItem struct {
	AgentID uint `json:"agent_id"`
	KBID    uint `json:"knowledge_base_id"`
	// Priority 可选, 默认 0
	Priority int `json:"priority"`
}

// BatchBind 批量绑定 (事务; 全部成功或全部失败)
//
// 行为:
//   - 同一 (agent_id, kb_id) 重复: 覆盖
//   - 任一条失败: 整体回滚
//   - items 为空: 直接返回 nil (不视为错误)
func (s *AgentKBBindingService) BatchBind(ctx context.Context, items []BatchBindItem) error {
	// 空 items 不依赖 repo, 优先返回 nil (避免空操作走 nil repo 错误)
	if len(items) == 0 {
		return nil
	}
	if s.bindingRepo == nil {
		return errors.New("binding repo not initialized")
	}
	// 校验
	for i, it := range items {
		if it.AgentID == 0 {
			return fmt.Errorf("items[%d]: agent_id 不能为空", i)
		}
		if it.KBID == 0 {
			return fmt.Errorf("items[%d]: knowledge_base_id 不能为空", i)
		}
	}
	// 走事务
	if s.db != nil {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			tmpBindingRepo := repository.NewAgentKBBindingRepository(tx)
			tmpKBRepo := repository.NewKnowledgeBaseRepository(tx)
			for _, it := range items {
				// 校验 KB 存在
				kb, err := tmpKBRepo.GetByID(ctx, it.KBID)
				if err != nil {
					return fmt.Errorf("items[agent=%d kb=%d]: 知识库不存在: %w", it.AgentID, it.KBID, err)
				}
				if kb == nil {
					return fmt.Errorf("items[agent=%d kb=%d]: 知识库不存在", it.AgentID, it.KBID)
				}
				if err := tmpBindingRepo.DeleteByAgentAndKB(ctx, it.AgentID, it.KBID); err != nil {
					return fmt.Errorf("items[agent=%d kb=%d]: 清除旧绑定失败: %w", it.AgentID, it.KBID, err)
				}
				// 取 KB 类型, 用于 KBType 字段冗余
				kbType := kb.Type
				b := &model.AgentKBBinding{
					AgentID:  it.AgentID,
					KBID:     it.KBID,
					KBType:   kbType,
					Role:     model.AgentKBBindingRolePrimary,
					Priority: it.Priority,
					Enabled:  boolPtr(true),
				}
				if err := tmpBindingRepo.Create(ctx, b); err != nil {
					return fmt.Errorf("items[agent=%d kb=%d]: 创建绑定失败: %w", it.AgentID, it.KBID, err)
				}
			}
			return nil
		})
	}
	// 无 db (测试场景): 直接执行
	for _, it := range items {
		if err := s.Bind(ctx, it.AgentID, it.KBID, it.Priority); err != nil {
			return err
		}
	}
	return nil
}

// boolPtr 构造 *bool
func boolPtr(b bool) *bool { return &b }
