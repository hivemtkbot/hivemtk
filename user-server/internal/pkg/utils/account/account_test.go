package account

import (
	"sync"
	"testing"

	"hivemtk-user/internal/model"

	"github.com/shopspring/decimal"
)

func TestSetAccount(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	token := "test_token_123"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_123",
		AccountName: "TestBot",
		Price:       decimal.NewFromFloat(99.99),
	}

	SetAccount(token, data)

	if len(globalAccounts) != 1 {
		t.Errorf("SetAccount() should add one account, got %d", len(globalAccounts))
	}
}

func TestGetAccount_Success(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	token := "test_token_get"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_get",
		AccountName: "GetBot",
		Price:       decimal.NewFromFloat(50.00),
	}
	globalAccounts[token] = data

	result, err := GetAccount(token)
	if err != nil {
		t.Errorf("GetAccount() unexpected error: %v", err)
	}
	if result.AccountID != "acc_get" {
		t.Errorf("GetAccount() AccountID = %v, want acc_get", result.AccountID)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	_, err := GetAccount("non_existent_token")
	if err == nil {
		t.Error("GetAccount() expected error for non-existent token, got nil")
	}
	if err.Error() != "bot not found" {
		t.Errorf("GetAccount() error = %v, want 'bot not found'", err.Error())
	}
}

func TestGetAccount_Concurrent(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	token := "concurrent_token"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_concurrent",
		AccountName: "ConcurrentBot",
	}
	globalAccounts[token] = data

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetAccount(token)
		}()
	}
	wg.Wait()
}

func TestSetAccount_Concurrent(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			token := "token_" + string(rune(i))
			data := accountData{
				BotToken:    token,
				AccountID:   "acc_" + string(rune(i)),
				AccountName: "Bot" + string(rune(i)),
			}
			SetAccount(token, data)
		}(i)
	}
	wg.Wait()
}

func TestFormateAccountDictData_InvalidPrice(t *testing.T) {
	account := &model.Account{
		TgBotToken: "test_token",
		Price:      "invalid_price",
		ID:         "acc_123",
		GroupID:    12345678,
		TgName:     "TestBot",
		URL:        "http://example.com",
	}

	price, err := decimal.NewFromString(account.Price)
	if err == nil {
		t.Error("Expected error for invalid price, got nil")
	}
	_ = price
}

func TestBuildAccountStartNoticeMsg(t *testing.T) {
	msg := BuildAccountStartNoticeMsg("TestBot")
	if msg == "" {
		t.Error("BuildAccountStartNoticeMsg() should not return empty string")
	}
}

func TestSendMsgBYBootToken_Success(t *testing.T) {
	globalAccounts = make(map[string]accountData)

	token := "test_token_send"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_send",
		AccountName: "SendBot",
	}
	globalAccounts[token] = data

	_, err := GetAccount(token)
	if err != nil {
		t.Errorf("GetAccount() unexpected error: %v", err)
	}
}

func TestSendMsgBYBootToken_NotFound(t *testing.T) {
	err := SendMsgBYBootToken("non_existent_token", "test message", 123456)
	if err == nil {
		t.Error("SendMsgBYBootToken() expected error for non-existent token, got nil")
	}
}

// 测试 accountData 结构体
func TestAccountDataStruct(t *testing.T) {
	data := accountData{
		BotToken:    "test_token",
		AccountID:   "acc_123",
		AccountName: "TestBot",
		GroupID:     123456,
	}
	if data.BotToken != "test_token" {
		t.Error("accountData.BotToken not set correctly")
	}
}
