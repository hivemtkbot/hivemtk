package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// password_policy.go 密码策略服务
//
// 五层架构归属：L4 业务层
// - 数据访问完全委托给 repository（PasswordHistoryRepository / SystemConfigKVRepository / SystemUserRepository）
// - 不再 import db 包，符合五层架构规范
//
// 默认策略（私域独立部署）：
//   - min_length: 8
//   - max_length: 64
//   - require_uppercase: true
//   - require_lowercase: true
//   - require_digit: true
//   - require_special: true（默认 false，按需启用）
//   - forbid_common: true（禁止常见弱密码）
//   - forbid_reuse: true（最近 5 个密码不能重复）
//   - expiry_days: 90（密码 90 天过期，0 表示不过期）
type PasswordPolicy struct {
	MinLength        int      `json:"min_length"`
	MaxLength        int      `json:"max_length"`
	RequireUppercase bool     `json:"require_uppercase"`
	RequireLowercase bool     `json:"require_lowercase"`
	RequireDigit     bool     `json:"require_digit"`
	RequireSpecial   bool     `json:"require_special"`
	ForbidCommon     bool     `json:"forbid_common"`
	CommonPasswords  []string `json:"common_passwords"`
	ForbidReuse      bool     `json:"forbid_reuse"`
	ReuseCount       int      `json:"reuse_count"` // 禁止重复最近 N 个密码
	ExpiryDays       int      `json:"expiry_days"` // 密码过期天数（0=不过期）
}

// DefaultPasswordPolicy 默认密码策略
var DefaultPasswordPolicy = PasswordPolicy{
	MinLength:        8,
	MaxLength:        64,
	RequireUppercase: true,
	RequireLowercase: true,
	RequireDigit:     true,
	RequireSpecial:   false,
	ForbidCommon:     true,
	CommonPasswords:  DefaultCommonPasswords,
	ForbidReuse:      true,
	ReuseCount:       5,
	ExpiryDays:       90,
}

// DefaultCommonPasswords 默认弱密码列表
var DefaultCommonPasswords = []string{
	"123456", "12345678", "123456789", "1234567890",
	"password", "Password", "PASSWORD",
	"admin", "admin123", "admin888", "admin666",
	"qwerty", "qwerty123", "abc123", "abcdef",
	"111111", "000000", "666666", "888888",
	"iloveyou", "letmein", "welcome", "monkey",
	"dragon", "master", "login", "passw0rd",
	"root", "toor", "test", "guest",
}

// specialCharRegex 特殊字符正则
var specialCharRegex = regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?~\` + "`" + `]`)

// policyCache 策略缓存（避免每次校验都查库）
var (
	policyCache      *PasswordPolicy
	policyCacheMutex sync.RWMutex
)

// PolicyKVKey 密码策略在 system_config_kv 表中存储的 key
const PolicyKVKey = "password_policy"

// PasswordPolicyService 密码策略服务
type PasswordPolicyService struct {
	kvRepo         repository.SystemConfigKVRepository
	historyRepo    repository.PasswordHistoryRepository
	systemUserRepo repository.SystemUserRepository
}

// NewPasswordPolicyService 创建密码策略服务
func NewPasswordPolicyService() *PasswordPolicyService {
	return &PasswordPolicyService{
		kvRepo:         repository.NewSystemConfigKVRepository(),
		historyRepo:    repository.NewPasswordHistoryRepository(),
		systemUserRepo: repository.NewSystemUserRepository(),
	}
}

// GetPolicy 获取当前密码策略
// 优先级：system_config_kv.password_policy > DefaultPasswordPolicy
// 结果会缓存到进程内存（修改策略后调用 InvalidatePolicyCache 失效）
func (s *PasswordPolicyService) GetPolicy() *PasswordPolicy {
	policyCacheMutex.RLock()
	if policyCache != nil {
		p := *policyCache
		policyCacheMutex.RUnlock()
		return &p
	}
	policyCacheMutex.RUnlock()

	policyCacheMutex.Lock()
	defer policyCacheMutex.Unlock()

	// 双检锁
	if policyCache != nil {
		p := *policyCache
		return &p
	}

	policy := s.loadPolicyFromDB()
	policyCache = policy
	return policy
}

