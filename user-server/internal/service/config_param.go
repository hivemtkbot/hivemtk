package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	llmpkg "hivemtk-user/internal/aiagent/llm"
	knowledgesvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// paramEntry 缓存条目
type paramEntry struct {
	value     string
	expiresAt time.Time // D12: 过期时间（读时过期替代写时广播，多实例 60s 窗口失效）
}

// configParamTTL 单条缓存存活期；到期后下次读取触发整组重拉。
// 多实例失效语义：写仅本进程 invalidate，他实例最迟 TTL 后回源读到新值。
const configParamTTL = 60 * time.Second

// ConfigParamService 动态参数服务
//
// 对外提供 GetInt/GetFloat/GetBool/GetDuration/GetString 五个类型化读取方法。
// 启动时 Seed 一次，运行期读操作走内存缓存（sync.RWMutex），写操作后失效缓存。
//
// 设计约束：
//  - 不允许 nil DB（启动必须 Migrate + Seed）
//  - 缓存 key = "group.key"
//  - 读操作在缓存 miss 时拉一次 group 全量（同组一次性加载，减少 DB 往返）
type ConfigParamService struct {
	repo *repository.ConfigParamRepository
	db   *gorm.DB

	mu     sync.RWMutex
	cache  map[string]paramEntry // group.key → entry
	loaded map[string]bool        // group 是否已完整加载
	nowFn  func() time.Time       // D12: 可注入时钟（测试用），默认 time.Now；nil 安全走 now()
}

var globalConfigParam *ConfigParamService
var globalOnce sync.Once

// NewConfigParamService 构造（main 启动时调用）
func NewConfigParamService(db *gorm.DB) *ConfigParamService {
	return &ConfigParamService{
		repo:   repository.NewConfigParamRepository(db),
		db:     db,
		cache:  make(map[string]paramEntry, 256),
		loaded: make(map[string]bool, 32),
		nowFn:  time.Now,
	}
}

// SetGlobal 装配层注入单例（各 module 通过 GlobalConfigParam() 读取）
func SetGlobal(svc *ConfigParamService) {
	globalOnce.Do(func() {
		globalConfigParam = svc
	})
}

// SetGlobalForTest 测试专用：强制替换全局实例（绕过 globalOnce）。
// 生产代码禁止调用——生产一律走 SetGlobal/NewConfigParamService+Seed。
func SetGlobalForTest(svc *ConfigParamService) {
	globalConfigParam = svc
}

// GlobalConfigParam 获取全局单例（nil-safe：返回一个无 DB 的 fallback stub，
// 所有 Get* 方法走 fallback 默认值而非 panic）
func GlobalConfigParam() *ConfigParamService {
	if globalConfigParam != nil {
		return globalConfigParam
	}
	// 返回一个空实例，所有读取返回 fallback 默认值（测试/离线场景安全）
	return &ConfigParamService{
		cache:  make(map[string]paramEntry),
		loaded: make(map[string]bool),
	}
}

// -------- 种子数据 --------

// SeedConfigParams 启动时调用：AutoMigrate + Upsert 默认参数。
// 首次启动会写入全部 60+ 参数；后续启动只补齐缺失项，不覆盖用户已改值。
func SeedConfigParams(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&model.ConfigParam{}, &model.ConfigParamAuditLog{}); err != nil {
		return fmt.Errorf("AutoMigrate config_params: %w", err)
	}
	repo := repository.NewConfigParamRepository(db)
	svc := NewConfigParamService(db)
	SetGlobal(svc)

	// 注入知识库子域的 ConfigReader（避免 knowledge/service 反向依赖 internal/service 循环）
	knowledgesvc.SetConfigReader(svc)

	// 注入跨包模块的 DB 驱动 getter（llm / platform 无法 import internal/service 避免循环）
	ctxBG := context.Background()
	llmpkg.SetEmbeddingMaxBatchGetter(func() int {
		return svc.GetInt(ctxBG, "knowledge", "embedding_max_batch", 64)
	})
	platform.SetHeartbeatIntervalGetter(func() time.Duration {
		return svc.GetDuration(ctxBG, "misc", "heartbeat_interval", 3*time.Minute)
	})

	var created, existing int
	for _, def := range DefaultParamDefs() {
		p, err := repo.GetByGroupKey(ctx, def.Group, def.Key)
		if err == nil && p != nil {
			existing++
			continue
		}
		// Upsert：缺失则按默认值插入
		if err := db.WithContext(ctx).Create(&model.ConfigParam{
			Group:        def.Group,
			Key:          def.Key,
			Name:         def.Name,
			Description:  def.Description,
			ValueType:    def.ValueType,
			Value:        def.DefaultValue,
			DefaultValue: def.DefaultValue,
			Min:          def.Min,
			Max:          def.Max,
			Step:         def.Step,
			ReadOnly:     def.ReadOnly,
			Restart:      def.Restart,
			Category:     def.Category,
		}).Error; err != nil {
			logger.Warnf("[ConfigParam] seed create %s.%s failed: %v", def.Group, def.Key, err)
			continue
		}
		created++
	}
	logger.Infof("[ConfigParam] seed done: created=%d existing=%d total_defs=%d",
		created, existing, len(DefaultParamDefs()))
	return nil
}

