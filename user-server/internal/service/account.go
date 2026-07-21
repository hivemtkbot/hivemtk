package service

import (
	"fmt"
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
)

type AccountService struct {
	repo repository.AccountRepository
}

func NewAccountService() *AccountService {
	return &AccountService{repo: repository.NewAccountRepository()}
}

func (s *AccountService) CreateAccount(account model.Account) (*model.Account, error) {
	if err := s.repo.Create(&account); err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountService) GetAccount(id string) (*model.Account, error) {
	return s.repo.GetByID(id)
}

func (s *AccountService) GetAccountList() ([]*model.Account, error) {
	return s.repo.GetAccountList()
}

func (s *AccountService) UpdateAccount(account model.Account) error {
	return s.repo.Update(&account)
}

func (s *AccountService) DeleteAccount(id string) error {
	return s.repo.Delete(id)
}

func (s *AccountService) UpdateAccountStatusById(id string, status _type.AccountStatusType, failMsg string) error {
	return s.repo.UpdateAccountStatusById(id, status, failMsg)
}

func (s *AccountService) UpdateAccountTgNameById(id string, TgName string) error {
	return s.repo.UpdateAccountTgNameById(id, TgName)
}

func (s *AccountService) GetEpayConfigByID(account_ID string) (_type.EpayConfig, error) {
	account, err := s.repo.GetByID(account_ID)
	if err != nil {
		return _type.EpayConfig{}, err
	}
	EpayConfig := _type.EpayConfig{
		Pid:       account.EpayPid,
		Key:       account.EpayKey,
		Type:      account.EpayPayType,
		NotifyUrl: fmt.Sprintf("%s%s", account.EpayURL, "/api/epay_notify"),
		ReturnUrl: account.URL,
		QueryUrl:  account.EpayQueryUrl,
		EpayUrl:   account.EpayURL,
	}
	return EpayConfig, nil
}
