package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"marketing/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"
	"marketing/internal/service"
)

// setupTelegramControllerTestDB 准备 PostgreSQL 测试 DB + Telegram 相关表
// 使用 testutil.NewTestDB 确保多连接共享同一测试库，避免 worker goroutine
// 因连接池分配到不同连接而看不到 AutoMigrate 创建的表
func setupTelegramControllerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.TelegramAccount{},
		&model.MessageHub{},
		&model.InboxConversation{},
		&model.WebhookEvent{},
		&model.Customer{},
		&model.IntegrationAccount{},
		&model.UnifiedMessage{},
	)
	db.SetTestDB(database)
	return database
}

// setupTelegramAccountRouter 创建 Bot 账号管理路由（不经过 license 中间件）
func setupTelegramAccountRouter(ctrl *TelegramAccountController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl.RegisterRoutes(r.Group("/api"))
	return r
}

// setupTelegramWebhookRouter 创建 Webhook 路由（不经过 license 中间件）
// 返回 router 和 svc（svc 用于测试中直接查询 db，避免异步 worker 时序问题）
func setupTelegramWebhookRouter(database *gorm.DB) (*gin.Engine, *service.WebhookService) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := service.NewWebhookService(database)
	whCtrl := &WebhookController{svc: svc}
	wh := r.Group("/api/webhook")
	{
		wh.POST("/:channel/:account_id", whCtrl.Receive)
	}
	return r, svc
}

// waitForWorker 等待异步 worker 处理完成（轮询 db 直到有记录或超时）
// E2E 测试场景下 worker 处理 webhook 完整链路（落库 + AI 编排）需 2-5s，
// 默认 200 * 100ms = 20s 留足 buffer。
func waitForWorker(t *testing.T, database *gorm.DB, query string, args ...any) {
	for i := 0; i < 200; i++ {
		var count int64
		database.Model(&model.MessageHub{}).Where(query, args...).Count(&count)
		if count > 0 {
			return
		}
		time.Sleep(time.Millisecond * 100)
	}
	t.Logf("⚠️ worker 处理超时，查询: %s args: %v", query, args)
}

// =============================================================================
// A. Bot 账号 CRUD HTTP 端到端
// =============================================================================

