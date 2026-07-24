package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/httpclient"

	"gorm.io/gorm"
)

// setupFeishuTestDB 准备飞书服务测试库（按需迁移，与 e2e 测试一致）
func setupFeishuTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.FeishuAccount{},
		&model.FeishuMessage{},
		&model.FeishuCustomer{},
		&model.MessageHub{},
		&model.InboxConversation{},
	)
	db.SetTestDB(database)
	return database
}

// feishuMockTransport 拦截飞书开放平台 HTTP 请求，返回成功响应
type feishuMockTransport struct {
	code int // 用于错误用例：非 0 表示 API 业务错误
}

func (m feishuMockTransport) RoundTrip(ctx context.Context, r *http.Request) (*http.Response, error) {
	var body []byte
	status := http.StatusOK
	switch r.URL.Path {
	case "/open-apis/im/v1/messages":
		if m.code == 0 {
			body = []byte(`{"code":0,"msg":"success","data":{"message_id":"om_test_123","msg_id":"om_test_123"}}`)
		} else {
			// 飞书仅以 HTTP 状态码判定成败，业务 code!=0 时返回 500 触发错误分支
			body = []byte(`{"code":9999,"msg":"api error","data":{}}`)
			status = http.StatusInternalServerError
		}
	case "/open-apis/auth/v3/tenant_access_token/internal":
		body = []byte(`{"code":0,"msg":"ok","tenant_access_token":"fake_token","expire":7200}`)
	default:
		body = []byte(`{"code":0,"msg":"ok"}`)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func newFeishuTestAccount(t *testing.T, database *gorm.DB) *model.FeishuAccount {
	t.Helper()
	exp := time.Now().Add(time.Hour)
	acc := &model.FeishuAccount{
		AccountName:  "test-feishu",
		AppID:        "cli_test_app",
		AppSecret:    "test_secret",
		AccessToken:  "valid-token",
		TokenExpires: &exp,
		Status:       1,
	}
	if err := database.Create(acc).Error; err != nil {
		t.Fatalf("create feishu account: %v", err)
	}
	return acc
}

// TestFeishuIntegrationService_SendMessage 主动发消息（语法 + 行为测试）
func TestFeishuIntegrationService_SendMessage(t *testing.T) {
	database := setupFeishuTestDB(t)
	svc := NewFeishuIntegrationService(database)
	acc := newFeishuTestAccount(t, database)

	// 拦截飞书 HTTP，避免真实外网调用
	orig := httpclient.Client
	httpclient.Client = &http.Client{Transport: feishuMockTransport{}}
	defer func() { httpclient.Client = orig }()

	if err := svc.SendMessage(context.Background(), acc.ID, "ou_abc123", "你好，飞书"); err != nil {
		t.Fatalf("SendMessage 失败: %v", err)
	}

	// 断言：飞书消息落库且拿到平台 MsgID
	var fm model.FeishuMessage
	if err := database.Where("account_id = ?", acc.ID).First(&fm).Error; err != nil {
		t.Fatalf("未找到飞书出站消息: %v", err)
	}
	if !strings.HasPrefix(fm.MsgID, "feishu-out-") {
		t.Errorf("期望 MsgID 以 feishu-out- 开头, 实际=%q", fm.MsgID)
	}

	// 断言：消息中台出站记录落库（主动发）
	var hub model.MessageHub
	if err := database.Where("account_id = ? AND direction = ?", uintToStr(acc.ID), "outbound").First(&hub).Error; err != nil {
		t.Fatalf("未找到消息中台出站记录: %v", err)
	}
	if hub.Content != "你好，飞书" {
		t.Errorf("期望中台内容=你好，飞书, 实际=%q", hub.Content)
	}
}

// TestFeishuIntegrationService_SendMessage_APIError 主动发消息 API 返回业务错误
func TestFeishuIntegrationService_SendMessage_APIError(t *testing.T) {
	database := setupFeishuTestDB(t)
	svc := NewFeishuIntegrationService(database)
	acc := newFeishuTestAccount(t, database)

	orig := httpclient.Client
	httpclient.Client = &http.Client{Transport: feishuMockTransport{code: 9999}}
	defer func() { httpclient.Client = orig }()

	err := svc.SendMessage(context.Background(), acc.ID, "ou_abc123", "你好")
	if err == nil {
		t.Fatal("期望 SendMessage 返回错误，实际为 nil")
	}

	// 断言：账号记录最后错误
	var updated model.FeishuAccount
	if err := database.First(&updated, acc.ID).Error; err != nil {
		t.Fatalf("读取账号失败: %v", err)
	}
	if updated.LastErrorMsg == "" {
		t.Error("期望账号 LastErrorMsg 被记录")
	}
}

// TestFeishuIntegrationService_IngestMessage 被动收消息（语法 + 行为测试）
func TestFeishuIntegrationService_IngestMessage(t *testing.T) {
	database := setupFeishuTestDB(t)
	svc := NewFeishuIntegrationService(database)
	acc := newFeishuTestAccount(t, database)

	hub, conv, err := svc.IngestMessage(context.Background(), &FeishuIngestRequest{
		AccountID: acc.ID,
		OpenID:    "ou_sender",
		Name:      "张三",
		MsgType:   "text",
		Content:   "hi",
		ChatType:  "p2p",
	})
	if err != nil {
		t.Fatalf("IngestMessage 失败: %v", err)
	}
	if hub == nil {
		t.Fatal("期望返回非空 MessageHub")
	}
	if hub.SenderID != "ou_sender" || hub.SenderName != "张三" {
		t.Errorf("期望 sender=ou_sender/张三, 实际=%q/%q", hub.SenderID, hub.SenderName)
	}
	if conv == nil {
		t.Fatal("期望返回非空 InboxConversation")
	}
}

func uintToStr(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
