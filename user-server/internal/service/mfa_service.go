package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// TOTP 默认参数（RFC 6238）
//   - 时间步长：30 秒
//   - 数字位数：6
//   - 哈希算法：SHA1
//   - 密钥长度：20 字节（base32 编码后 32 字符）
const (
	totpTimeStep    = 30
	totpDigits      = 6
	totpSecretBytes = 20
	// 允许前后各 1 个时间窗口（±30 秒），平衡时钟漂移与安全
	totpAllowedWindow = 1
)

// MFASetupResponse MFA 设置响应
type MFASetupResponse struct {
	Secret     string `json:"secret"`      // base32 密钥（前端可手动输入）
	OTPAuthURL string `json:"otpauth_url"` // otpauth://provisioning URI（前端生成二维码）
	QRCodeURL  string `json:"qr_code_url"` // 直接可渲染的二维码图片 URL（chart API）
}

// MFAVerifyRequest MFA 验证请求
type MFAVerifyRequest struct {
	TempToken string `json:"temp_token" binding:"required"` // 登录后下发的临时令牌
	Code      string `json:"code" binding:"required,len=6"` // 6 位 TOTP 码
}

// MFASetupVerifyRequest MFA 设置确认请求（用户扫码后输入 6 位码确认）
type MFASetupVerifyRequest struct {
	Code string `json:"code" binding:"required,len=6"`
}

// MFADisableRequest MFA 禁用请求
type MFADisableRequest struct {
	Code     string `json:"code" binding:"required,len=6"`
	Password string `json:"password" binding:"required"`
}

// MFAService MFA 服务
type MFAService struct{}

// NewMFAService 创建 MFA 服务
func NewMFAService() *MFAService {
	return &MFAService{}
}

// tempTokenStore 临时令牌存储（登录后等待 MFA 验证阶段）
// 设计：单进程内存，5 分钟过期；多实例部署需迁移到 Redis
var (
	tempTokenStore      = make(map[string]tempTokenEntry)
	tempTokenStoreMutex sync.RWMutex
)

type tempTokenEntry struct {
	UserID    uint
	Username  string
	Role      string
	ExpiresAt time.Time
}

// GenerateMFASecret 生成 TOTP 密钥
// 返回 base32 编码的密钥
func (s *MFAService) GenerateMFASecret() (string, error) {
	secret := make([]byte, totpSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("生成密钥失败: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// GenerateOTPAuthURL 生成 otpauth URL（用于二维码扫描）
// 格式：otpauth://totp/<issuer>:<account>?secret=<secret>&issuer=<issuer>&algorithm=SHA1&digits=6&period=30
func (s *MFAService) GenerateOTPAuthURL(secret, account, issuer string) string {
	if issuer == "" {
		issuer = "MarketingSystem"
	}
	if account == "" {
		account = "user"
	}
	label := fmt.Sprintf("%s:%s", url.PathEscape(issuer), url.PathEscape(account))
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpTimeStep))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, params.Encode())
}

// GenerateTOTP 生成指定时间戳的 TOTP 码
// 实现 RFC 6238
func (s *MFAService) GenerateTOTP(secret string, t time.Time) (string, error) {
	// base32 解码密钥
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("密钥解码失败: %w", err)
	}

	// 计算时间步计数器
	counter := t.Unix() / totpTimeStep
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))

	// HMAC-SHA1
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	hash := mac.Sum(nil)

	// 动态截取
	offset := int(hash[len(hash)-1] & 0x0F)
	code := (int(hash[offset])&0x7F)<<24 |
		int(hash[offset+1])&0xFF<<16 |
		int(hash[offset+2])&0xFF<<8 |
		int(hash[offset+3])&0xFF

	// 取模得到 6 位
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	code = code % int(mod)

	return fmt.Sprintf("%0*d", totpDigits, code), nil
}

// VerifyTOTP 验证 TOTP 码
// 允许 ±1 个时间窗口（±30 秒），平衡时钟漂移
func (s *MFAService) VerifyTOTP(secret, code string) bool {
	return s.VerifyTOTPAt(secret, code, time.Now())
}

