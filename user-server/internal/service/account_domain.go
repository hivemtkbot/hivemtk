package service

import (
	"strconv"

	"context"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// ============================================================================
// Account / Clue 领域 DTO 外观方法（保持原 model 签名方法不变，新增 DTO 外观）
// 供 controller 层使用，避免 controller 直接构造/读取 model。
// ============================================================================

// NewAccountServiceWithRepo 创建带 repository 的 AccountService（兼容原构造函数语义）
func NewAccountServiceWithRepo(repo repository.AccountRepository) *AccountService {
	return &AccountService{repo: repo}
}

// CreateAccountDTO 根据请求 DTO 创建商户账户，并返回响应 DTO
func (s *AccountService) CreateAccountDTO(ctx context.Context, req dto.CreateAccountRequest) (*dto.AccountResponse, error) {
	account := model.Account{
		TgBotToken:          req.TgBotToken,
		Price:               req.Price,
		GroupID:             req.GroupID,
		ProxyEnableProxy:    req.ProxyEnableProxy,
		ProxyProtoclo:       req.ProxyProtoclo,
		ProxyHost:           req.ProxyHost,
		ProxyPort:           req.ProxyPort,
	}
	created, err := s.CreateAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return toAccountResponse(created), nil
}

// GetAccountDTO 根据 ID 获取账户响应 DTO
func (s *AccountService) GetAccountDTO(ctx context.Context, id string) (*dto.AccountResponse, error) {
	account, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	return toAccountResponse(account), nil
}

// GetAccountListDTO 获取账户列表响应 DTO
func (s *AccountService) GetAccountListDTO(ctx context.Context) (*dto.GetAccountListResponse, error) {
	accounts, err := s.GetAccountList(ctx)
	if err != nil {
		return nil, err
	}
	resp := &dto.GetAccountListResponse{
		Total: int64(len(accounts)),
		List:  []*dto.AccountResponse{},
	}
	for _, a := range accounts {
		resp.List = append(resp.List, toAccountResponse(a))
	}
	return resp, nil
}

// UpdateAccountDTO 根据请求 DTO 更新账户，并返回响应 DTO
func (s *AccountService) UpdateAccountDTO(ctx context.Context, req dto.UpdateAccountRequest) (*dto.AccountResponse, error) {
	account := model.Account{
		ID:                  req.ID,
		TgName:              req.TgName,
		TgBotToken:          req.TgBotToken,
		Price:               req.Price,
		GroupID:             req.GroupID,
		ProxyEnableProxy:    req.ProxyEnableProxy,
		ProxyProtoclo:       req.ProxyProtoclo,
		ProxyHost:           req.ProxyHost,
		ProxyPort:           req.ProxyPort,
	}
	if err := s.UpdateAccount(ctx, account); err != nil {
		return nil, err
	}
	return s.GetAccountDTO(ctx, req.ID)
}

// toAccountResponse 将 model.Account 转换为 dto.AccountResponse
func toAccountResponse(a *model.Account) *dto.AccountResponse {
	return &dto.AccountResponse{
		ID:                  a.ID,
		TgName:              a.TgName,
		TgBotToken:          a.TgBotToken,
		Price:               a.Price,
		GroupID:             a.GroupID,
		ProxyEnableProxy:    a.ProxyEnableProxy,
		ProxyProtoclo:       a.ProxyProtoclo,
		ProxyHost:           a.ProxyHost,
		ProxyPort:           a.ProxyPort,
		Status:              a.Status,
		CreateTime:          a.CreateTime,
		Msg:                 a.Msg,
		URL:                 a.URL,
	}
}

// BatchImportCluesFromDTO 批量导入线索（请求 DTO 已在 controller 完成类型校验）
func (s *ClueService) BatchImportCluesFromDTO(ctx context.Context, reqs []dto.ImportClueRequest) (int64, int64, error) {
	clues := make([]*model.Clue, 0, len(reqs))
	for _, item := range reqs {
		clueType, err := strconv.ParseInt(item.Type, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		clues = append(clues, &model.Clue{
			Name:     item.Name,
			Account:  item.Account,
			City:     item.City,
			Address:  item.Address,
			Type:     clueType,
			IsVerify: 0,
		})
	}
	return s.BatchImportClues(ctx, clues)
}