// loadPolicyFromDB 从 system_config_kv 表加载密码策略
// 失败时返回默认策略
func (s *PasswordPolicyService) loadPolicyFromDB() *PasswordPolicy {
	defaultPolicy := DefaultPasswordPolicy
	ctx := context.Background()

	jsonStr, err := s.kvRepo.Get(ctx, PolicyKVKey)
	if err != nil {
		// 仓储调用失败（非 NotFound），记日志后回退默认策略
		logger.Errorf("查询密码策略失败: %v", err)
		return &defaultPolicy
	}
	if jsonStr == "" {
		return &defaultPolicy
	}

	var policy PasswordPolicy
	if err := json.Unmarshal([]byte(jsonStr), &policy); err != nil {
		logger.Errorf("密码策略 JSON 解析失败: %v", err)
		return &defaultPolicy
	}

	// 合并默认值（防止字段缺失）
	if policy.MinLength <= 0 {
		policy.MinLength = defaultPolicy.MinLength
	}
	if policy.MaxLength <= 0 {
		policy.MaxLength = defaultPolicy.MaxLength
	}
	if policy.ReuseCount <= 0 && policy.ForbidReuse {
		policy.ReuseCount = defaultPolicy.ReuseCount
	}
	if len(policy.CommonPasswords) == 0 && policy.ForbidCommon {
		policy.CommonPasswords = defaultPolicy.CommonPasswords
	}

	return &policy
}

// InvalidatePolicyCache 失效策略缓存
// 在策略更新后调用
func (s *PasswordPolicyService) InvalidatePolicyCache() {
	policyCacheMutex.Lock()
	defer policyCacheMutex.Unlock()
	policyCache = nil
}

// SavePolicy 保存密码策略到 system_config_kv 表
func (s *PasswordPolicyService) SavePolicy(policy *PasswordPolicy) error {
	if policy == nil {
		return errors.New("策略不能为空")
	}
	if err := s.validatePolicy(policy); err != nil {
		return err
	}

	// 立即更新内存缓存（即使 db 未初始化，调用方也能立即使用新 policy）
	policyCacheMutex.Lock()
	cache := *policy
	policyCache = &cache
	policyCacheMutex.Unlock()

	jsonBytes, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("策略序列化失败: %w", err)
	}
	jsonStr := string(jsonBytes)

	if _, err := s.kvRepo.Upsert(context.Background(), PolicyKVKey, jsonStr); err != nil {
		return fmt.Errorf("写入策略失败: %w", err)
	}

	s.InvalidatePolicyCache()
	return nil
}

// validatePolicy 校验策略本身的合理性
func (s *PasswordPolicyService) validatePolicy(p *PasswordPolicy) error {
	if p.MinLength < 4 {
		return errors.New("最小长度不能小于 4")
	}
	if p.MaxLength < p.MinLength {
		return errors.New("最大长度不能小于最小长度")
	}
	if p.MaxLength > 256 {
		return errors.New("最大长度不能超过 256")
	}
	if p.ForbidReuse && p.ReuseCount <= 0 {
		return errors.New("启用 forbid_reuse 时 reuse_count 必须大于 0")
	}
	if p.ExpiryDays < 0 {
		return errors.New("过期天数不能为负")
	}
	return nil
}

// ValidatePassword 校验密码是否符合策略
// 参数：password 待校验密码，userID 用户 ID（用于检查历史密码复用）
// 返回：error 描述具体不符合哪条规则
func (s *PasswordPolicyService) ValidatePassword(password string, userID uint) error {
	policy := s.GetPolicy()
	return s.validateWithPolicy(password, userID, policy)
}

// ValidatePasswordWithPolicy 使用指定策略校验密码（不查库）
// 用于测试
func (s *PasswordPolicyService) ValidatePasswordWithPolicy(password string, userID uint, policy *PasswordPolicy) error {
	if policy == nil {
		policy = &DefaultPasswordPolicy
	}
	return s.validateWithPolicy(password, userID, policy)
}

