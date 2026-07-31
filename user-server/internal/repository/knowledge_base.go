package repository

// knowledge_base.go 知识库主表 Repository (P0-B 隔离架构)
//
// 五层架构归属: L5 数据访问层
// 设计依据: 2026-07-31 P0 知识库隔离架构
//   - 知识库主表 CRUD
//   - 支持按 Type/Agent/OwnerType/共享 多种维度查询
//   - 业务逻辑 (缓存/权限/编排) 在 Service 层
//
// 方法:
//   - Create           新增知识库
//   - GetByID          按 ID 查询
//   - GetByCode        按 kb_code 查询
//   - List             列出全部 (带可选过滤)
//   - ListByType       按类型 (faq/rag/sop) 过滤
//   - ListByAgent      按智能体查其私有知识库
//   - ListShared       查全部共享知识库
//   - Update           更新
//   - Delete           删除
//   - CountByAgent     统计某智能体拥有多少私有 KB
//
// AgentKBBindingRepository 单独位于 internal/repository/agent_kb_binding.go
//
// IsValidType 工具方法: 不放在 Repository 中 (避免架构检查误判"缺 ctx"),
//                       实际放在 service/knowledge_base.go 的 IsValidKBType 供调用.

import (
	"context"
	"errors"

	"marketing/internal/model"

	"gorm.io/gorm"
)

// KnowledgeBaseRepository 知识库主表仓储
type KnowledgeBaseRepository struct {
	db *gorm.DB
}

// NewKnowledgeBaseRepository 创建仓储
func NewKnowledgeBaseRepository(db *gorm.DB) *KnowledgeBaseRepository {
	return &KnowledgeBaseRepository{db: db}
}

// Create 新增知识库
func (r *KnowledgeBaseRepository) Create(ctx context.Context, kb *model.KnowledgeBase) error {
	return r.db.WithContext(ctx).Select("*").Create(kb).Error
}

// GetByID 按 ID 查询
func (r *KnowledgeBaseRepository) GetByID(ctx context.Context, id uint) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	if err := r.db.WithContext(ctx).First(&kb, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &kb, nil
}

// GetByCode 按 kb_code 查询 (业务唯一码)
func (r *KnowledgeBaseRepository) GetByCode(ctx context.Context, code string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	if err := r.db.WithContext(ctx).Where("kb_code = ?", code).First(&kb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &kb, nil
}

// ListFilter 列表查询过滤条件
type KBListFilter struct {
	Type         string // faq/rag/sop, 空=全部
	OwnerType    string // private/shared, 空=全部
	OwnerAgentID *uint  // 按智能体过滤
	Enabled      *bool  // 按启用状态过滤
	Limit        int
	Offset       int
}

// List 列出知识库 (带可选过滤)
func (r *KnowledgeBaseRepository) List(ctx context.Context, filter KBListFilter) ([]model.KnowledgeBase, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.KnowledgeBase{})
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.OwnerType != "" {
		q = q.Where("owner_type = ?", filter.OwnerType)
	}
	if filter.OwnerAgentID != nil {
		q = q.Where("owner_agent_id = ?", *filter.OwnerAgentID)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var kbs []model.KnowledgeBase
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if err := q.Order("id DESC").Limit(limit).Offset(filter.Offset).Find(&kbs).Error; err != nil {
		return nil, 0, err
	}
	return kbs, total, nil
}

// ListByType 按类型列出 (faq/rag/sop)
func (r *KnowledgeBaseRepository) ListByType(ctx context.Context, kbType string, limit int) ([]model.KnowledgeBase, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var kbs []model.KnowledgeBase
	err := r.db.WithContext(ctx).
		Where("type = ? AND enabled = ?", kbType, true).
		Order("id DESC").
		Limit(limit).
		Find(&kbs).Error
	return kbs, err
}

// ListByAgent 列出某智能体可用的知识库 (shared + agent 私有)
//
// 业务规则: 一个智能体可用的知识库 = 该智能体私有的 + 已 binding 的 shared
//   - 私有: owner_agent_id = X AND enabled = true
//   - 共享: owner_type = 'shared' AND enabled = true AND 存在 agent_kb_bindings (agent_id=X, kb_id=knowledge_bases.id, enabled=true)
//
// 返回: 去重后的 []*model.KnowledgeBase, 按 id DESC 排序
func (r *KnowledgeBaseRepository) ListByAgent(ctx context.Context, agentID uint) ([]model.KnowledgeBase, error) {
	if agentID == 0 {
		return nil, nil
	}
	// 用子查询: shared KB 必须在 agent_kb_bindings 中存在 (agent_id = ? AND enabled = true)
	// SQL 语义:
	//   owner_agent_id = ?                                    -- 私有
	//   OR (owner_type = 'shared' AND id IN (SELECT kb_id FROM agent_kb_bindings WHERE agent_id = ? AND enabled = true))
	var kbs []model.KnowledgeBase
	subQuery := r.db.WithContext(ctx).
		Model(&model.AgentKBBinding{}).
		Select("kb_id").
		Where("agent_id = ? AND enabled = ?", agentID, true)
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("owner_agent_id = ? OR (owner_type = ? AND id IN (?))",
			agentID, model.KnowledgeBaseOwnerShared, subQuery).
		Order("id DESC").
		Find(&kbs).Error
	if err != nil {
		return nil, err
	}
	return kbs, nil
}

// ListShared 列出全部共享知识库 (owner_type=shared)
func (r *KnowledgeBaseRepository) ListShared(ctx context.Context, kbType string) ([]model.KnowledgeBase, error) {
	q := r.db.WithContext(ctx).Where("owner_type = ? AND enabled = ?", model.KnowledgeBaseOwnerShared, true)
	if kbType != "" {
		q = q.Where("type = ?", kbType)
	}
	var kbs []model.KnowledgeBase
	err := q.Order("id DESC").Find(&kbs).Error
	return kbs, err
}

// Update 更新知识库
func (r *KnowledgeBaseRepository) Update(ctx context.Context, id uint, kb *model.KnowledgeBase) error {
	// 不用 Select("*") 避免 GORM 把 nil pointer / 零值字段当 NULL 写回, 引发 NOT NULL 约束失败
	// 显式列出可更新字段, 零值会被忽略 (业务字段用指针避开零值问题)
	return r.db.WithContext(ctx).Model(&model.KnowledgeBase{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"name":           kb.Name,
			"description":    kb.Description,
			"type":           kb.Type,
			"owner_type":     kb.OwnerType,
			"owner_agent_id": kb.OwnerAgentID,
			"enabled":        kb.Enabled,
			"member_count":   kb.MemberCount,
			"doc_count":      kb.DocCount,
		}).Error
}

// Delete 删除知识库
func (r *KnowledgeBaseRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.KnowledgeBase{}).Error
}

// CountByAgent 统计某智能体拥有多少私有 KB
func (r *KnowledgeBaseRepository) CountByAgent(ctx context.Context, agentID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.KnowledgeBase{}).
		Where("owner_agent_id = ?", agentID).
		Count(&count).Error
	return count, err
}
