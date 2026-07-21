package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// ============================================================================
// 多 AI 智能体架构 - Service 层
// ----------------------------------------------------------------------------
// 三个 Service：
//  1. AIAgentService              - 智能体管理 + AgentContext 加载 + 缓存
//  2. ChannelAgentBindingService  - 渠道绑定业务（含主智能体切换）
//  3. CustomerServiceAgentService - 客服挂载业务
//
// 五层架构：业务逻辑层，依赖 Repository，被 Controller 调用
// ============================================================================

// ----------------------------------------------------------------------------
// AgentContext - 智能体执行上下文（SalesEngine 按此执行）
// ----------------------------------------------------------------------------

// AgentContext 智能体执行上下文
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
// 注意：Go 不允许为类型别名（type X = Y）定义方法，相关方法必须挂在 dto 包中
type AgentContext = dto.AgentContext

// ----------------------------------------------------------------------------
// AIAgentService
// ----------------------------------------------------------------------------

// AIAgentService AI 智能体服务
type AIAgentService struct {
	repo *repository.AIAgentRepository
	db   *gorm.DB

	// AgentContext 缓存：agentID → *AgentContext（带 TTL）
	cacheMu  sync.RWMutex
	cache    map[uint]*agentCacheEntry
	cacheTTL time.Duration
}

type agentCacheEntry struct {
	ctx       *AgentContext
	expiresAt time.Time
}

// NewAIAgentService 创建智能体服务
func NewAIAgentService(db *gorm.DB) *AIAgentService {
	repo := repository.NewAIAgentRepository()
	repo.SetDB(db)
	return &AIAgentService{
		repo:     repo,
		db:       db,
		cache:    make(map[uint]*agentCacheEntry),
		cacheTTL: 30 * time.Second, // 30s 缓存 TTL，平衡一致性与性能
	}
}

// SetRepository 注入 repository（用于测试）
func (s *AIAgentService) SetRepository(repo *repository.AIAgentRepository) {
	if repo != nil {
		s.repo = repo
	}
}

// Create 创建智能体
func (s *AIAgentService) Create(a *model.AIAgent) error {
	if a.AgentCode == "" {
		return errors.New("agent_code 不能为空")
	}
	if a.Name == "" {
		return errors.New("name 不能为空")
	}
	if a.Persona == "" {
		return errors.New("persona 不能为空")
	}
	if a.AgentType == "" {
		a.AgentType = string(model.AgentTypeSales)
	}
	// 校验 agent_code 唯一性
	if existing, _ := s.repo.GetByCode(a.AgentCode); existing != nil {
		return fmt.Errorf("agent_code %s 已存在", a.AgentCode)
	}
	if err := s.repo.Create(a); err != nil {
		return err
	}
	s.invalidateCache(a.ID)
	return nil
}

// GetByID 获取智能体详情
func (s *AIAgentService) GetByID(id uint) (*model.AIAgent, error) {
	return s.repo.GetByID(id)
}

// List 列表查询
func (s *AIAgentService) List(agentType string, status int, keyword string) ([]*model.AIAgent, error) {
	return s.repo.List(agentType, status, keyword)
}

// ListEnabled 获取所有启用的智能体（供下拉选择）
func (s *AIAgentService) ListEnabled() ([]*model.AIAgent, error) {
	return s.repo.ListEnabled()
}

// Update 更新智能体
func (s *AIAgentService) Update(a *model.AIAgent) error {
	if a.ID == 0 {
		return errors.New("id 不能为空")
	}
	if a.Name == "" {
		return errors.New("name 不能为空")
	}
	if a.Persona == "" {
		return errors.New("persona 不能为空")
	}
	// version + 1 标记配置已变更
	a.Version = a.Version + 1
	if err := s.repo.Update(a); err != nil {
		return err
	}
	s.invalidateCache(a.ID)
	return nil
}

