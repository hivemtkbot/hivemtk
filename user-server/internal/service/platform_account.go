package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// PlatformAccountService 平台账号服务
type PlatformAccountService struct {
	accountRepo repository.PlatformAccountRepository
}

// NewPlatformAccountService 创建平台账号服务实例
func NewPlatformAccountService() *PlatformAccountService {
	return &PlatformAccountService{
		accountRepo: repository.NewPlatformAccountRepository(),
	}
}

// CreatePlatformAccountRequest 创建平台账号请求
type CreatePlatformAccountRequest struct {
	Platform    model.Platform `json:"platform" binding:"required"`
	AccountID   string         `json:"account_id"`
	AccountName string         `json:"account_name"`
	Config      string         `json:"config"`
}

// UpdatePlatformAccountRequest 更新平台账号请求
type UpdatePlatformAccountRequest struct {
	AccountName string `json:"account_name"`
	Config      string `json:"config"`
	Status      *int   `json:"status"`
}

// PlatformLoginRequest 平台登录请求
type PlatformLoginRequest struct {
	Credentials map[string]string `json:"credentials"`
}

// GetAccounts 获取所有账号列表
func (s *PlatformAccountService) GetAccounts(ctx context.Context) ([]*model.PlatformAccount, error) {
	return s.accountRepo.GetAll(ctx)
}

// GetAccountByID 获取账号详情
func (s *PlatformAccountService) GetAccountByID(ctx context.Context, id uint) (*model.PlatformAccount, error) {
	return s.accountRepo.GetByID(ctx, id)
}

// CreateAccount 创建账号
func (s *PlatformAccountService) CreateAccount(ctx context.Context, req *CreatePlatformAccountRequest) (*model.PlatformAccount, error) {
	account := &model.PlatformAccount{
		Platform:    req.Platform,
		AccountID:   req.AccountID,
		AccountName: req.AccountName,
		Config:      req.Config,
		Status:      1,
	}

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

// UpdateAccount 更新账号
func (s *PlatformAccountService) UpdateAccount(ctx context.Context, id uint, req *UpdatePlatformAccountRequest) (*model.PlatformAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.AccountName != "" {
		account.AccountName = req.AccountName
	}
	if req.Config != "" {
		account.Config = req.Config
	}
	if req.Status != nil {
		account.Status = *req.Status
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

// DeleteAccount 删除账号
func (s *PlatformAccountService) DeleteAccount(ctx context.Context, id uint) error {
	return s.accountRepo.Delete(ctx, id)
}

// Login 登录
//
// 原实现依赖 internal/platform 的 CDP 无头浏览器适配器（BrowserAdapter）对
// 抖音/快手/小红书/咸鱼/tiktok 做服务端扫码登录，该通道已删除。这些平台现由独立
// 桥接模块对接（登录在前端扩展侧完成），后端不再提供服务端无头登录。
// 故此处对所有平台统一返回"不支持"，由调用方走到桥接登录流程。
func (s *PlatformAccountService) Login(ctx context.Context, id uint, req *PlatformLoginRequest) (*model.PlatformAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("平台 %s 不支持服务端无头登录（CDP 自动回复通道已移除，请通过桥接扩展登录）", account.Platform)
}

// CheckLoginStatus 检查登录状态
//
// 同上：服务端 CDP 登录态检查已随通道删除移除，登录态改由桥接模块维护。
func (s *PlatformAccountService) CheckLoginStatus(ctx context.Context, id uint) (bool, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}

	return false, &UnsupportedCapabilityError{Capability: "登录态检查", Platform: string(account.Platform)}
}

// UnsupportedCapabilityError 明确的"能力已下线"业务错误（400 语义）
type UnsupportedCapabilityError struct {
	Capability string
	Platform   string
}

func (e *UnsupportedCapabilityError) Error() string {
	return fmt.Sprintf("平台 %s 不支持服务端%s（CDP 自动回复通道已移除，登录态由桥接模块维护）", e.Platform, e.Capability)
}

var ErrPermissionDenied = &PermissionDeniedError{}

type PermissionDeniedError struct{}

func (e *PermissionDeniedError) Error(ctx context.Context) string {
	return "权限不足"
}
