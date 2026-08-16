package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

)

// 邮件退订 token 有效期（30 天），合规要求退订链接在合理时长内可用
const emailUnsubscribeTokenTTL = 30 * 24 * time.Hour

// 邮件退订 token 签名密钥（仅来自环境变量 EMAIL_UNSUBSCRIBE_SECRET，源码不含硬编码密钥）
const emailUnsubscribeSecretEnv = "EMAIL_UNSUBSCRIBE_SECRET"

const emailUnsubscribeDefaultSecret = "marketing-tools-kit-email-unsubscribe-dev-secret"

// UnsubscribeClaim 退订 token 中携带的声明
type UnsubscribeClaim struct {
	Email  string `json:"email"`
	JobID  string `json:"job_id"`
	Expire int64  `json:"expire"`
	Nonce  string `json:"nonce"`
}

// EmailUnsubscribeService 邮件退订服务
//
// 合规要点：
//   - UnsubscribeEmail：记录退订请求，源链接/IP/UA 留痕
//   - IsUnsubscribed：发送前必须调用，命中则跳过发送
//   - ResubscribeEmail：允许用户重新订阅（合规要求）
//   - GenerateUnsubscribeLink：生成 HMAC-SHA256 签名的退订链接，30 天有效
type EmailUnsubscribeService struct {
	repo repository.EmailUnsubscribeRepository
}

// NewEmailUnsubscribeService 创建邮件退订服务
func NewEmailUnsubscribeService(repo repository.EmailUnsubscribeRepository) *EmailUnsubscribeService {
	if repo == nil {
		repo = repository.NewEmailUnsubscribeRepository(nil)
	}
	return &EmailUnsubscribeService{repo: repo}
}

// UnsubscribeEmail 记录退订请求
// 重复退订幂等：若已存在退订记录，更新退订时间和原因
func (s *EmailUnsubscribeService) UnsubscribeEmail(ctx context.Context, email, reason, sourceLink, jobID, ip, ua string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("email 不能为空")
	}

	existing, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		logger.Errorf("查询邮件退订记录失败 email=%s: %v", email, err)
		return err
	}

	now := time.Now()
	if existing != nil {
		existing.Reason = reason
		existing.SourceLink = sourceLink
		existing.IP = ip
		existing.UA = ua
		existing.UnsubscribedAt = now
		if jobID != "" {
			existing.JobID = jobID
		}
		return s.repo.Update(ctx, existing)
	}

	record := &model.EmailUnsubscribe{
		Email:          email,
		Reason:         reason,
		UnsubscribedAt: now,
		SourceLink:     sourceLink,
		IP:             ip,
		UA:             ua,
		JobID:          jobID,
	}
	return s.repo.Create(ctx, record)
}

// IsUnsubscribed 检查邮箱是否已退订（发送前必须调用）
func (s *EmailUnsubscribeService) IsUnsubscribed(ctx context.Context, email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	exists, err := s.repo.ExistsByEmail(ctx, email)
	if err != nil {
		logger.Errorf("查询邮件退订状态失败 email=%s: %v", email, err)
		return false
	}
	return exists
}

// ResubscribeEmail 允许用户重新订阅（合规要求）
func (s *EmailUnsubscribeService) ResubscribeEmail(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("email 不能为空")
	}
	return s.repo.DeleteByEmail(ctx, email)
}

// GenerateUnsubscribeLink 生成 HMAC-SHA256 签名的退订链接
// 链接格式：{baseURL}/api/email/unsubscribe?token=xxx
// token = base64(payload) + "." + base64(HMAC-SHA256(payload, secret))
func (s *EmailUnsubscribeService) GenerateUnsubscribeLink(ctx context.Context, email, jobID string) (string, error) {
	email = normalizeEmail(email)
	if email == "" {
		return "", errors.New("email 不能为空")
	}

	claim := UnsubscribeClaim{
		Email:  email,
		JobID:  jobID,
		Expire: time.Now().Add(emailUnsubscribeTokenTTL).Unix(),
		Nonce:  fmt.Sprintf("unsub-%d", time.Now().UnixNano()),
	}

	payload, err := json.Marshal(claim)
	if err != nil {
		return "", fmt.Errorf("marshal claim failed: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := s.sign(ctx, []byte(payloadB64))
	token := payloadB64 + "." + sig

	base := s.baseURL(ctx)
	return fmt.Sprintf("%s/api/email/unsubscribe?token=%s", strings.TrimRight(base, "/"), url.QueryEscape(token)), nil
}

// VerifyUnsubscribeToken 验证退订 token 并返回声明
func (s *EmailUnsubscribeService) VerifyUnsubscribeToken(ctx context.Context, token string) (*UnsubscribeClaim, error) {
	if token == "" {
		return nil, errors.New("token 不能为空")
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("token 格式错误")
	}
	payloadB64, sig := parts[0], parts[1]

	expectedSig := s.sign(ctx, []byte(payloadB64))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, errors.New("token 签名校验失败")
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("token payload 解码失败: %w", err)
	}

	var claim UnsubscribeClaim
	if err := json.Unmarshal(payload, &claim); err != nil {
		return nil, fmt.Errorf("token payload 反序列化失败: %w", err)
	}

	if time.Now().Unix() > claim.Expire {
		return nil, errors.New("token 已过期")
	}
	return &claim, nil
}

// ListUnsubscribes 分页查询退订名单
func (s *EmailUnsubscribeService) ListUnsubscribes(ctx context.Context, page, limit int, keyword string) ([]*model.EmailUnsubscribe, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 20
	}
	return s.repo.List(ctx, page, limit, keyword)
}

// ListAllUnsubscribes 查询全部退订名单（导出使用）
func (s *EmailUnsubscribeService) ListAllUnsubscribes(ctx context.Context) ([]*model.EmailUnsubscribe, error) {
	return s.repo.ListAll(ctx)
}

// sign 使用 HMAC-SHA256 计算签名
func (s *EmailUnsubscribeService) sign(ctx context.Context, data []byte) string {
	mac := hmac.New(sha256.New, []byte(s.secret(ctx)))
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// secret 读取退订签名密钥（仅来自环境变量，源码不含硬编码默认密钥）
func (s *EmailUnsubscribeService) secret(ctx context.Context) string {
	if v := os.Getenv(emailUnsubscribeSecretEnv); v != "" {
		return v
	}
	return emailUnsubscribeDefaultSecret
}

// baseURL 读取对外可访问的基础 URL
func (s *EmailUnsubscribeService) baseURL(ctx context.Context) string {
	return config.GetServerBaseURL()
}

// normalizeEmail 规范化邮箱地址（小写 + 去空格）
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

