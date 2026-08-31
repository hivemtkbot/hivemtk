package service

import (
	"fmt"
	"time"

	"context"
	"hivemtk-user/internal/model"
)

// FeishuAccountVO 飞书账号视图（敏感字段掩码）
type FeishuAccountVO struct {
	ID                uint       `json:"id"`
	AccountName       string     `json:"account_name"`
	AppID             string     `json:"app_id"`
	AppSecretMasked   string     `json:"app_secret_masked"`
	VerificationToken string     `json:"verification_token"`
	EncryptKeyMasked  string     `json:"encrypt_key_masked"`
	WebhookEnabled    bool       `json:"webhook_enabled"`
	AIAgentEnabled    bool       `json:"ai_agent_enabled"`
	LastSyncAt        *time.Time `json:"last_sync_at"`
	LastErrorAt       *time.Time `json:"last_error_at"`
	LastErrorMsg      string     `json:"last_error_msg"`
	Status            int        `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func toFeishuVO(a *model.FeishuAccount) *FeishuAccountVO {
	return &FeishuAccountVO{
		ID:                a.ID,
		AccountName:       a.AccountName,
		AppID:             a.AppID,
		AppSecretMasked:   maskSecret(a.AppSecret),
		VerificationToken: a.VerificationToken,
		EncryptKeyMasked:  maskSecret(a.EncryptKey),
		WebhookEnabled:    a.WebhookEnabled,
		AIAgentEnabled:    a.AIAgentEnabled,
		LastSyncAt:        a.LastSyncAt,
		LastErrorAt:       a.LastErrorAt,
		LastErrorMsg:      a.LastErrorMsg,
		Status:            a.Status,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
}

// FeishuAccountCreateReq 飞书账号创建请求
type FeishuAccountCreateReq struct {
	AccountName       string
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	WebhookEnabled    bool
	AIAgentEnabled    bool
}

// FeishuAccountUpdateReq 飞书账号更新请求
type FeishuAccountUpdateReq struct {
	AccountName       *string
	AppSecret         *string
	VerificationToken *string
	EncryptKey        *string
	WebhookEnabled    *bool
	AIAgentEnabled    *bool
	Status            *int
}

// ListFeishuAccountVOs 列出所有飞书账号（视图）
func (s *FeishuService) ListFeishuAccountVOs(ctx context.Context) ([]*FeishuAccountVO, error) {
	accs, err := s.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*FeishuAccountVO, 0, len(accs))
	for _, a := range accs {
		out = append(out, toFeishuVO(a))
	}
	return out, nil
}

// GetFeishuAccountVO 获取单个飞书账号（视图）
func (s *FeishuService) GetFeishuAccountVO(ctx context.Context, id uint) (*FeishuAccountVO, error) {
	acc, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	return toFeishuVO(acc), nil
}

// CreateFeishuAccountVO 创建飞书账号（视图）
func (s *FeishuService) CreateFeishuAccountVO(ctx context.Context, req FeishuAccountCreateReq) (*FeishuAccountVO, error) {
	acc := &model.FeishuAccount{
		AccountName:       req.AccountName,
		AppID:             req.AppID,
		AppSecret:         req.AppSecret,
		VerificationToken: req.VerificationToken,
		EncryptKey:        req.EncryptKey,
		WebhookEnabled:    req.WebhookEnabled,
		AIAgentEnabled:    req.AIAgentEnabled,
		Status:            1,
	}
	out, err := s.CreateAccount(ctx, acc)
	if err != nil {
		return nil, err
	}
	return toFeishuVO(out), nil
}

// UpdateFeishuAccountVO 更新飞书账号（视图）
func (s *FeishuService) UpdateFeishuAccountVO(ctx context.Context, id uint, req FeishuAccountUpdateReq) (*FeishuAccountVO, error) {
	acc, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.AccountName != nil {
		acc.AccountName = *req.AccountName
	}
	if req.AppSecret != nil && *req.AppSecret != "" {
		acc.AppSecret = *req.AppSecret
		acc.AccessToken = ""
		acc.TokenExpires = nil
	}
	if req.VerificationToken != nil {
		acc.VerificationToken = *req.VerificationToken
	}
	if req.EncryptKey != nil && *req.EncryptKey != "" {
		acc.EncryptKey = *req.EncryptKey
	}
	if req.WebhookEnabled != nil {
		acc.WebhookEnabled = *req.WebhookEnabled
	}
	if req.AIAgentEnabled != nil {
		acc.AIAgentEnabled = *req.AIAgentEnabled
	}
	if req.Status != nil {
		acc.Status = *req.Status
	}
	if err := s.UpdateAccount(ctx, acc); err != nil {
		return nil, err
	}
	return toFeishuVO(acc), nil
}

// DeleteFeishuAccountVO 删除飞书账号
func (s *FeishuService) DeleteFeishuAccountVO(ctx context.Context, id uint) error {
	return s.DeleteAccount(ctx, id)
}

// WhatsAppCloudAccountVO WhatsApp Cloud 账号视图（敏感字段掩码）
type WhatsAppCloudAccountVO struct {
	ID                 uint       `json:"id"`
	AccountName        string     `json:"account_name"`
	PhoneNumberID      string     `json:"phone_number_id"`
	WhatsAppBusinessID string     `json:"whatsapp_business_id"`
	AccessTokenMasked  string     `json:"access_token_masked"`
	VerifyToken        string     `json:"verify_token"`
	AppSecretMasked    string     `json:"app_secret_masked"`
	WebhookEnabled     bool       `json:"webhook_enabled"`
	AIAgentEnabled     bool       `json:"ai_agent_enabled"`
	LastSyncAt         *time.Time `json:"last_sync_at"`
	LastErrorAt        *time.Time `json:"last_error_at"`
	LastErrorMsg       string     `json:"last_error_msg"`
	Status             int        `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func toWhatsAppCloudVO(a *model.WhatsAppCloudAccount) *WhatsAppCloudAccountVO {
	return &WhatsAppCloudAccountVO{
		ID:                 a.ID,
		AccountName:        a.AccountName,
		PhoneNumberID:      a.PhoneNumberID,
		WhatsAppBusinessID: a.WhatsAppBusinessID,
		AccessTokenMasked:  maskSecret(a.AccessToken),
		VerifyToken:        a.VerifyToken,
		AppSecretMasked:    maskSecret(a.AppSecret),
		WebhookEnabled:     a.WebhookEnabled,
		AIAgentEnabled:     a.AIAgentEnabled,
		LastSyncAt:         a.LastSyncAt,
		LastErrorAt:        a.LastErrorAt,
		LastErrorMsg:       a.LastErrorMsg,
		Status:             a.Status,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

// WhatsAppCloudAccountCreateReq WhatsApp Cloud 账号创建请求
type WhatsAppCloudAccountCreateReq struct {
	AccountName        string
	PhoneNumberID      string
	WhatsAppBusinessID string
	AccessToken        string
	VerifyToken        string
	AppSecret          string
	WebhookEnabled     bool
	AIAgentEnabled     bool
}

// WhatsAppCloudAccountUpdateReq WhatsApp Cloud 账号更新请求
type WhatsAppCloudAccountUpdateReq struct {
	AccountName    *string
	AccessToken    *string
	VerifyToken    *string
	AppSecret      *string
	WebhookEnabled *bool
	AIAgentEnabled *bool
	Status         *int
}

// ListWhatsAppCloudAccountVOs 列出所有 WhatsApp Cloud 账号（视图）
func (s *WhatsAppCloudService) ListWhatsAppCloudAccountVOs(ctx context.Context) ([]*WhatsAppCloudAccountVO, error) {
	accs, err := s.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*WhatsAppCloudAccountVO, 0, len(accs))
	for _, a := range accs {
		out = append(out, toWhatsAppCloudVO(a))
	}
	return out, nil
}

// GetWhatsAppCloudAccountVO 获取单个 WhatsApp Cloud 账号（视图）
func (s *WhatsAppCloudService) GetWhatsAppCloudAccountVO(ctx context.Context, id uint) (*WhatsAppCloudAccountVO, error) {
	acc, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	return toWhatsAppCloudVO(acc), nil
}

// CreateWhatsAppCloudAccountVO 创建 WhatsApp Cloud 账号（视图）
func (s *WhatsAppCloudService) CreateWhatsAppCloudAccountVO(ctx context.Context, req WhatsAppCloudAccountCreateReq) (*WhatsAppCloudAccountVO, error) {
	acc := &model.WhatsAppCloudAccount{
		AccountName:        req.AccountName,
		PhoneNumberID:      req.PhoneNumberID,
		WhatsAppBusinessID: req.WhatsAppBusinessID,
		AccessToken:        req.AccessToken,
		VerifyToken:        req.VerifyToken,
		AppSecret:          req.AppSecret,
		WebhookEnabled:     req.WebhookEnabled,
		AIAgentEnabled:     req.AIAgentEnabled,
		Status:             1,
	}
	out, err := s.CreateAccount(ctx, acc)
	if err != nil {
		return nil, err
	}
	return toWhatsAppCloudVO(out), nil
}

// UpdateWhatsAppCloudAccountVO 更新 WhatsApp Cloud 账号（视图）
func (s *WhatsAppCloudService) UpdateWhatsAppCloudAccountVO(ctx context.Context, id uint, req WhatsAppCloudAccountUpdateReq) (*WhatsAppCloudAccountVO, error) {
	acc, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.AccountName != nil {
		acc.AccountName = *req.AccountName
	}
	if req.AccessToken != nil && *req.AccessToken != "" {
		acc.AccessToken = *req.AccessToken
	}
	if req.VerifyToken != nil {
		acc.VerifyToken = *req.VerifyToken
	}
	if req.AppSecret != nil && *req.AppSecret != "" {
		acc.AppSecret = *req.AppSecret
	}
	if req.WebhookEnabled != nil {
		acc.WebhookEnabled = *req.WebhookEnabled
	}
	if req.AIAgentEnabled != nil {
		acc.AIAgentEnabled = *req.AIAgentEnabled
	}
	if req.Status != nil {
		acc.Status = *req.Status
	}
	if err := s.UpdateAccount(ctx, acc); err != nil {
		return nil, err
	}
	return toWhatsAppCloudVO(acc), nil
}

// DeleteWhatsAppCloudAccountVO 删除 WhatsApp Cloud 账号
func (s *WhatsAppCloudService) DeleteWhatsAppCloudAccountVO(ctx context.Context, id uint) error {
	return s.DeleteAccount(ctx, id)
}

// maskSecret 敏感信息掩码
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s****%s", secret[:4], secret[len(secret)-4:])
}