// VerifyTOTPAt 验证指定时间点的 TOTP 码
func (s *MFAService) VerifyTOTPAt(secret, code string, now time.Time) bool {
	if len(code) != totpDigits {
		return false
	}
	// 检查窗口 [-1, 0, +1]
	for offset := -totpAllowedWindow; offset <= totpAllowedWindow; offset++ {
		expected, err := s.GenerateTOTP(secret, now.Add(time.Duration(offset)*time.Second*totpTimeStep))
		if err != nil {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

// SetupMFA 设置 MFA：生成密钥并暂存到 user_mfa 表（mfa_enabled=false）
// 返回 otpauth URL 供前端生成二维码
func (s *MFAService) SetupMFA(userID uint, username string) (*MFASetupResponse, error) {
	secret, err := s.GenerateMFASecret()
	if err != nil {
		return nil, err
	}

	otpAuthURL := s.GenerateOTPAuthURL(secret, username, "MarketingSystem")

	database := db.GetDB()

	// 查找或创建 MFA 记录
	var mfa model.UserMFA
	result := database.Where("user_id = ?", userID).First(&mfa)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logger.Error(result.Error, "MFA 设置查询失败")
		return nil, errors.New("MFA 设置失败")
	}

	mfa.UserID = userID
	mfa.MFASecret = secret
	mfa.MFAEnabled = false // 设置阶段不启用，需用户验证一次后启用
	mfa.MFAType = model.MFATypeTOTP

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := database.Create(&mfa).Error; err != nil {
			logger.Error(err, "MFA 记录创建失败")
			return nil, errors.New("MFA 设置失败")
		}
	} else {
		if err := database.Save(&mfa).Error; err != nil {
			logger.Error(err, "MFA 记录保存失败")
			return nil, errors.New("MFA 设置失败")
		}
	}

	return &MFASetupResponse{
		Secret:     secret,
		OTPAuthURL: otpAuthURL,
		QRCodeURL:  fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=%s", url.QueryEscape(otpAuthURL)),
	}, nil
}

// ConfirmMFASetup 确认 MFA 设置：用户输入 6 位码验证，验证成功后启用 MFA
func (s *MFAService) ConfirmMFASetup(userID uint, code string) error {
	database := db.GetDB()
	var mfa model.UserMFA
	if err := database.Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("请先调用 MFA 设置接口生成密钥")
		}
		return errors.New("MFA 验证失败")
	}

	if mfa.MFAEnabled {
		return errors.New("MFA 已启用")
	}

	if !s.VerifyTOTP(mfa.MFASecret, code) {
		return errors.New("验证码错误")
	}

	now := time.Now()
	mfa.MFAEnabled = true
	mfa.EnabledAt = &now
	// 注意：ConfirmMFASetup 不设置 LastUsedCode / LastUsedAt，
	// 这两个字段只在 VerifyMFALogin 中写入，用于登录阶段的 60s 重放保护。
	// 否则 Confirm 阶段使用的 TOTP 会被立即标记为已用，登录时即便凭据正确也会被拒。

	if err := database.Save(&mfa).Error; err != nil {
		return errors.New("MFA 启用失败")
	}
	return nil
}

// DisableMFA 禁用 MFA
// 需要校验当前密码 + TOTP 码（双重保护，防误关）
func (s *MFAService) DisableMFA(userID uint, password, code string) error {
	database := db.GetDB()

	// 校验用户密码
	var user model.SystemUser
	if err := database.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if !user.CheckPassword(password) {
		return errors.New("密码错误")
	}

	// 校验 TOTP
	var mfa model.UserMFA
	if err := database.Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		return errors.New("MFA 未设置")
	}
	if !mfa.MFAEnabled {
		return errors.New("MFA 未启用")
	}
	if !s.VerifyTOTP(mfa.MFASecret, code) {
		return errors.New("验证码错误")
	}

	now := time.Now()
	mfa.MFAEnabled = false
	mfa.DisabledAt = &now
	if err := database.Save(&mfa).Error; err != nil {
		return errors.New("MFA 禁用失败")
	}
	return nil
}

// IsMFAEnabled 检查用户是否启用了 MFA
func (s *MFAService) IsMFAEnabled(userID uint) (bool, error) {
	database := db.GetDB()
	var mfa model.UserMFA
	if err := database.Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return mfa.MFAEnabled, nil
}