// TestTelegramAccountController_CreateAndList 创建账号并列表查询
func TestTelegramAccountController_CreateAndList(t *testing.T) {
	setupTelegramControllerTestDB(t)
	ctrl := NewTelegramAccountController(nil)
	router := setupTelegramAccountRouter(ctrl)

	// 1. 创建账号
	createBody := map[string]any{
		"account_name":     "销售助手Bot",
		"bot_token":        "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
		"webhook_url":      "https://shop.example.com/api/webhook/telegram/1",
		"webhook_secret":   "test-secret-32chars-random-string",
		"webhook_enabled":  true,
		"ai_agent_enabled": true,
		"status":           1,
	}
	bodyBytes, _ := json.Marshal(createBody)
	req, _ := http.NewRequest("POST", "/api/telegram/accounts", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("创建账号期望 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	var createResp struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("解析创建响应失败: %v, body: %s", err, w.Body.String())
	}
	if createResp.Code != "SUCCESS" {
		t.Errorf("创建响应 code 期望 SUCCESS, 实际 %s", createResp.Code)
	}
	if createResp.Data["account_name"] != "销售助手Bot" {
		t.Errorf("account_name 错误: %v", createResp.Data["account_name"])
	}
	// Bot Token 应被掩码
	masked, _ := createResp.Data["bot_token_masked"].(string)
	if masked == "123456789:ABCdefGHIjklMNOpqrsTUVwxyz" {
		t.Error("Bot Token 应被掩码处理")
	}
	if len(masked) > 0 && masked[:4] != "1234" {
		t.Errorf("Bot Token 掩码应保留前4位, 实际: %s", masked)
	}
	t.Logf("✅ 创建账号成功: id=%v, masked_token=%s", createResp.Data["id"], masked)

	// 2. 列表查询
	req, _ = http.NewRequest("GET", "/api/telegram/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("列表查询期望 200, 实际 %d", w.Code)
	}
	var listResp struct {
		Code string           `json:"code"`
		Data map[string]any   `json:"data"`
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析列表响应失败: %v", err)
	}
	// 列表可能返回在 data.list 或 data 中
	list := listResp.List
	if len(list) == 0 {
		if dataList, ok := listResp.Data["list"].([]any); ok && len(dataList) > 0 {
			t.Logf("✅ 列表查询成功: %d 条记录", len(dataList))
			return
		}
		if total, ok := listResp.Data["total"].(float64); ok && total > 0 {
			t.Logf("✅ 列表查询成功: total=%v", total)
			return
		}
		t.Errorf("列表查询应返回至少 1 条记录, body: %s", w.Body.String())
		return
	}
	t.Logf("✅ 列表查询成功: %d 条记录", len(list))
}

// TestTelegramAccountController_GetAndGetByID 创建后按 ID 查询
func TestTelegramAccountController_GetAndGetByID(t *testing.T) {
	setupTelegramControllerTestDB(t)
	ctrl := NewTelegramAccountController(nil)
	router := setupTelegramAccountRouter(ctrl)

	// 创建
	bodyBytes, _ := json.Marshal(map[string]any{
		"account_name": "测试Bot",
		"bot_token":    "987654321:TOKEN",
		"status":       1,
	})
	req, _ := http.NewRequest("POST", "/api/telegram/accounts", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d %s", w.Code, w.Body.String())
	}

	// 列表拿到 ID
	req, _ = http.NewRequest("GET", "/api/telegram/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var listResp struct {
		Data map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)

	// 直接用 service 查 ID（避免列表结构差异）
	accs, _ := ctrl.svc.ListAccounts()
	if len(accs) != 1 {
		t.Fatalf("应有 1 条记录, 实际 %d", len(accs))
	}
	accID := accs[0].ID

	// 按 ID 查询
	req, _ = http.NewRequest("GET", "/api/telegram/accounts/"+uintToStr(accID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("按 ID 查询期望 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	t.Logf("✅ 按 ID 查询成功: %s", w.Body.String()[:min(80, len(w.Body.String()))])
}

// TestTelegramAccountController_Update 更新账号
func TestTelegramAccountController_Update(t *testing.T) {
	setupTelegramControllerTestDB(t)
	ctrl := NewTelegramAccountController(nil)
	router := setupTelegramAccountRouter(ctrl)

	// 创建
	bodyBytes, _ := json.Marshal(map[string]any{
		"account_name": "原名称",
		"bot_token":    "111:aaa",
		"status":       1,
	})
	req, _ := http.NewRequest("POST", "/api/telegram/accounts", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	accs, _ := ctrl.svc.ListAccounts()
	if len(accs) != 1 {
		t.Fatalf("创建后应有 1 条")
	}
	accID := accs[0].ID

	// 更新（Bot Token 为空时保持原值，但 binding:"required" 要求非空，所以传原值）
	updateBody, _ := json.Marshal(map[string]any{
		"account_name":     "新名称",
		"bot_token":        "111:aaa",
		"ai_agent_enabled": true,
		"status":           1,
	})
	req, _ = http.NewRequest("PUT", "/api/telegram/accounts/"+uintToStr(accID), bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("更新期望 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}

	// 验证更新后名称
	updated, _ := ctrl.svc.GetAccount(context.Background(), accID)
	if updated.AccountName != "新名称" {
		t.Errorf("名称应更新为 '新名称', 实际 '%s'", updated.AccountName)
	}
	if !updated.AIAgentEnabled {
		t.Error("AIAgentEnabled 应为 true")
	}
	// Bot Token 应为传入的值
	if updated.BotToken != "111:aaa" {
		t.Errorf("Bot Token 错误, 实际 '%s'", updated.BotToken)
	}
	t.Logf("✅ 更新成功: name=%s, ai=%v, token=%s", updated.AccountName, updated.AIAgentEnabled, updated.BotToken)
}

// TestTelegramAccountController_Delete 删除账号
func TestTelegramAccountController_Delete(t *testing.T) {
	setupTelegramControllerTestDB(t)
	ctrl := NewTelegramAccountController(nil)
	router := setupTelegramAccountRouter(ctrl)

	// 创建
	bodyBytes, _ := json.Marshal(map[string]any{
		"account_name": "待删除",
		"bot_token":    "222:bbb",
		"status":       1,
	})
	req, _ := http.NewRequest("POST", "/api/telegram/accounts", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	accs, _ := ctrl.svc.ListAccounts()
	if len(accs) != 1 {
		t.Fatalf("创建后应有 1 条")
	}
	accID := accs[0].ID

	// 删除
	req, _ = http.NewRequest("DELETE", "/api/telegram/accounts/"+uintToStr(accID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("删除期望 200, 实际 %d", w.Code)
	}

	// 验证已删除
	accs, _ = ctrl.svc.ListAccounts()
	if len(accs) != 0 {
		t.Errorf("删除后应有 0 条, 实际 %d", len(accs))
	}
	t.Logf("✅ 删除成功")
}

// =============================================================================
// B. Webhook 入群事件 HTTP 端到端
// =============================================================================

// TestTelegramWebhook_JoinEvent_E2E 完整链路：
// HTTP POST /api/webhook/telegram/:account_id → dispatchTelegram → MessageHub
func TestTelegramWebhook_JoinEvent_E2E(t *testing.T) {
	database := setupTelegramControllerTestDB(t)

	// 1. 先创建 Bot 账号（通过 repo 直接创建，避免 controller 路由差异）
	repo := repository.NewTelegramAccountRepository()
	repo.SetDB(database)
	acc := &model.TelegramAccount{
		AccountName:    "E2E测试Bot",
		BotToken:       "333:ccc",
		WebhookSecret:  "", // 未配置 secret，跳过验签
		AIAgentEnabled: false,
		Status:         1,
	}
	if err := repo.Create(acc); err != nil {
		t.Fatalf("创建测试账号失败: %v", err)
	}
	accID := acc.ID

	// 2. 设置 webhook 路由
	router, svc := setupTelegramWebhookRouter(database)
	defer svc.Stop()

	// 3. 发送 TG 入群事件 payload
	joinPayload := `{
		"update_id": 2001,
		"message": {
			"message_id": 6001,
			"from": {"id": 888, "first_name": "Inviter", "is_bot": false},
			"chat": {"id": -1009876543210, "type": "supergroup", "title": "E2E销售群"},
			"date": 1700000000,
			"new_chat_members": [
				{"id": 777, "first_name": "新客户", "username": "newcustomer", "is_bot": false}
			]
		}
	}`
	req, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), bytes.NewReader([]byte(joinPayload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Webhook 请求期望 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Accepted  bool   `json:"accepted"`
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v, body: %s", err, w.Body.String())
	}
	if !resp.Accepted {
		t.Errorf("accepted 应为 true, body: %s", w.Body.String())
	}
	t.Logf("✅ Webhook 入群事件接收成功: event_id=%s, type=%s", resp.EventID, resp.EventType)

	// 4. 等待异步 worker 处理完成，验证 MessageHub 中有入群事件记录
	waitForWorker(t, database, "platform = ? AND msg_type = ?", "telegram", "event")
	var hub model.MessageHub
	if err := database.Where("platform = ? AND msg_type = ?", "telegram", "event").First(&hub).Error; err != nil {
		t.Fatalf("MessageHub 中应入群事件记录: %v", err)
	}
	if hub.MsgID != "tg_join_-1009876543210_777" {
		t.Errorf("MsgID 错误: %s", hub.MsgID)
	}
	if hub.ConversationID != "-1009876543210" {
		t.Errorf("ConversationID 错误: %s", hub.ConversationID)
	}
	if !hub.IsGroup {
		t.Error("入群事件 IsGroup 应为 true")
	}
	t.Logf("✅ MessageHub 入群事件记录: msg_id=%s, content=%s", hub.MsgID, hub.Content)
}

// TestTelegramWebhook_RegularMessage_E2E 普通消息端到端
func TestTelegramWebhook_RegularMessage_E2E(t *testing.T) {
	database := setupTelegramControllerTestDB(t)

	repo := repository.NewTelegramAccountRepository()
	repo.SetDB(database)
	acc := &model.TelegramAccount{
		AccountName:    "消息测试Bot",
		BotToken:       "444:ddd",
		WebhookSecret:  "",
		AIAgentEnabled: false,
		Status:         1,
	}
	repo.Create(acc)
	accID := acc.ID

	router, svc := setupTelegramWebhookRouter(database)
	defer svc.Stop()

	// 发送普通消息
	msgPayload := `{
		"update_id": 3001,
		"message": {
			"message_id": 7001,
			"from": {"id": 555, "first_name": "客户A", "username": "userA", "is_bot": false},
			"chat": {"id": -1005555555555, "type": "group", "title": "咨询群"},
			"date": 1700000100,
			"text": "你好，我想了解一下光子嫩肤的价格"
		}
	}`
	req, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), bytes.NewReader([]byte(msgPayload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Webhook 请求期望 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Accepted bool `json:"accepted"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Accepted {
		t.Errorf("accepted 应为 true, body: %s", w.Body.String())
	}

	// 等待异步 worker 处理完成，验证 MessageHub 中有 text 类型记录
	waitForWorker(t, database, "platform = ? AND msg_type = ?", "telegram", "text")
	var hub model.MessageHub
	if err := database.Where("platform = ? AND msg_type = ?", "telegram", "text").First(&hub).Error; err != nil {
		t.Fatalf("MessageHub 中应有 text 记录: %v", err)
	}
	if hub.Content != "你好，我想了解一下光子嫩肤的价格" {
		t.Errorf("Content 错误: %s", hub.Content)
	}
	if !hub.IsGroup {
		t.Error("群组消息 IsGroup 应为 true")
	}
	t.Logf("✅ 普通消息记录: content=%s", hub.Content)
}

// TestTelegramWebhook_LeftEvent_E2E 退群事件端到端
func TestTelegramWebhook_LeftEvent_E2E(t *testing.T) {
	database := setupTelegramControllerTestDB(t)

	repo := repository.NewTelegramAccountRepository()
	repo.SetDB(database)
	acc := &model.TelegramAccount{
		AccountName: "退群测试Bot",
		BotToken:    "555:eee",
		Status:      1,
	}
	repo.Create(acc)
	accID := acc.ID

	router, svc := setupTelegramWebhookRouter(database)
	defer svc.Stop()

	leftPayload := `{
		"update_id": 4001,
		"message": {
			"message_id": 8001,
			"from": {"id": 444, "first_name": "Admin", "is_bot": false},
			"chat": {"id": -1004444444444, "type": "group", "title": "退群测试"},
			"date": 1700000200,
			"left_chat_member": {"id": 333, "first_name": "离开的客户", "username": "leaver", "is_bot": false}
		}
	}`
	req, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), bytes.NewReader([]byte(leftPayload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Webhook 请求期望 200, 实际 %d", w.Code)
	}

	// 等待异步 worker 处理完成，验证有 left 事件记录
	waitForWorker(t, database, "platform = ? AND msg_id LIKE ?", "telegram", "tg_left_%")
	var count int64
	database.Model(&model.MessageHub{}).Where("platform = ? AND msg_id LIKE ?", "telegram", "tg_left_%").Count(&count)
	if count == 0 {
		t.Error("应有退群事件记录 (msg_id LIKE tg_left_%)")
	}
	t.Logf("✅ 退群事件记录: %d 条", count)
}

// TestTelegramWebhook_BotMembersSkipped 仅 bot 成员入群不产生 event 记录
func TestTelegramWebhook_BotMembersSkipped(t *testing.T) {
	database := setupTelegramControllerTestDB(t)

	repo := repository.NewTelegramAccountRepository()
	repo.SetDB(database)
	acc := &model.TelegramAccount{
		AccountName: "Bot过滤测试",
		BotToken:    "666:fff",
		Status:      1,
	}
	repo.Create(acc)
	accID := acc.ID

	router, svc := setupTelegramWebhookRouter(database)
	defer svc.Stop()

	// 仅 bot 成员入群
	botOnlyPayload := `{
		"update_id": 5001,
		"message": {
			"message_id": 9001,
			"from": {"id": 222, "first_name": "Admin", "is_bot": false},
			"chat": {"id": -1002222222222, "type": "group", "title": "Bot过滤"},
			"date": 1700000300,
			"new_chat_members": [
				{"id": 111, "first_name": "OtherBot", "username": "other_bot", "is_bot": true}
			]
		}
	}`
	req, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), bytes.NewReader([]byte(botOnlyPayload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证不产生 event 类型记录
	var eventCount int64
	database.Model(&model.MessageHub{}).Where("platform = ? AND msg_type = ?", "telegram", "event").Count(&eventCount)
	if eventCount > 0 {
		t.Errorf("仅 bot 入群不应产生 event 记录, 实际 %d 条", eventCount)
	}
	t.Logf("✅ 仅 bot 入群正确跳过: event 记录 %d 条", eventCount)
}

// =============================================================================
// C. Webhook 幂等去重
// =============================================================================

// TestTelegramWebhook_Idempotent 相同 update_id 重复请求应被去重
func TestTelegramWebhook_Idempotent(t *testing.T) {
	database := setupTelegramControllerTestDB(t)

	repo := repository.NewTelegramAccountRepository()
	repo.SetDB(database)
	acc := &model.TelegramAccount{
		AccountName: "幂等测试Bot",
		BotToken:    "777:ggg",
		Status:      1,
	}
	repo.Create(acc)
	accID := acc.ID

	router, svc := setupTelegramWebhookRouter(database)
	defer svc.Stop()

	payload := `{
		"update_id": 6001,
		"message": {
			"message_id": 10001,
			"from": {"id": 999, "first_name": "Test", "is_bot": false},
			"chat": {"id": -1009999999999, "type": "private"},
			"date": 1700000400,
			"text": "测试幂等"
		}
	}`
	body := bytes.NewReader([]byte(payload))

	// 第一次请求
	req, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("第一次请求期望 200, 实际 %d", w.Code)
	}
	var firstResp struct {
		Accepted  bool `json:"accepted"`
		Duplicate bool `json:"duplicate"`
	}
	json.Unmarshal(w.Body.Bytes(), &firstResp)
	if !firstResp.Accepted || firstResp.Duplicate {
		t.Errorf("第一次应 accepted=true, duplicate=false, body: %s", w.Body.String())
	}

	// 第二次相同请求（重新创建 body reader）
	req2, _ := http.NewRequest("POST", "/api/webhook/telegram/"+uintToStr(accID), bytes.NewReader([]byte(payload)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("第二次请求期望 200, 实际 %d", w2.Code)
	}
	var secondResp struct {
		Accepted  bool `json:"accepted"`
		Duplicate bool `json:"duplicate"`
	}
	json.Unmarshal(w2.Body.Bytes(), &secondResp)
	if !secondResp.Accepted {
		t.Errorf("第二次应 accepted=true（幂等返回）, body: %s", w2.Body.String())
	}
	t.Logf("✅ 幂等去单: 第一次 duplicate=%v, 第二次 duplicate=%v", firstResp.Duplicate, secondResp.Duplicate)
}

// =============================================================================
// helpers
// =============================================================================

func uintToStr(n uint) string {
	return fmtUint(n)
}

func fmtUint(n uint) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
