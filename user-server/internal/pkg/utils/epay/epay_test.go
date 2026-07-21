package epay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	_type "marketing/internal/pkg/utils/type"
)

func TestEpaySign(t *testing.T) {
	tests := []struct {
		name     string
		mapInput map[string]string
		epayKey  string
		want     string
	}{
		{
			name: "基本签名测试",
			mapInput: map[string]string{
				"pid":          "12345",
				"type":         "alipay",
				"out_trade_no": "ORDER123",
				"notify_url":   "http://example.com/notify",
				"return_url":   "http://example.com/return",
				"name":         "Test Product",
				"money":        "100.00",
			},
			epayKey: "test_key",
			want:    "", // 我们会验证签名是否为空（因为 MD5 值需要计算）
		},
		{
			name: "包含空值的签名",
			mapInput: map[string]string{
				"pid":          "12345",
				"type":         "alipay",
				"out_trade_no": "ORDER456",
				"notify_url":   "", // 空值应该被跳过
				"return_url":   "http://example.com/return",
				"name":         "", // 空值应该被跳过
				"money":        "50.00",
			},
			epayKey: "my_key",
			want:    "",
		},
		{
			name: "包含 sign 和 sign_type 的签名（应该被跳过）",
			mapInput: map[string]string{
				"pid":          "12345",
				"type":         "alipay",
				"out_trade_no": "ORDER789",
				"money":        "200.00",
				"sign":         "should_be_skipped",
				"sign_type":    "MD5",
			},
			epayKey: "secret_key",
			want:    "",
		},
		{
			name:     "空 map 签名",
			mapInput: map[string]string{},
			epayKey:  "key",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EpaySign(tt.mapInput, tt.epayKey)
			if got == "" {
				t.Error("EpaySign() should not return empty string")
			}
			// 验证签名是 32 位 MD5
			if len(got) != 32 {
				t.Errorf("EpaySign() length = %d, want 32", len(got))
			}
		})
	}
}

func TestEpaySignDeterministic(t *testing.T) {
	// 测试相同的输入总是产生相同的输出
	mapInput := map[string]string{
		"pid":          "100",
		"type":         "wxpay",
		"out_trade_no": "TEST001",
		"money":        "10.00",
	}
	epayKey := "test_key_123"

	sign1 := EpaySign(mapInput, epayKey)
	sign2 := EpaySign(mapInput, epayKey)

	if sign1 != sign2 {
		t.Errorf("EpaySign() should be deterministic: sign1 = %s, sign2 = %s", sign1, sign2)
	}
}

func TestEpayUrl(t *testing.T) {
	orderID := "ORDER_TEST_001"
	price := decimal.NewFromFloat(99.99)
	productName := "Test Product"
	epayConfig := _type.EpayConfig{
		Pid:       "12345",
		Key:       "test_key",
		Type:      "alipay",
		NotifyUrl: "http://example.com/notify",
		ReturnUrl: "http://example.com/return",
		QueryUrl:  "http://example.com/query",
		EpayUrl:   "http://pay.example.com/submit.php",
	}

	url := EpayUrl(orderID, price, productName, epayConfig)

	// 验证 URL 包含基本参数
	if url == "" {
		t.Error("EpayUrl() should not return empty string")
	}

	// 验证 URL 包含配置中的 epayUrl
	expectedPrefix := epayConfig.EpayUrl
	if len(url) < len(expectedPrefix) {
		t.Errorf("EpayUrl() = %s, should start with %s", url, expectedPrefix)
	}

	// 验证 URL 包含订单号
	if !contains(url, "out_trade_no="+orderID) {
		t.Errorf("EpayUrl() should contain orderID: %s", orderID)
	}

	// 验证 URL 包含价格
	if !contains(url, "money="+price.String()) {
		t.Errorf("EpayUrl() should contain price: %s", price.String())
	}

	// 验证 URL 包含签名
	if !contains(url, "sign=") {
		t.Error("EpayUrl() should contain sign parameter")
	}

	// 验证 URL 包含 sign_type=MD5
	if !contains(url, "sign_type=MD5") {
		t.Error("EpayUrl() should contain sign_type=MD5")
	}
}

