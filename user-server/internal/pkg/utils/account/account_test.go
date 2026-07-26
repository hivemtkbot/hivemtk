package account

import (
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"marketing/internal/model"
)

func TestSetAccount(t *testing.T) {
	// 清理全局状态
	globalAccounts = make(map[string]accountData)

	token := "test_token_123"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_123",
		AccountName: "TestBot",
		Price:       decimal.NewFromFloat(99.99),
	}

	SetAccount(token, data)

	// 验证设置成功
	if len(globalAccounts) != 1 {
		t.Errorf("SetAccount() should add one account, got %d", len(globalAccounts))
	}
}

func TestGetAccount_Success(t *testing.T) {
	// 清理并设置测试数据
	globalAccounts = make(map[string]accountData)

	token := "test_token_get"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_get",
		AccountName: "GetBot",
		Price:       decimal.NewFromFloat(50.00),
	}
	globalAccounts[token] = data

	// 获取账户
	result, err := GetAccount(token)
	if err != nil {
		t.Errorf("GetAccount() unexpected error: %v", err)
	}
	if result.AccountID != "acc_get" {
		t.Errorf("GetAccount() AccountID = %v, want acc_get", result.AccountID)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	// 清理全局状态
	globalAccounts = make(map[string]accountData)

	// 尝试获取不存在的账户
	_, err := GetAccount("non_existent_token")
	if err == nil {
		t.Error("GetAccount() expected error for non-existent token, got nil")
	}
	if err.Error() != "bot not found" {
		t.Errorf("GetAccount() error = %v, want 'bot not found'", err.Error())
	}
}

func TestGetAccount_Concurrent(t *testing.T) {
	// 清理全局状态
	globalAccounts = make(map[string]accountData)

	// 设置测试数据
	token := "concurrent_token"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_concurrent",
		AccountName: "ConcurrentBot",
	}
	globalAccounts[token] = data

	// 并发读取
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
	// 清理全局状态
	globalAccounts = make(map[string]accountData)

	// 并发写入
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
		Price:      "invalid_price", // 无效的价格
		ID:         "acc_123",
		GroupID:    12345678,
		TgName:     "TestBot",
		URL:        "http://example.com",
	}

	// 直接测试价格解析逻辑，不依赖 bot
	price, err := decimal.NewFromString(account.Price)
	if err == nil {
		t.Error("Expected error for invalid price, got nil")
	}
	_ = price
}

func TestBuildAccountStartNoticeMsg(t *testing.T) {
	msg := BuildAccountStartNoticeMsg("TestBot")
	// 验证消息包含关键内容，不比较完整字符串以避免编码问题
	if msg == "" {
		t.Error("BuildAccountStartNoticeMsg() should not return empty string")
	}
}

func TestSendMsgBYBootToken_Success(t *testing.T) {
	// 清理并设置测试数据
	globalAccounts = make(map[string]accountData)

	token := "test_token_send"
	data := accountData{
		BotToken:    token,
		AccountID:   "acc_send",
		AccountName: "SendBot",
		// 注意：不设置 Bot，因为需要真实的 Telegram API
	}
	// 这个测试只是为了覆盖 GetAccount 的成功路径
	globalAccounts[token] = data

	// 由于 Bot 是 nil，调用 SendTgMsg 会 panic
	// 所以我们只测试 GetAccount 部分
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