// -------- 类型化读取 --------

func (s *ConfigParamService) GetInt(ctx context.Context, group, key string, fallback int) int {
	v, ok := s.getString(ctx, group, key)
	if !ok {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func (s *ConfigParamService) GetFloat(ctx context.Context, group, key string, fallback float64) float64 {
	v, ok := s.getString(ctx, group, key)
	if !ok {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func (s *ConfigParamService) GetBool(ctx context.Context, group, key string, fallback bool) bool {
	v, ok := s.getString(ctx, group, key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// GetDuration 值单位为秒（float 可带小数），返回 time.Duration
func (s *ConfigParamService) GetDuration(ctx context.Context, group, key string, fallback time.Duration) time.Duration {
	v, ok := s.getString(ctx, group, key)
	if !ok {
		return fallback
	}
	// 优先尝试解析 time.Duration 格式（15s/5m/1h）
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	// fallback 按秒解析
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return time.Duration(f * float64(time.Second))
}

func (s *ConfigParamService) GetString(ctx context.Context, group, key, fallback string) string {
	v, ok := s.getString(ctx, group, key)
	if !ok {
		return fallback
	}
	return v
}

// now 返回当前时间（nowFn nil 安全——直构实例（如测试）未注入时回退 time.Now）
func (s *ConfigParamService) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

// getString 核心读取 + 缓存加载
func (s *ConfigParamService) getString(ctx context.Context, group, key string) (string, bool) {
	cacheKey := group + "." + key

	// 快速路径：缓存命中（未过期）；过期 → 删 entry + 重置组加载标记，走下方全组重拉
	now := s.now()
	s.mu.RLock()
	e, hit := s.cache[cacheKey]
	expired := hit && !now.Before(e.expiresAt)
	s.mu.RUnlock()
	if hit && !expired {
		return e.value, true
	}
	if expired {
		s.mu.Lock()
		delete(s.cache, cacheKey)
		s.loaded[group] = false
		s.mu.Unlock()
	}

	// D12: 过期或无 DB 前先判 DB——nil DB 且缓存已有陈旧条目时仍返回陈旧值
	//（测试/离线场景唯一可用数据源），保持既有语义

	// DB 未接入（nil DB）→ 返回 false，由上层使用 fallback
	if s.db == nil {
		return "", false
	}

	// 首次 miss：加载整组
	s.mu.Lock()
	if s.loaded[group] {
		// 已有并发 goroutine 加载过，再查一次
		e, ok := s.cache[cacheKey]
		s.mu.Unlock()
		if ok {
			return e.value, true
		}
		return "", false
	}
	// 标记已加载（防止其他 goroutine 重复加载）
	s.loaded[group] = true
	s.mu.Unlock()

	params, err := s.repo.ListByGroup(ctx, group)
	if err != nil {
		// D12 修复：加载失败必须回滚 loaded——否则该组永久返回 fallback（既有 bug）
		logger.Warnf("[ConfigParam] load group %s failed: %v", group, err)
		s.mu.Lock()
		s.loaded[group] = false
		s.mu.Unlock()
		return "", false
	}

	s.mu.Lock()
	filledAt := s.now().Add(configParamTTL)
	for _, p := range params {
		s.cache[p.Group+"."+p.Key] = paramEntry{value: p.Value, expiresAt: filledAt}
	}
	s.mu.Unlock()

	s.mu.RLock()
	e, ok := s.cache[cacheKey]
	s.mu.RUnlock()
	if ok {
		return e.value, true
	}
	return "", false
}

// -------- 写操作（管理端用） --------

// List 返回全部参数（管理端 CRUD）
func (s *ConfigParamService) List(ctx context.Context) ([]model.ConfigParam, error) {
	return s.repo.List(ctx)
}

// ListByGroup 按分组返回参数
func (s *ConfigParamService) ListByGroup(ctx context.Context, group string) ([]model.ConfigParam, error) {
	return s.repo.ListByGroup(ctx, group)
}

// UpdateValue 更新单个参数值（会做范围校验 + 失效缓存）
func (s *ConfigParamService) UpdateValue(ctx context.Context, group, key, newValue string, actorID uint) error {
	if s.repo == nil {
		return fmt.Errorf("config_param repo not initialized")
	}
	// 范围校验
	p, err := s.repo.GetByGroupKey(ctx, group, key)
	if err != nil {
		return err
	}
	if err := validateValue(p.ValueType, newValue, p.Min, p.Max); err != nil {
		return err
	}
	if err := s.repo.UpdateValue(ctx, group, key, newValue, actorID); err != nil {
		return err
	}
	// 失效缓存
	s.invalidate(group, key)
	return nil
}

// ResetToDefault 重置单条为默认值
func (s *ConfigParamService) ResetToDefault(ctx context.Context, group, key string, actorID uint) error {
	if err := s.repo.ResetToDefault(ctx, group, key, actorID); err != nil {
		return err
	}
	s.invalidate(group, key)
	return nil
}

// BulkResetGroup 整组重置默认值
func (s *ConfigParamService) BulkResetGroup(ctx context.Context, group string, actorID uint) error {
	if err := s.repo.BulkResetGroup(ctx, group, actorID); err != nil {
		return err
	}
	s.invalidateGroup(group)
	return nil
}

// AuditLogs 变更日志
func (s *ConfigParamService) AuditLogs(ctx context.Context, limit int) ([]model.ConfigParamAuditLog, error) {
	return s.repo.AuditLogs(ctx, limit)
}

// -------- 缓存失效 --------

func (s *ConfigParamService) invalidate(group, key string) {
	s.mu.Lock()
	delete(s.cache, group+"."+key)
	s.loaded[group] = false // 必须重置，否则下次 getString 以为组已加载过直接返回 miss
	s.mu.Unlock()
}

func (s *ConfigParamService) invalidateGroup(group string) {
	s.mu.Lock()
	for k := range s.cache {
		if len(k) >= len(group) && k[:len(group)] == group {
			delete(s.cache, k)
		}
	}
	s.loaded[group] = false
	s.mu.Unlock()
}

// -------- 范围校验 --------

func validateValue(valueType, value string, min, max *string) error {
	if min == nil && max == nil {
		return nil
	}
	switch valueType {
	case "int":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("value %q not valid int: %w", value, err)
		}
		if min != nil {
			m, _ := strconv.Atoi(*min)
			if v < m {
				return fmt.Errorf("value %d < min %d", v, m)
			}
		}
		if max != nil {
			m, _ := strconv.Atoi(*max)
			if v > m {
				return fmt.Errorf("value %d > max %d", v, m)
			}
		}
	case "float":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("value %q not valid float: %w", value, err)
		}
		if min != nil {
			m, _ := strconv.ParseFloat(*min, 64)
			if v < m {
				return fmt.Errorf("value %f < min %f", v, m)
			}
		}
		if max != nil {
			m, _ := strconv.ParseFloat(*max, 64)
			if v > m {
				return fmt.Errorf("value %f > max %f", v, m)
			}
		}
	case "duration":
		// duration 也按秒 float 校验
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			// 可能是 "15s"/"5m" 这种格式，跳过校验
			return nil
		}
		if min != nil {
			m, _ := strconv.ParseFloat(*min, 64)
			if v < m {
				return fmt.Errorf("value %f < min %f", v, m)
			}
		}
		if max != nil {
			m, _ := strconv.ParseFloat(*max, 64)
			if v > m {
				return fmt.Errorf("value %f > max %f", v, m)
			}
		}
	}
	return nil
}