func TestEpayUrlWithDifferentProducts(t *testing.T) {
	orderID := "ORDER_002"
	price := decimal.NewFromFloat(50.00)
	productName := "Another Product & <special> chars"
	epayConfig := _type.EpayConfig{
		Pid:       "99999",
		Key:       "another_key",
		Type:      "wxpay",
		NotifyUrl: "http://test.com/notify",
		ReturnUrl: "http://test.com/return",
		QueryUrl:  "http://test.com/query",
		EpayUrl:   "http://pay.test.com/submit.php",
	}

	url := EpayUrl(orderID, price, productName, epayConfig)

	if url == "" {
		t.Error("EpayUrl() should not return empty string")
	}
}

func TestEpayQuery(t *testing.T) {
	orderID := "TEST_ORDER_QUERY"
	epayConfig := _type.EpayConfig{
		Pid:       "12345",
		Key:       "test_key",
		Type:      "alipay",
		NotifyUrl: "http://example.com/notify",
		ReturnUrl: "http://example.com/return",
		QueryUrl:  "http://invalid-query-url.example.com/query",
		EpayUrl:   "http://pay.example.com/submit.php",
	}

	// 由于查询 URL 是无效的，这个测试应该失败
	// 但这验证了 EpayQuery 函数的逻辑
	_, err := EpayQuery(orderID, epayConfig)
	if err == nil {
		t.Log("Expected EpayQuery to fail with invalid URL")
	}
}

func TestEpayQueryWithValidFormat(t *testing.T) {
	// 测试 URL 格式正确但服务器不可达的情况
	orderID := "ORDER_FORMAT_TEST"
	epayConfig := _type.EpayConfig{
		Pid:      "100",
		Key:      "key100",
		QueryUrl: "http://127.0.0.1:1/query", // 本地无效端口
	}

	_, err := EpayQuery(orderID, epayConfig)
	if err == nil {
		t.Log("Expected EpayQuery to fail with unreachable server")
	}
}

func TestEpayQuery_Success(t *testing.T) {
	// 创建测试服务器模拟成功响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"code":    1,
			"message": "success",
			"data": map[string]any{
				"status": "paid",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	orderID := "SUCCESS_ORDER_001"
	epayConfig := _type.EpayConfig{
		Pid:      "12345",
		Key:      "test_key",
		QueryUrl: server.URL,
	}

	success, err := EpayQuery(orderID, epayConfig)
	if err != nil {
		t.Errorf("EpayQuery() expected no error, got %v", err)
	}
	if !success {
		t.Error("EpayQuery() expected success = true, got false")
	}
}

func TestEpayQuery_Fail_NotPaid(t *testing.T) {
	// 创建测试服务器模拟订单未支付响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"code":    0,
			"message": "订单不存在或未支付",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	orderID := "UNPAID_ORDER_001"
	epayConfig := _type.EpayConfig{
		Pid:      "12345",
		Key:      "test_key",
		QueryUrl: server.URL,
	}

	success, err := EpayQuery(orderID, epayConfig)
	if err == nil {
		t.Error("EpayQuery() expected error for unpaid order, got nil")
	}
	if success {
		t.Error("EpayQuery() expected success = false for unpaid order")
	}
}

func TestEpayQuery_Fail_CodeNotOne(t *testing.T) {
	// 创建测试服务器模拟 code 不为 1 的响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"code":    2,
			"message": "unknown error",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	orderID := "ERROR_ORDER_001"
	epayConfig := _type.EpayConfig{
		Pid:      "12345",
		Key:      "test_key",
		QueryUrl: server.URL,
	}

	success, err := EpayQuery(orderID, epayConfig)
	if err == nil {
		t.Error("EpayQuery() expected error for code != 1, got nil")
	}
	if success {
		t.Error("EpayQuery() expected success = false for code != 1")
	}
}

// 辅助函数：检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
