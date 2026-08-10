package service

// anomaly_login_detector_test.go A 域 异常登录预警服务测试
//
// 测试目标（5+ 核心场景）：
//  1. NewAnomalyLoginDetector / NewAnomalyLoginDetectorWithConfig - 构造
//  2. DefaultAnomalyLoginDetectorConfig - 默认配置合理性
//  3. DetectAndAlert - nil context 报错
//  4. DetectAndAlert - 正常登录（无风险）→ 不应告警
//  5. DetectAndAlert - 凌晨异常时段登录 → 风险 medium
//  6. DetectAndAlert - 异地登录 → 风险 high
//  7. DetectAndAlert - 频繁失败 → 风险 high
//  8. SetAdminEmails / SetConfig - 配置修改
//  9. ListAlerts / ListLoginEvents - 分页查询

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// ensure gorm is referenced to avoid unused import in this file
var _ gorm.DB

// setupAnomalyLoginTestDB 准备异常登录测试库
func setupAnomalyLoginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.SystemUser{},
		&model.LoginEvent{},
		&model.SecurityAlert{},
		&model.Notification{},
		&model.OperationLog{},
		&model.EmailSend{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewAnomalyLoginDetector 测试构造函数
func TestNewAnomalyLoginDetector(t *testing.T) {
	d := NewAnomalyLoginDetector()
	if d == nil {
		t.Fatal("NewAnomalyLoginDetector returned nil")
	}
	if d.riskService == nil {
		t.Error("riskService 未初始化")
	}
	if !d.config.AuditEnabled {
		t.Error("默认应启用审计")
	}
	if !d.config.InboxEnabled {
		t.Error("默认应启用站内信")
	}
}

// TestNewAnomalyLoginDetectorWithConfig 测试自定义配置
func TestNewAnomalyLoginDetectorWithConfig(t *testing.T) {
	cfg := AnomalyLoginDetectorConfig{
		EmailEnabled:         true,
		AuditEnabled:         false,
		InboxEnabled:         true,
		AdminEmails:          []string{"admin@example.com"},
		EmailSubjectTemplate: "[%s] Test - %s",
		EmailBodyTemplate:    "Test body %s",
	}
	d := NewAnomalyLoginDetectorWithConfig(cfg)
	if d.config.AuditEnabled {
		t.Error("AuditEnabled 应为 false")
	}
	if !d.config.EmailEnabled {
		t.Error("EmailEnabled 应为 true")
	}
	if len(d.config.AdminEmails) != 1 || d.config.AdminEmails[0] != "admin@example.com" {
		t.Errorf("AdminEmails 配置错误: %v", d.config.AdminEmails)
	}
}

// TestDefaultAnomalyLoginDetectorConfig 测试默认配置
func TestDefaultAnomalyLoginDetectorConfig(t *testing.T) {
	cfg := DefaultAnomalyLoginDetectorConfig()
	if !cfg.AuditEnabled {
		t.Error("默认应启用审计")
	}
	if !cfg.InboxEnabled {
		t.Error("默认应启用站内信")
	}
	if cfg.EmailSubjectTemplate == "" {
		t.Error("EmailSubjectTemplate 不应为空")
	}
	if cfg.EmailBodyTemplate == "" {
		t.Error("EmailBodyTemplate 不应为空")
	}
}

// TestSetAdminEmails 测试管理员邮箱设置
func TestSetAdminEmails(t *testing.T) {
	d := NewAnomalyLoginDetector()
	emails := []string{"a@x.com", "b@y.com"}
	d.SetAdminEmails(context.Background(), emails)
	if len(d.config.AdminEmails) != 2 {
		t.Errorf("AdminEmails 长度 = %d, want 2", len(d.config.AdminEmails))
	}
}

// TestSetConfig 测试配置整体替换
func TestSetConfig(t *testing.T) {
	d := NewAnomalyLoginDetector()
	newCfg := AnomalyLoginDetectorConfig{
		EmailEnabled:         false,
		AuditEnabled:         false,
		InboxEnabled:         false,
		EmailSubjectTemplate: "X",
		EmailBodyTemplate:    "Y",
	}
	d.SetConfig(context.Background(), newCfg)
	if d.config.EmailEnabled {
		t.Error("EmailEnabled 应为 false")
	}
}

// TestDetectAndAlert_NilContext 测试 nil 上下文
func TestDetectAndAlert_NilContext(t *testing.T) {
	d := NewAnomalyLoginDetector()
	_, err := d.DetectAndAlert(context.Background(), nil)
	if err == nil {
		t.Fatal("nil context 应返回错误")
	}
}

// TestDetectAndAlert_NormalLogin 测试正常登录（白天 IP 一致）
func TestDetectAndAlert_NormalLogin(t *testing.T) {
	setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	// 中午 12 点登录（不在异常时段）
	lctx := &LoginRiskContext{
		UserID:    1,
		Username:  "alice",
		IP:        "192.168.1.100",
		UserAgent: "Mozilla/5.0",
		Success:   true,
		LoginAt:   time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	// 正常登录 ShouldAlert=false
	if result.ShouldForceMFA {
		t.Error("正常登录不应强制 MFA")
	}
	if result.RiskLevel != model.RiskLevelLow {
		t.Errorf("风险等级 = %s, want low", result.RiskLevel)
	}
	// 不应触发任何告警通道
	if len(result.ChannelsSent) > 0 {
		t.Errorf("正常登录不应触发告警通道: %v", result.ChannelsSent)
	}

	// login_event 应已写入
	if result.LoginEventID == 0 {
		t.Error("LoginEventID 应被赋值")
	}
}

// TestDetectAndAlert_AbnormalHour 测试凌晨异常时段
func TestDetectAndAlert_AbnormalHour(t *testing.T) {
	setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	// 凌晨 3 点登录
	lctx := &LoginRiskContext{
		UserID:    1,
		Username:  "bob",
		IP:        "192.168.1.100",
		UserAgent: "Mozilla/5.0",
		Success:   true,
		LoginAt:   time.Date(2026, 7, 21, 3, 30, 0, 0, time.UTC),
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}

	// 凌晨 3 点应至少是 medium 风险
	if result.RiskLevel == model.RiskLevelLow {
		t.Error("凌晨 3 点登录应至少为 medium 风险")
	}
	// 但可能不告警（medium 不一定 ShouldAlert）
}

// TestDetectAndAlert_FrequentFailure 测试频繁失败（高风险）
func TestDetectAndAlert_FrequentFailure(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	// 预写入 5 条失败事件（1 小时内）
	now := time.Now()
	for i := 0; i < 5; i++ {
		event := &model.LoginEvent{
			UserID:    99,
			Username:  "victim",
			IP:        "10.0.0.1",
			LoginAt:   now.Add(-time.Duration(i*5) * time.Minute),
			Success:   false,
			RiskLevel: model.RiskLevelLow,
		}
		database.Create(event)
	}

	// 第 6 次失败
	lctx := &LoginRiskContext{
		UserID:    99,
		Username:  "victim",
		IP:        "10.0.0.1",
		UserAgent: "attacker",
		Success:   false,
		LoginAt:   now,
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}

	// 频繁失败应升级为 high
	if result.RiskLevel != model.RiskLevelHigh && result.RiskLevel != model.RiskLevelCritical {
		t.Errorf("频繁失败风险等级 = %s, want high/critical", result.RiskLevel)
	}
	// 频繁失败应触发告警
	if !result.ShouldForceMFA {
		t.Error("频繁失败应强制 MFA")
	}
	// 应至少有审计 + 站内信两个通道成功
	if !result.AuditLogged {
		t.Error("审计日志应记录")
	}
	if !result.InboxCreated {
		t.Error("站内信应发送")
	}
	// 失败通道列表
	if len(result.ChannelsFailed) > 0 {
		t.Errorf("失败通道: %v", result.ChannelsFailed)
	}
}

// TestDetectAndAlert_AbnormalLogin 测试异地登录
func TestDetectAndAlert_AbnormalLogin(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	// 预写一条成功登录（北京地区）
	prev := &model.LoginEvent{
		UserID:    50,
		Username:  "traveler",
		IP:        "1.1.1.1",
		Location:  "geo(40.0,116.0)", // 北京
		LoginAt:   time.Now().Add(-1 * time.Hour),
		Success:   true,
		RiskLevel: model.RiskLevelLow,
	}
	database.Create(prev)

	// 异地登录（用差异巨大的 IP）
	lctx := &LoginRiskContext{
		UserID:    50,
		Username:  "traveler",
		IP:        "200.200.200.200", // 完全不同的 IP 段
		UserAgent: "Mozilla/5.0",
		Success:   true,
		LoginAt:   time.Now(),
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}

	// 异地登录至少是 high
	if result.RiskLevel == model.RiskLevelLow {
		t.Error("异地登录风险等级不应为 low")
	}
	if !result.ShouldForceMFA {
		t.Error("异地登录应强制 MFA")
	}
}

// TestDetectAndAlert_EmailChannel 测试邮件通道
func TestDetectAndAlert_EmailChannel(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	cfg := DefaultAnomalyLoginDetectorConfig()
	cfg.AdminEmails = []string{"admin@example.com", "security@example.com"}
	d := NewAnomalyLoginDetectorWithConfig(cfg)

	// 写入 5 条失败
	now := time.Now()
	for i := 0; i < 5; i++ {
		event := &model.LoginEvent{
			UserID:    100,
			Username:  "emailtest",
			IP:        "10.0.0.1",
			LoginAt:   now.Add(-time.Duration(i*5) * time.Minute),
			Success:   false,
			RiskLevel: model.RiskLevelLow,
		}
		database.Create(event)
	}

	lctx := &LoginRiskContext{
		UserID:    100,
		Username:  "emailtest",
		IP:        "10.0.0.1",
		UserAgent: "attacker",
		Success:   false,
		LoginAt:   now,
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}

	// 应触发邮件
	if !result.EmailDispatched {
		t.Error("邮件应被派发")
	}

	// 验证 email_sends 表有记录
	var emails []model.EmailSend
	if err := database.Find(&emails).Error; err != nil {
		t.Fatalf("查询邮件失败: %v", err)
	}
	if len(emails) == 0 {
		t.Error("应有 1 封邮件记录")
	} else {
		if emails[0].To == "" {
			t.Error("邮件 To 字段不应为空")
		}
		if emails[0].Subject == "" {
			t.Error("邮件 Subject 不应为空")
		}
		if !containsStr(emails[0].To, "admin@example.com") {
			t.Errorf("收件人应包含 admin@example.com: %s", emails[0].To)
		}
	}
}

// TestDetectAndAlert_NoEmailWhenDisabled 测试禁用邮件时不发
func TestDetectAndAlert_NoEmailWhenDisabled(t *testing.T) {
	setupAnomalyLoginTestDB(t)
	cfg := DefaultAnomalyLoginDetectorConfig()
	cfg.EmailEnabled = false
	cfg.AdminEmails = []string{"admin@example.com"}
	d := NewAnomalyLoginDetectorWithConfig(cfg)

	// 强制触发告警
	database := db.GetDB()
	now := time.Now()
	for i := 0; i < 5; i++ {
		event := &model.LoginEvent{
			UserID:    200,
			Username:  "no_email",
			IP:        "10.0.0.1",
			LoginAt:   now.Add(-time.Duration(i*5) * time.Minute),
			Success:   false,
			RiskLevel: model.RiskLevelLow,
		}
		database.Create(event)
	}

	lctx := &LoginRiskContext{
		UserID:   200,
		Username: "no_email",
		IP:       "10.0.0.1",
		Success:  false,
		LoginAt:  now,
	}

	result, err := d.DetectAndAlert(context.Background(), lctx)
	if err != nil {
		t.Fatalf("DetectAndAlert 失败: %v", err)
	}

	if result.EmailDispatched {
		t.Error("EmailEnabled=false 时不应发送邮件")
	}
}

// TestListAlerts 测试告警列表
func TestListAlerts(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	// 插入 3 条告警
	for i := 0; i < 3; i++ {
		alert := &model.SecurityAlert{
			UserID:      1,
			Username:    "u",
			AlertType:   model.AlertTypeAbnormalLogin,
			RiskLevel:   model.RiskLevelHigh,
			Title:       "test",
			Description: "desc",
			IP:          "1.1.1.1",
			Status:      model.SecurityAlertStatusOpen,
		}
		database.Create(alert)
	}

	alerts, total, err := d.ListAlerts(context.Background(), 1, "", 1, 10)
	if err != nil {
		t.Fatalf("ListAlerts 失败: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(alerts) != 3 {
		t.Errorf("alerts 长度 = %d, want 3", len(alerts))
	}
}

// TestListAlerts_FilterByStatus 测试按状态过滤
func TestListAlerts_FilterByStatus(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	database.Create(&model.SecurityAlert{
		UserID: 1, Username: "u", AlertType: model.AlertTypeAbnormalLogin,
		RiskLevel: model.RiskLevelHigh, Title: "open", Status: model.SecurityAlertStatusOpen,
	})
	database.Create(&model.SecurityAlert{
		UserID: 1, Username: "u", AlertType: model.AlertTypeAbnormalLogin,
		RiskLevel: model.RiskLevelHigh, Title: "resolved", Status: model.SecurityAlertStatusResolved,
	})

	open, total, err := d.ListAlerts(context.Background(), 1, "open", 1, 10)
	if err != nil {
		t.Fatalf("ListAlerts 失败: %v", err)
	}
	if total != 1 {
		t.Errorf("open total = %d, want 1", total)
	}
	if len(open) != 1 {
		t.Errorf("open 长度 = %d, want 1", len(open))
	}
	if open[0].Title != "open" {
		t.Errorf("过滤结果错误: %s", open[0].Title)
	}
}

// TestListLoginEvents 测试登录事件列表
func TestListLoginEvents(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	for i := 0; i < 5; i++ {
		database.Create(&model.LoginEvent{
			UserID:    1,
			Username:  "u",
			IP:        "1.1.1.1",
			LoginAt:   time.Now(),
			Success:   true,
			RiskLevel: model.RiskLevelLow,
		})
	}

	events, total, err := d.ListLoginEvents(context.Background(), 1, 1, 10)
	if err != nil {
		t.Fatalf("ListLoginEvents 失败: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(events) != 5 {
		t.Errorf("events 长度 = %d, want 5", len(events))
	}
}

// TestResolveAlert 测试解决告警
func TestResolveAlert(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	alert := &model.SecurityAlert{
		UserID:    1,
		Username:  "u",
		AlertType: model.AlertTypeAbnormalLogin,
		RiskLevel: model.RiskLevelHigh,
		Title:     "to_resolve",
		Status:    model.SecurityAlertStatusOpen,
	}
	database.Create(alert)

	err := d.ResolveAlert(context.Background(), alert.ID, 1, "handled by admin")
	if err != nil {
		t.Fatalf("ResolveAlert 失败: %v", err)
	}

	// 重新查询
	var resolved model.SecurityAlert
	database.First(&resolved, alert.ID)
	if resolved.Status != model.SecurityAlertStatusResolved {
		t.Errorf("状态 = %s, want resolved", resolved.Status)
	}
	if resolved.ResolveNote != "handled by admin" {
		t.Errorf("note = %s, want handled by admin", resolved.ResolveNote)
	}
	if resolved.ResolvedBy != 1 {
		t.Errorf("resolved_by = %d, want 1", resolved.ResolvedBy)
	}
}

// TestIgnoreAlert 测试忽略告警
func TestIgnoreAlert(t *testing.T) {
	database := setupAnomalyLoginTestDB(t)
	d := NewAnomalyLoginDetector()

	alert := &model.SecurityAlert{
		UserID:    1,
		Username:  "u",
		AlertType: model.AlertTypeAbnormalLogin,
		RiskLevel: model.RiskLevelHigh,
		Title:     "to_ignore",
		Status:    model.SecurityAlertStatusOpen,
	}
	database.Create(alert)

	err := d.IgnoreAlert(context.Background(), alert.ID, 1, "false positive")
	if err != nil {
		t.Fatalf("IgnoreAlert 失败: %v", err)
	}

	var ignored model.SecurityAlert
	database.First(&ignored, alert.ID)
	if ignored.Status != model.SecurityAlertStatusIgnored {
		t.Errorf("状态 = %s, want ignored", ignored.Status)
	}
}

// TestAlertChannels 验证告警通道枚举
func TestAlertChannels(t *testing.T) {
	cases := []struct {
		ch  AnomalyLoginAlertChannel
		exp string
	}{
		{AnomalyLoginChannelAudit, "audit"},
		{AnomalyLoginChannelEmail, "email"},
		{AnomalyLoginChannelInbox, "inbox"},
		{AnomalyLoginChannelWebsock, "websocket"},
	}
	for _, c := range cases {
		if string(c.ch) != c.exp {
			t.Errorf("通道名 = %s, want %s", c.ch, c.exp)
		}
	}
}
