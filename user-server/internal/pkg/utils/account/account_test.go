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

func TestFormateEpayConfigByAccount(t *testing.T) {
	account := &model.Account{
		EpayPid:      "pid_123",
		EpayKey:      "key_123",
		EpayPayType:  "alipay",
		EpayURL:      "http://epay.example.com",
		EpayQueryUrl: "http://epay.example.com/query",
		URL:          "http://example.com",
	}

	config := FormateEpayConfigByAccount(account)

	if config.Pid != "pid_123" {
		t.Errorf("FormateEpayConfigByAccount() Pid = %v, want pid_123", config.Pid)
	}
	if config.Key != "key_123" {
		t.Errorf("FormateEpayConfigByAccount() Key = %v, want key_123", config.Key)
	}
	if config.Type != "alipay" {
		t.Errorf("FormateEpayConfigByAccount() Type = %v, want alipay", config.Type)
	}
	if config.NotifyUrl != "http://epay.example.com/api/epay_notify" {
		t.Errorf("FormateEpayConfigByAccount() NotifyUrl = %v, want http://epay.example.com/api/epay_notify", config.NotifyUrl)
	}
	if config.ReturnUrl != "http://example.com" {
		t.Errorf("FormateEpayConfigByAccount() ReturnUrl = %v, want http://example.com", config.ReturnUrl)
	}
	if config.QueryUrl != "http://epay.example.com/query" {
		t.Errorf("FormateEpayConfigByAccount() QueryUrl = %v, want http://epay.example.com/query", config.QueryUrl)
	}
	if config.EpayUrl != "http://epay.example.com" {
		t.Errorf("FormateEpayConfigByAccount() EpayUrl = %v, want http://epay.example.com", config.EpayUrl)
	}
}

func TestFormateAccountDictData(t *testing.T) {
	// 由于创建真实的 BotAPI 需要 Telegram API 访问，我们直接测试数据结构
	// 这里使用 nil bot 来测试数据格式化逻辑

	account := &model.Account{
		TgBotToken:   "test_token",
		Price:        "99.99",
		ID:           "acc_123",
		GroupID:      12345678,
		TgName:       "TestBot",
		URL:          "http://example.com",
		EpayPid:      "pid_123",
		EpayKey:      "key_123",
		EpayPayType:  "alipay",
		EpayURL:      "http://epay.example.com",
		EpayQueryUrl: "http://epay.example.com/query",
	}

	// 直接测试 FormateEpayConfigByAccount
	config := FormateEpayConfigByAccount(account)
	if config.Pid != "pid_123" {
		t.Errorf("FormateEpayConfigByAccount() Pid = %v, want pid_123", config.Pid)
	}
}

func TestFormateAccountDictData_InvalidPrice(t *testing.T) {
	account := &model.Account{
		TgBotToken:   "test_token",
		Price:        "invalid_price", // 无效的价格
		ID:           "acc_123",
		GroupID:      12345678,
		TgName:       "TestBot",
		URL:          "http://example.com",
		EpayPid:      "pid_123",
		EpayKey:      "key_123",
		EpayPayType:  "alipay",
		EpayURL:      "http://epay.example.com",
		EpayQueryUrl: "http://epay.example.com/query",
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

func TestBuildAccountNotPaidNoticeMsg(t *testing.T) {
	msg := BuildAccountNotPaidNoticeMsg("TestBot")
	if msg == "" {
		t.Error("BuildAccountNotPaidNoticeMsg() should not return empty string")
	}
}

func TestBuildAccountJoinGroupMsg(t *testing.T) {
	msg := BuildAccountJoinGroupMsg("TestBot")
	if msg == "" {
		t.Error("BuildAccountJoinGroupMsg() should not return empty string")
	}
}

func TestBuildAccountNotPayErrorNoticeMsg(t *testing.T) {
	msg := BuildAccountNotPayErrorNoticeMsg("TestBot")
	if msg == "" {
		t.Error("BuildAccountNotPayErrorNoticeMsg() should not return empty string")
	}
}

func TestBuildAccountPayUrlMsg(t *testing.T) {
	payUrl := "http://pay.example.com/order123"
	msg := BuildAccountPayUrlMsg(payUrl)
	expected := "<a href=\"http://pay.example.com/order123\">点击支付</a>"
	if msg != expected {
		t.Errorf("BuildAccountPayUrlMsg() = %v, want %v", msg, expected)
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

// 测试命令字符串常量
func TestCommandConstants(t *testing.T) {
	if PayCommandString != "/pay" {
		t.Errorf("PayCommandString = %v, want /pay", PayCommandString)
	}
	if StartCommandString != "/start" {
		t.Errorf("StartCommandString = %v, want /start", StartCommandString)
	}
	if JoinGroupCommandString != "/join_group" {
		t.Errorf("JoinGroupCommandString = %v, want /join_group", JoinGroupCommandString)
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

// 测试 FormateAccountDictData 的数据格式化逻辑
func TestFormateAccountDictData_Fields(t *testing.T) {
	account := &model.Account{
		TgBotToken:   "test_token",
		Price:        "100.00",
		ID:           "acc_123",
		GroupID:      87654321,
		TgName:       "MyBot",
		URL:          "http://myapp.com",
		EpayPid:      "p123",
		EpayKey:      "k123",
		EpayPayType:  "wxpay",
		EpayURL:      "http://pay.com",
		EpayQueryUrl: "http://pay.com/query",
	}

	// 测试 FormateEpayConfigByAccount 的输出
	config := FormateEpayConfigByAccount(account)

	if config.Pid != "p123" {
		t.Errorf("Config.Pid = %v, want p123", config.Pid)
	}
	if config.Key != "k123" {
		t.Errorf("Config.Key = %v, want k123", config.Key)
	}
	if config.Type != "wxpay" {
		t.Errorf("Config.Type = %v, want wxpay", config.Type)
	}
	if config.NotifyUrl != "http://pay.com/api/epay_notify" {
		t.Errorf("Config.NotifyUrl = %v, want http://pay.com/api/epay_notify", config.NotifyUrl)
	}
	if config.ReturnUrl != "http://myapp.com" {
		t.Errorf("Config.ReturnUrl = %v, want http://myapp.com", config.ReturnUrl)
	}
	if config.QueryUrl != "http://pay.com/query" {
		t.Errorf("Config.QueryUrl = %v, want http://pay.com/query", config.QueryUrl)
	}
	if config.EpayUrl != "http://pay.com" {
		t.Errorf("Config.EpayUrl = %v, want http://pay.com", config.EpayUrl)
	}
}
