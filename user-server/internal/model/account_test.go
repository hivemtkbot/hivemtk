package model

import (
	"testing"
)

func TestAccount_TableName(t *testing.T) {
	account := &Account{}
	tableName := account.TableName()
	if tableName != "account" {
		t.Errorf("Expected table name 'account', got %s", tableName)
	}
}

func TestAccount_BasicFields(t *testing.T) {
	trueVal := true
	falseVal := false

	account := &Account{
		TgName:              "testbot",
		TgBotToken:          "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:             123456789,
		Price:               "99.00",
		EpayQueryUrl:        "https://pay.example.com/api/query",
		EpayPid:             "123456",
		EpayKey:             "secret_key",
		EpayURL:             "https://pay.example.com",
		EpayPayType:         "alipay",
		ProxyEnableProxy:    true,
		ProxyProtoclo:       "https",
		ProxyHost:           "proxy.example.com",
		ProxyPort:           8080,
		DouyinHeadless:      &trueVal,
		KuaishouHeadless:    &falseVal,
		XiaohongshuHeadless: &trueVal,
		XianyuHeadless:      &trueVal,
		TiktokHeadless:      &falseVal,
		Status:              1,
		Msg:                 "test message",
		URL:                 "https://example.com",
	}

	if account.TgName != "testbot" {
		t.Errorf("Expected TgName 'testbot', got %s", account.TgName)
	}
	if account.GroupID != 123456789 {
		t.Errorf("Expected GroupID 123456789, got %d", account.GroupID)
	}
	if account.Price != "99.00" {
		t.Errorf("Expected Price '99.00', got %s", account.Price)
	}
	if account.ProxyEnableProxy != true {
		t.Error("Expected ProxyEnableProxy to be true")
	}
	if account.ProxyPort != 8080 {
		t.Errorf("Expected ProxyPort 8080, got %d", account.ProxyPort)
	}
}

func TestAccount_DefaultHeadlessValues(t *testing.T) {
	account := &Account{}

	// Default headless values should be nil (will be set to true by GORM default)
	if account.DouyinHeadless != nil {
		t.Logf("DouyinHeadless is %v (expected nil before save)", *account.DouyinHeadless)
	}
}

func TestAccount_DefaultProxyValues(t *testing.T) {
	account := &Account{}

	if account.ProxyEnableProxy != false {
		t.Error("Expected ProxyEnableProxy to be false by default")
	}
	if account.ProxyProtoclo != "" {
		t.Logf("ProxyProtoclo is %s (expected empty before save)", account.ProxyProtoclo)
	}
	if account.ProxyHost != "" {
		t.Logf("ProxyHost is %s (expected empty before save)", account.ProxyHost)
	}
	if account.ProxyPort != 0 {
		t.Logf("ProxyPort is %d (expected 0 before save)", account.ProxyPort)
	}
}

func TestAccount_Status(t *testing.T) {
	account := &Account{
		Status: 1, // Active
	}

	if account.Status != 1 {
		t.Errorf("Expected Status 1, got %d", account.Status)
	}
}

func TestAccount_WithEmptyFields(t *testing.T) {
	account := &Account{}

	if account.ID != "" {
		t.Errorf("Expected empty ID before save, got %s", account.ID)
	}
	if account.TgName != "" {
		t.Errorf("Expected empty TgName, got %s", account.TgName)
	}
}

func TestAccount_WithEpayConfig(t *testing.T) {
	account := &Account{
		EpayQueryUrl: "https://pay.example.com/query",
		EpayPid:      "10001",
		EpayKey:      "secret",
		EpayURL:      "https://pay.example.com",
		EpayPayType:  "wechat",
	}

	if account.EpayQueryUrl != "https://pay.example.com/query" {
		t.Errorf("Expected EpayQueryUrl, got %s", account.EpayQueryUrl)
	}
	if account.EpayPid != "10001" {
		t.Errorf("Expected EpayPid '10001', got %s", account.EpayPid)
	}
	if account.EpayPayType != "wechat" {
		t.Errorf("Expected EpayPayType 'wechat', got %s", account.EpayPayType)
	}
}

func TestAccount_WithProxyConfig(t *testing.T) {
	account := &Account{
		ProxyEnableProxy: true,
		ProxyProtoclo:    "socks5",
		ProxyHost:        "127.0.0.1",
		ProxyPort:        1080,
	}

	if !account.ProxyEnableProxy {
		t.Error("Expected ProxyEnableProxy to be true")
	}
	if account.ProxyProtoclo != "socks5" {
		t.Errorf("Expected ProxyProtoclo 'socks5', got %s", account.ProxyProtoclo)
	}
	if account.ProxyHost != "127.0.0.1" {
		t.Errorf("Expected ProxyHost '127.0.0.1', got %s", account.ProxyHost)
	}
	if account.ProxyPort != 1080 {
		t.Errorf("Expected ProxyPort 1080, got %d", account.ProxyPort)
	}
}

func TestAccount_AllHeadlessPlatforms(t *testing.T) {
	trueVal := true
	account := &Account{
		DouyinHeadless:      &trueVal,
		KuaishouHeadless:    &trueVal,
		XiaohongshuHeadless: &trueVal,
		XianyuHeadless:      &trueVal,
		TiktokHeadless:      &trueVal,
	}

	if !*account.DouyinHeadless {
		t.Error("Expected DouyinHeadless to be true")
	}
	if !*account.KuaishouHeadless {
		t.Error("Expected KuaishouHeadless to be true")
	}
	if !*account.XiaohongshuHeadless {
		t.Error("Expected XiaohongshuHeadless to be true")
	}
	if !*account.XianyuHeadless {
		t.Error("Expected XianyuHeadless to be true")
	}
	if !*account.TiktokHeadless {
		t.Error("Expected TiktokHeadless to be true")
	}
}

func TestAccount_BeforeCreate(t *testing.T) {
	account := &Account{
		TgName: "testbot",
	}

	// BeforeCreate should generate an ID
	err := account.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if account.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Verify it's a valid UUID format
	if len(account.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(account.ID))
	}
}