// validateWithPolicy 实际校验逻辑
func (s *PasswordPolicyService) validateWithPolicy(password string, userID uint, policy *PasswordPolicy) error {
	if password == "" {
		return errors.New("密码不能为空")
	}

	// 长度校验
	if len(password) < policy.MinLength {
		return fmt.Errorf("密码长度至少 %d 位", policy.MinLength)
	}
	if len(password) > policy.MaxLength {
		return fmt.Errorf("密码长度不能超过 %d 位", policy.MaxLength)
	}

	// 大写字母
	if policy.RequireUppercase {
		if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
			return errors.New("密码必须包含大写字母")
		}
	}

	// 小写字母
	if policy.RequireLowercase {
		if !regexp.MustCompile(`[a-z]`).MatchString(password) {
			return errors.New("密码必须包含小写字母")
		}
	}

	// 数字
	if policy.RequireDigit {
		if !regexp.MustCompile(`[0-9]`).MatchString(password) {
			return errors.New("密码必须包含数字")
		}
	}

	// 特殊字符
	if policy.RequireSpecial {
		if !specialCharRegex.MatchString(password) {
			return errors.New("密码必须包含特殊字符")
		}
	}

	// 弱密码检查
	if policy.ForbidCommon {
		lowerPwd := strings.ToLower(password)
		for _, common := range policy.CommonPasswords {
			if lowerPwd == strings.ToLower(common) {
				return errors.New("密码过于简单，请使用更复杂的密码")
			}
			// 检查是否包含常见弱密码作为子串（如 admin123 含 admin）
			if len(common) >= 5 && strings.Contains(lowerPwd, strings.ToLower(common)) {
				return errors.New("密码包含常见弱密码片段，请更换")
			}
		}
	}

	// 历史密码复用检查
	if policy.ForbidReuse && userID > 0 {
		if err := s.checkPasswordHistory(userID, password, policy.ReuseCount); err != nil {
			return err
		}
	}

	return nil
}

// checkPasswordHistory 检查密码是否与最近 N 个历史密码重复
func (s *PasswordPolicyService) checkPasswordHistory(userID uint, password string, reuseCount int) error {
	histories, err := s.historyRepo.ListRecent(context.Background(), userID, reuseCount)
	if err != nil {
		logger.Errorf("查询密码历史失败: %v", err)
		return nil // 查询失败不阻塞
	}

	for _, h := range histories {
		if bcrypt.CheckPassword(h.PasswordHash, password) == nil {
			return fmt.Errorf("密码与最近 %d 个历史密码重复，请使用不同的密码", reuseCount)
		}
	}
	return nil
}

// RecordPasswordHistory 记录密码历史
// 在密码变更成功后调用
func (s *PasswordPolicyService) RecordPasswordHistory(userID uint, newPassword string, source string) error {
	hashed, err := bcrypt.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}

	history := &model.PasswordHistory{
		UserID:       userID,
		PasswordHash: hashed,
		ChangedAt:    time.Now(),
		Source:       source,
	}
	if history.Source == "" {
		history.Source = model.PasswordSourceChangePassword
	}

	if err := s.historyRepo.Create(context.Background(), history); err != nil {
		return fmt.Errorf("写入密码历史失败: %w", err)
	}
	return nil
}

// IsPasswordExpired 检查密码是否过期
// 基于最近一次密码变更时间 + expiry_days
//
// 注意：当 policy.ExpiryDays <= 0 时直接返回 (false, nil)，不过度依赖 db 初始化。
// 这样可以让测试场景（policy 仅为内存对象）正常工作。
func (s *PasswordPolicyService) IsPasswordExpired(userID uint) (bool, error) {
	policy := s.GetPolicy()
	if policy == nil || policy.ExpiryDays <= 0 {
		return false, nil
	}

	ctx := context.Background()
	latest, err := s.historyRepo.Latest(ctx, userID)
	if err != nil {
		return false, err
	}
	if latest == nil {
		// 无历史记录：检查 user.updated_at（首次创建密码的时间）
		updatedAt, e := s.systemUserRepo.GetUpdatedAt(ctx, userID)
		if e != nil {
			return false, fmt.Errorf("查询用户失败: %w", e)
		}
		if updatedAt == nil {
			return false, nil
		}
		expiryAt := updatedAt.Add(time.Duration(policy.ExpiryDays) * 24 * time.Hour)
		return time.Now().After(expiryAt), nil
	}

	expiryAt := latest.ChangedAt.Add(time.Duration(policy.ExpiryDays) * 24 * time.Hour)
	return time.Now().After(expiryAt), nil
}

// ValidatePasswordStrength 静态函数式校验（仅校验强度，不查 DB）
// 用于初始化场景（user_id=0，无历史密码）
func ValidatePasswordStrength(password string) error {
	s := NewPasswordPolicyService()
	return s.ValidatePasswordWithPolicy(password, 0, &DefaultPasswordPolicy)
}