// UpdateStatus 更新状态
func (s *AIAgentService) UpdateStatus(id uint, status int) error {
	if err := s.repo.UpdateStatus(id, status); err != nil {
		return err
	}
	s.invalidateCache(id)
	return nil
}

// Delete 删除智能体
// 业务约束：若智能体被渠道绑定或客服挂载引用，应先解绑
func (s *AIAgentService) Delete(id uint) error {
	// 校验是否被引用
	var bindings int64
	s.repo.GetDB().Model(&model.ChannelAgentBinding{}).Where("agent_id = ?", id).Count(&bindings)
	if bindings > 0 {
		return fmt.Errorf("智能体被 %d 个渠道账号绑定，请先解绑", bindings)
	}
	var mounts int64
	s.repo.GetDB().Model(&model.CustomerServiceAgent{}).Where("ai_agent_id = ?", id).Count(&mounts)
	if mounts > 0 {
		return fmt.Errorf("智能体被 %d 个客服座席挂载，请先解挂", mounts)
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.invalidateCache(id)
	return nil
}

// LoadContext 加载智能体执行上下文（带缓存）
// 返回 nil, nil 表示未找到（不视为错误，调用方回退默认配置）
func (s *AIAgentService) LoadContext(ctx context.Context, agentID uint) (*AgentContext, error) {
	if agentID == 0 {
		return nil, nil
	}
	// 1. 查缓存
	s.cacheMu.RLock()
	if entry, ok := s.cache[agentID]; ok && time.Now().Before(entry.expiresAt) {
		s.cacheMu.RUnlock()
		return entry.ctx, nil
	}
	s.cacheMu.RUnlock()

	// 2. 查数据库
	agent, err := s.repo.GetByID(agentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if agent.Status != 1 {
		return nil, nil // 禁用智能体视为未绑定
	}

	// 3. 构造 AgentContext
	agentCtx := &AgentContext{
		AgentID:              agent.ID,
		AgentCode:            agent.AgentCode,
		Name:                 agent.Name,
		AgentType:            agent.AgentType,
		Persona:              agent.Persona,
		SystemPrompt:         agent.SystemPrompt,
		Greeting:             agent.Greeting,
		RagProductIDs:        []string(agent.RagProductIDs),
		SOPIDs:               []string(agent.SOPIDs),
		ScriptLibraryIDs:     []string(agent.ScriptLibraryIDs),
		LLMModel:             agent.LLMModel,
		LLMProviderConfig:    agent.LLMProviderConfig,
		Temperature:          agent.Temperature,
		MaxTokens:            agent.MaxTokens,
		TopP:                 agent.TopP,
		FrequencyPenalty:     agent.FrequencyPenalty,
		PresencePenalty:      agent.PresencePenalty,
		EnableRAG:            agent.EnableRAG,
		EnableScriptMatch:    agent.EnableScriptMatch,
		EnableHumanizePolish: agent.EnableHumanizePolish,
		EnableContentAudit:   agent.EnableContentAudit,
		EnablePlaybook:       agent.EnablePlaybook,
		RAGTopK:              agent.RAGTopK,
		ConfidenceThreshold:  agent.ConfidenceThreshold,
		MaxAIConsecutive:     agent.MaxAIConsecutive,
	}

	// 4. 写缓存
	s.cacheMu.Lock()
	s.cache[agentID] = &agentCacheEntry{
		ctx:       agentCtx,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
	s.cacheMu.Unlock()

	return agentCtx, nil
}

// invalidateCache 失效缓存
func (s *AIAgentService) invalidateCache(agentID uint) {
	s.cacheMu.Lock()
	delete(s.cache, agentID)
	s.cacheMu.Unlock()
}

// TestAgent 测试智能体执行
// 输入消息和客户ID，返回 SalesEngine 完整链路日志
// 此方法在 Controller 层调用，由 Controller 注入 SalesEngine
func (s *AIAgentService) TestAgent(ctx context.Context, agentID uint, customerID, message string, engine *SalesEngine) (*SalesResponse, error) {
	agentCtx, err := s.LoadContext(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("加载智能体上下文失败: %w", err)
	}
	if agentCtx == nil {
		return nil, errors.New("智能体不存在或已禁用")
	}
	if engine == nil {
		return nil, errors.New("SalesEngine 未注入")
	}
	req := &SalesRequest{
		CustomerID:  customerID,
		UserMessage: message,
		AutoExecute: true,
	}
	return engine.HandleWithAgent(ctx, req, agentCtx)
}

// ----------------------------------------------------------------------------
// ChannelAgentBindingService
// ----------------------------------------------------------------------------

// ChannelAgentBindingService 渠道绑定服务
type ChannelAgentBindingService struct {
	repo      *repository.ChannelAgentBindingRepository
	agentRepo *repository.AIAgentRepository
	agentSvc  *AIAgentService
	db        *gorm.DB
}

// NewChannelAgentBindingService 创建渠道绑定服务
func NewChannelAgentBindingService(db *gorm.DB, agentSvc *AIAgentService) *ChannelAgentBindingService {
	repo := repository.NewChannelAgentBindingRepository()
	repo.SetDB(db)
	agentRepo := repository.NewAIAgentRepository()
	agentRepo.SetDB(db)
	return &ChannelAgentBindingService{
		repo:      repo,
		agentRepo: agentRepo,
		agentSvc:  agentSvc,
		db:        db,
	}
}

// Create 创建绑定
// 业务约束：agentID 必须存在且启用；若 is_primary=true，需先清除同账号其他主绑定
func (s *ChannelAgentBindingService) Create(b *model.ChannelAgentBinding) error {
	if b.ChannelType == "" {
		return errors.New("channel_type 不能为空")
	}
	if b.AccountID == "" {
		return errors.New("account_id 不能为空")
	}
	if b.AgentID == 0 {
		return errors.New("agent_id 不能为空")
	}
	// 校验智能体存在
	agent, err := s.agentRepo.GetByID(b.AgentID)
	if err != nil {
		return fmt.Errorf("智能体不存在: %w", err)
	}
	if agent.Status != 1 {
		return errors.New("智能体已禁用，无法绑定")
	}
	// 主绑定切换：先清空同账号其他主绑定
	if b.IsPrimary {
		if err := s.repo.ClearPrimaryByChannelAccount(b.ChannelType, b.AccountID); err != nil {
			return fmt.Errorf("清除旧主绑定失败: %w", err)
		}
	}
	return s.repo.Create(b)
}

// ListByChannelAccount 按渠道账号查询所有绑定
func (s *ChannelAgentBindingService) ListByChannelAccount(channelType, accountID string) ([]*model.ChannelAgentBinding, error) {
	return s.repo.ListByChannelAccount(channelType, accountID)
}

// ListByAgentID 反查智能体被哪些渠道使用
func (s *ChannelAgentBindingService) ListByAgentID(agentID uint) ([]*model.ChannelAgentBinding, error) {
	return s.repo.ListByAgentID(agentID)
}

// GetByID 获取绑定详情
func (s *ChannelAgentBindingService) GetByID(id uint) (*model.ChannelAgentBinding, error) {
	return s.repo.GetByID(id)
}

// Update 更新绑定
func (s *ChannelAgentBindingService) Update(b *model.ChannelAgentBinding) error {
	if b.IsPrimary {
		// 主绑定切换：先清除同账号其他主绑定（排除自身）
		existing, _ := s.repo.GetByID(b.ID)
		if existing != nil && (existing.ChannelType != b.ChannelType || existing.AccountID != b.AccountID) {
			// 渠道/账号变更，清除新账号下的旧主绑定
			if err := s.repo.ClearPrimaryByChannelAccount(b.ChannelType, b.AccountID); err != nil {
				return err
			}
		} else if existing != nil {
			// 同账号内切换主绑定，先清除其他
			if err := s.repo.ClearPrimaryByChannelAccount(existing.ChannelType, existing.AccountID); err != nil {
				return err
			}
		}
	}
	if err := s.repo.Update(b); err != nil {
		return err
	}
	return nil
}

// Delete 删除绑定
func (s *ChannelAgentBindingService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// LoadAgentForChannel 加载渠道账号绑定的主智能体上下文
// WebhookService.triggerSalesEngine 调用此方法
// 返回 nil, nil 表示未绑定（调用方回退默认配置）
func (s *ChannelAgentBindingService) LoadAgentForChannel(ctx context.Context, channelType, accountID string) (*AgentContext, error) {
	binding, err := s.repo.GetPrimaryByChannelAccount(channelType, accountID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, nil
	}
	return s.agentSvc.LoadContext(ctx, binding.AgentID)
}

// NormalizeChannelType 规范化渠道类型字符串
// 将 WebhookChannel 转换为 model.ChannelType
func NormalizeChannelType(ch string) string {
	ch = strings.ToLower(strings.TrimSpace(ch))
	switch ch {
	case "telegram", "tg":
		return string(model.ChannelTypeTelegram)
	case "wecom", "wechat_work", "wechatwork":
		return string(model.ChannelTypeWeCom)
	case "feishu", "lark":
		return string(model.ChannelTypeFeishu)
	case "whatsapp", "wa":
		return string(model.ChannelTypeWhatsApp)
	case "douyin":
		return string(model.ChannelTypeDouyin)
	case "xiaohongshu", "xhs":
		return string(model.ChannelTypeXiaohongshu)
	case "kuaishou", "ks":
		return string(model.ChannelTypeKuaishou)
	case "xianyu":
		return string(model.ChannelTypeXianyu)
	case "tiktok":
		return string(model.ChannelTypeTikTok)
	default:
		return ch
	}
}

// ----------------------------------------------------------------------------
// CustomerServiceAgentService
// ----------------------------------------------------------------------------

// CustomerServiceAgentService 客服挂载服务
type CustomerServiceAgentService struct {
	repo      *repository.CustomerServiceAgentRepository
	agentRepo *repository.AIAgentRepository
	agentSvc  *AIAgentService
	db        *gorm.DB
}

// NewCustomerServiceAgentService 创建客服挂载服务
func NewCustomerServiceAgentService(db *gorm.DB, agentSvc *AIAgentService) *CustomerServiceAgentService {
	repo := repository.NewCustomerServiceAgentRepository()
	repo.SetDB(db)
	agentRepo := repository.NewAIAgentRepository()
	agentRepo.SetDB(db)
	return &CustomerServiceAgentService{
		repo:      repo,
		agentRepo: agentRepo,
		agentSvc:  agentSvc,
		db:        db,
	}
}

// Create 创建挂载
func (s *CustomerServiceAgentService) Create(c *model.CustomerServiceAgent) error {
	if c.AgentStatusID == 0 {
		return errors.New("agent_status_id 不能为空")
	}
	if c.AIAgentID == 0 {
		return errors.New("ai_agent_id 不能为空")
	}
	// 校验智能体存在
	agent, err := s.agentRepo.GetByID(c.AIAgentID)
	if err != nil {
		return fmt.Errorf("智能体不存在: %w", err)
	}
	if agent.Status != 1 {
		return errors.New("智能体已禁用，无法挂载")
	}
	// 主挂载切换：先清除同座席其他主挂载
	if c.IsPrimary {
		if err := s.repo.ClearPrimaryByAgentStatusID(c.AgentStatusID); err != nil {
			return fmt.Errorf("清除旧主挂载失败: %w", err)
		}
	}
	return s.repo.Create(c)
}

// ListByAgentStatusID 按座席查询所有挂载
func (s *CustomerServiceAgentService) ListByAgentStatusID(agentStatusID uint) ([]*model.CustomerServiceAgent, error) {
	return s.repo.ListByAgentStatusID(agentStatusID)
}

// ListByAIAgentID 反查智能体被哪些客服使用
func (s *CustomerServiceAgentService) ListByAIAgentID(aiAgentID uint) ([]*model.CustomerServiceAgent, error) {
	return s.repo.ListByAIAgentID(aiAgentID)
}

// GetByID 获取挂载详情
func (s *CustomerServiceAgentService) GetByID(id uint) (*model.CustomerServiceAgent, error) {
	return s.repo.GetByID(id)
}

// Update 更新挂载
func (s *CustomerServiceAgentService) Update(c *model.CustomerServiceAgent) error {
	if c.IsPrimary {
		existing, _ := s.repo.GetByID(c.ID)
		if existing != nil {
			if err := s.repo.ClearPrimaryByAgentStatusID(existing.AgentStatusID); err != nil {
				return err
			}
		}
	}
	return s.repo.Update(c)
}

// Delete 删除挂载
func (s *CustomerServiceAgentService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// LoadAgentForSeat 加载客服座席挂载的主智能体上下文
// SmartCSOrchestrator 处理消息时调用此方法
// 返回 nil, nil 表示未挂载
func (s *CustomerServiceAgentService) LoadAgentForSeat(ctx context.Context, agentStatusID uint) (*AgentContext, error) {
	mount, err := s.repo.GetPrimaryByAgentStatusID(agentStatusID)
	if err != nil {
		return nil, err
	}
	if mount == nil {
		return nil, nil
	}
	return s.agentSvc.LoadContext(ctx, mount.AIAgentID)
}

// GetOrCreateAgentStatusByUserID 按用户ID查找座席状态，不存在则创建
// 用于前端"为团队成员挂载AI智能体"场景：团队成员即座席，AgentStatus.AgentID = 用户ID
func (s *CustomerServiceAgentService) GetOrCreateAgentStatusByUserID(userID uint, name string) (*model.AgentStatus, error) {
	if userID == 0 {
		return nil, errors.New("user_id 不能为空")
	}
	var st model.AgentStatus
	err := s.db.Where("agent_id = ?", userID).First(&st).Error
	if err == nil {
		return &st, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询座席状态失败: %w", err)
	}
	// 创建座席状态
	st = model.AgentStatus{
		AgentID:     userID,
		AgentName:   name,
		Status:      "offline",
		MaxSessions: 5,
	}
	if err := s.db.Create(&st).Error; err != nil {
		return nil, fmt.Errorf("创建座席状态失败: %w", err)
	}
	return &st, nil
}

// ListByUserID 按用户ID查询挂载（先查 AgentStatus，再查挂载）
func (s *CustomerServiceAgentService) ListByUserID(userID uint) ([]*model.CustomerServiceAgent, error) {
	if userID == 0 {
		return nil, errors.New("user_id 不能为空")
	}
	var st model.AgentStatus
	err := s.db.Where("agent_id = ?", userID).First(&st).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return []*model.CustomerServiceAgent{}, nil // 无座席状态则无挂载
		}
		return nil, fmt.Errorf("查询座席状态失败: %w", err)
	}
	return s.repo.ListByAgentStatusID(st.ID)
}

// CreateByUserID 按用户ID创建挂载（自动创建 AgentStatus）
func (s *CustomerServiceAgentService) CreateByUserID(userID uint, userName string, aiAgentID uint, isPrimary bool) (*model.CustomerServiceAgent, error) {
	if aiAgentID == 0 {
		return nil, errors.New("ai_agent_id 不能为空")
	}
	st, err := s.GetOrCreateAgentStatusByUserID(userID, userName)
	if err != nil {
		return nil, err
	}
	m := &model.CustomerServiceAgent{
		AgentStatusID: st.ID,
		AIAgentID:     aiAgentID,
		IsPrimary:     isPrimary,
		Enabled:       true,
	}
	if err := s.Create(m); err != nil {
		return nil, err
	}
	return m, nil
}
