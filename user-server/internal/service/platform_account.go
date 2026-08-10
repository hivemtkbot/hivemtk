package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/platform"
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
func (s *PlatformAccountService) Login(ctx context.Context, id uint, req *PlatformLoginRequest) (*model.PlatformAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 获取平台适配器
	adapter, err := platform.GetAdapter(account.Platform)
	if err != nil {
		return nil, err
	}

	// 执行登录
	result, err := adapter.Login(req.Credentials)
	if err != nil {
		return nil, err
	}

	// 更新账号信息
	account.AccountID = result.AccountID
	account.AccountName = result.AccountName
	account.Status = 1

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

// CheckLoginStatus 检查登录状态
func (s *PlatformAccountService) CheckLoginStatus(ctx context.Context, id uint) (bool, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}

	// 获取平台适配器
	adapter, err := platform.GetAdapter(account.Platform)
	if err != nil {
		return false, err
	}

	return adapter.CheckLoginStatus(account.AccountID)
}

var ErrPermissionDenied = &PermissionDeniedError{}

type PermissionDeniedError struct{}

func (e *PermissionDeniedError) Error(ctx context.Context) string {
	return "权限不足"
}