// IssueTempToken 颁发临时令牌（登录密码验证 OK 后，等待 MFA 二次验证）
// 有效期 5 分钟
func (s *MFAService) IssueTempToken(userID uint, username, role string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", errors.New("生成临时令牌失败")
	}
	token := fmt.Sprintf("%x", tokenBytes)

	tempTokenStoreMutex.Lock()
	defer tempTokenStoreMutex.Unlock()

	tempTokenStore[token] = tempTokenEntry{
		UserID:    userID,
		Username:  username,
		Role:      role,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	// 异步清理过期令牌
	go s.cleanupExpiredTempTokens()

	return token, nil
}

// ValidateTempToken 校验临时令牌并返回用户信息
func (s *MFAService) ValidateTempToken(token string) (uint, string, string, error) {
	tempTokenStoreMutex.Lock()
	defer tempTokenStoreMutex.Unlock()

	entry, ok := tempTokenStore[token]
	if !ok {
		return 0, "", "", errors.New("临时令牌无效")
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(tempTokenStore, token)
		return 0, "", "", errors.New("临时令牌已过期")
	}
	return entry.UserID, entry.Username, entry.Role, nil
}

// ConsumeTempToken 消费临时令牌（验证成功后删除）
func (s *MFAService) ConsumeTempToken(token string) {
	tempTokenStoreMutex.Lock()
	defer tempTokenStoreMutex.Unlock()
	delete(tempTokenStore, token)
}

// cleanupExpiredTempTokens 清理过期令牌
func (s *MFAService) cleanupExpiredTempTokens() {
	tempTokenStoreMutex.Lock()
	defer tempTokenStoreMutex.Unlock()
	now := time.Now()
	for k, v := range tempTokenStore {
		if now.After(v.ExpiresAt) {
			delete(tempTokenStore, k)
		}
	}
}

// VerifyMFALogin MFA 登录验证
// 步骤：校验临时令牌 → 校验 TOTP 码 → 检查重放 → 更新 last_used_at → 返回用户 ID
func (s *MFAService) VerifyMFALogin(tempToken, code string) (uint, string, string, error) {
	userID, username, role, err := s.ValidateTempToken(tempToken)
	if err != nil {
		return 0, "", "", err
	}

	database := db.GetDB()
	var mfa model.UserMFA
	if err := database.Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		return 0, "", "", errors.New("MFA 未设置")
	}
	if !mfa.MFAEnabled {
		return 0, "", "", errors.New("MFA 未启用")
	}

	// 防重放：同一码 30 秒内不可重复使用
	if mfa.LastUsedCode == code && mfa.LastUsedAt != nil && time.Since(*mfa.LastUsedAt) < 60*time.Second {
		return 0, "", "", errors.New("验证码已使用，请等待下一次刷新")
	}

	if !s.VerifyTOTP(mfa.MFASecret, code) {
		return 0, "", "", errors.New("验证码错误")
	}

	now := time.Now()
	mfa.LastUsedAt = &now
	mfa.LastUsedCode = code
	if err := database.Save(&mfa).Error; err != nil {
		logger.Errorf("MFA last_used_at 更新失败: %v", err)
	}

	s.ConsumeTempToken(tempToken)
	return userID, username, role, nil
}

// GenerateBackupCodes 生成 10 个一次性恢复码
// 返回明文（仅展示给用户一次）+ bcrypt 哈希（存储到数据库）
func (s *MFAService) GenerateBackupCodes(userID uint) ([]string, error) {
	codes := make([]string, 10)
	hashedCodes := make([]string, 10)
	for i := 0; i < 10; i++ {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := fmt.Sprintf("%x", raw)
		codes[i] = code
		hashed, err := bcrypt.HashPassword(code)
		if err != nil {
			return nil, err
		}
		hashedCodes[i] = hashed
	}

	// 序列化哈希数组为 JSON
	hashedJSON := "[]"
	for i, h := range hashedCodes {
		if i == 0 {
			hashedJSON = fmt.Sprintf(`["%s"`, h)
		} else {
			hashedJSON += fmt.Sprintf(`,"%s"`, h)
		}
	}
	if len(hashedCodes) > 0 {
		hashedJSON += "]"
	}

	database := db.GetDB()
	if err := database.Model(&model.UserMFA{}).Where("user_id = ?", userID).
		Update("backup_codes", hashedJSON).Error; err != nil {
		return nil, err
	}
	return codes, nil
}
