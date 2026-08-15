package service


import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	dbUtil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"
)

// AgentKBBindingService 智能体知识库绑定服务
type AgentKBBindingService struct {
	bindingRepo *repository.AgentKBBindingRepository
	kbRepo      *repository.KnowledgeBaseRepository
	db          *gorm.DB
}

// NewAgentKBBindingServiceDefault 使用全局 DB 创建绑定服务（controller 层入口，避免 controller 持有 gorm.DB）。
func NewAgentKBBindingServiceDefault() *AgentKBBindingService {
	return NewAgentKBBindingService(dbUtil.GetDB())
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
	if s.kbRepo != nil {
		kb, err := s.kbRepo.GetByID(ctx, kbID)
		if err != nil {
			return fmt.Errorf("知识库不存在: %w", err)
		}
		if kb == nil {
			return errors.New("知识库不存在")
		}
	}
	if err := s.bindingRepo.DeleteByAgentAndKB(ctx, agentID, kbID); err != nil {
		return fmt.Errorf("清除旧绑定失败: %w", err)
	}
	binding := &model.AgentKBBinding{
		AgentID:  agentID,
		KBID:     kbID,
		KBType:   model.KnowledgeBaseTypeFAQ, 
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

// ReplaceByAgent 全量替换某智能体的知识库挂载（编辑页保存场景）
//
// 行为:
//   - 先删除该智能体全部已有绑定, 再按 kbIDs 重新绑定 (replace 语义)
//   - 任一 kbID 对应的知识库不存在则整体失败并回滚
//   - kbIDs 为空: 等价于清空该智能体的所有挂载
func (s *AgentKBBindingService) ReplaceByAgent(ctx context.Context, agentID uint, kbIDs []uint) error {
	if s.bindingRepo == nil {
		return errors.New("binding repo not initialized")
	}
	if agentID == 0 {
		return errors.New("agent_id 不能为空")
	}
	if s.db != nil {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			tmpBindingRepo := repository.NewAgentKBBindingRepository(tx)
			tmpKBRepo := repository.NewKnowledgeBaseRepository(tx)
			if err := tmpBindingRepo.DeleteByAgent(ctx, agentID); err != nil {
				return fmt.Errorf("清除旧绑定失败: %w", err)
			}
			for _, kbID := range kbIDs {
				if kbID == 0 {
					continue
				}
				kb, err := tmpKBRepo.GetByID(ctx, kbID)
				if err != nil {
					return fmt.Errorf("知识库不存在: %w", err)
				}
				if kb == nil {
					return fmt.Errorf("知识库 %d 不存在", kbID)
				}
				b := &model.AgentKBBinding{
					AgentID:  agentID,
					KBID:     kbID,
					KBType:   kb.Type,
					Role:     model.AgentKBBindingRolePrimary,
					Priority: 0,
					Enabled:  boolPtr(true),
				}
				if err := tmpBindingRepo.Create(ctx, b); err != nil {
					return fmt.Errorf("创建绑定失败: %w", err)
				}
			}
			return nil
		})
	}
	if err := s.bindingRepo.DeleteByAgent(ctx, agentID); err != nil {
		return err
	}
	for _, kbID := range kbIDs {
		if kbID == 0 {
			continue
		}
		if err := s.Bind(ctx, agentID, kbID, 0); err != nil {
			return err
		}
	}
	return nil
}

// BatchBindItem 批量绑定参数项
type BatchBindItem struct {
	AgentID uint `json:"agent_id"`
	KBID    uint `json:"knowledge_base_id"`
	Priority int `json:"priority"`
}

// BatchBind 批量绑定 (事务; 全部成功或全部失败)
//
// 行为:
//   - 同一 (agent_id, kb_id) 重复: 覆盖
//   - 任一条失败: 整体回滚
//   - items 为空: 直接返回 nil (不视为错误)
func (s *AgentKBBindingService) BatchBind(ctx context.Context, items []BatchBindItem) error {
	if len(items) == 0 {
		return nil
	}
	if s.bindingRepo == nil {
		return errors.New("binding repo not initialized")
	}
	for i, it := range items {
		if it.AgentID == 0 {
			return fmt.Errorf("items[%d]: agent_id 不能为空", i)
		}
		if it.KBID == 0 {
			return fmt.Errorf("items[%d]: knowledge_base_id 不能为空", i)
		}
	}
	if s.db != nil {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			tmpBindingRepo := repository.NewAgentKBBindingRepository(tx)
			tmpKBRepo := repository.NewKnowledgeBaseRepository(tx)
			for _, it := range items {
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
	for _, it := range items {
		if err := s.Bind(ctx, it.AgentID, it.KBID, it.Priority); err != nil {
			return err
		}
	}
	return nil
}

// boolPtr 构造 *bool
func boolPtr(b bool) *bool { return &b }

