package service

import (
	"fmt"
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
	"context"
)

type AccountService struct {
	repo repository.AccountRepository
}

func NewAccountService() *AccountService {
	return &AccountService{repo: repository.NewAccountRepository()}
}

func (s *AccountService) CreateAccount(ctx context.Context, account model.Account) (*model.Account, error) {
	if err := s.repo.Create(ctx, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountService) GetAccount(ctx context.Context, id string) (*model.Account, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AccountService) GetAccountList(ctx context.Context,) ([]*model.Account, error) {
	return s.repo.GetAccountList(ctx)
}

func (s *AccountService) UpdateAccount(ctx context.Context, account model.Account) error {
	return s.repo.Update(ctx, &account)
}

func (s *AccountService) DeleteAccount(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *AccountService) UpdateAccountStatusById(ctx context.Context, id string, status _type.AccountStatusType, failMsg string) error {
	return s.repo.UpdateAccountStatusById(ctx, id, status, failMsg)
}

func (s *AccountService) UpdateAccountTgNameById(ctx context.Context, id string, TgName string) error {
	return s.repo.UpdateAccountTgNameById(ctx, id, TgName)
}

func (s *AccountService) GetEpayConfigByID(ctx context.Context, account_ID string) (_type.EpayConfig, error) {
	account, err := s.repo.GetByID(ctx, account_ID)
	if err != nil {
		return _type.EpayConfig{}, err
	}
	EpayConfig := _type.EpayConfig{
		Pid:		account.EpayPid,
		Key:		account.EpayKey,
		Type:		account.EpayPayType,
		NotifyUrl:	fmt.Sprintf("%s%s", account.EpayURL, "/api/epay_notify"),
		ReturnUrl:	account.URL,
		QueryUrl:	account.EpayQueryUrl,
		EpayUrl:	account.EpayURL,
	}
	return EpayConfig, nil
}
