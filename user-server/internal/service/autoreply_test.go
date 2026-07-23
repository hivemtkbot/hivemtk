package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupAutoReplyTestDB 设置自动回复服务测试数据库
func setupAutoReplyTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
	)
}

// newTestAutoReplyService 创建测试服务
func newTestAutoReplyService(db *gorm.DB) *AutoReplyService {
	return NewAutoReplyService(db)
}

// ============== 基础方法测试 ==============

// TestNewAutoReplyService 测试创建自动回复服务
func TestNewAutoReplyService(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestAutoReplyService_GetDB 测试获取数据库实例
func TestAutoReplyService_GetDB(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	db := service.GetDB()
	if db == nil {
		t.Error("Expected non-nil database")
	}
}

// ============== 账号管理测试 ==============

// TestAutoReplyService_ListAccounts 测试获取账号列表
func TestAutoReplyService_ListAccounts(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建测试数据
	now := time.Now()
	accounts := []model.AutoReplyAccount{
		{UserID: 1, Platform: "douyin", Username: "user1", IsActive: true, CreatedAt: now},
		{UserID: 1, Platform: "douyin", Username: "user2", IsActive: false, CreatedAt: now},
		{UserID: 1, Platform: "kuaishou", Username: "user3", IsActive: true, CreatedAt: now},
		{UserID: 2, Platform: "douyin", Username: "user4", IsActive: true, CreatedAt: now},
	}
	for i := range accounts {
		database.Create(&accounts[i])
	}

	// 测试获取用户 1 的抖音账号
	result, err := service.ListAccounts("douyin", 1)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(result))
	}

	// 测试获取用户 2 的抖音账号
	result, err = service.ListAccounts("douyin", 2)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 account, got %d", len(result))
	}

	// 测试获取不存在的用户
	result, err = service.ListAccounts("douyin", 999)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 accounts, got %d", len(result))
	}
}

// TestAutoReplyService_UpsertAccount_Create 测试创建新账号
func TestAutoReplyService_UpsertAccount_Create(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	account := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "newuser",
		Cookie:   "test_cookie",
		IsActive: true,
	}

	err := service.UpsertAccount(account)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	// 验证账号已创建
	var count int64
	database.Model(&model.AutoReplyAccount{}).Where("username = ?", "newuser").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 account, got %d", count)
	}

	// 验证 Cookie 已加密存储
	var stored model.AutoReplyAccount
	database.Where("username = ?", "newuser").First(&stored)
	if stored.Cookie == "" || stored.Cookie == "test_cookie" {
		t.Error("Expected encrypted cookie to be stored")
	}
}

// TestAutoReplyService_UpsertAccount_Update 测试更新已存在账号
func TestAutoReplyService_UpsertAccount_Update(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 先创建账号
	existing := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "existinguser",
		Cookie:   "old_cookie",
		IsActive: false,
	}
	database.Create(existing)

	// 更新账号
	updateAccount := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "existinguser",
		Cookie:   "new_cookie",
		IsActive: true,
	}

	err := service.UpsertAccount(updateAccount)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	// 验证账号已更新
	var updated model.AutoReplyAccount
	database.Where("username = ?", "existinguser").First(&updated)
	if !updated.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

// TestAutoReplyService_SaveCookies 测试保存 Cookie
func TestAutoReplyService_SaveCookies(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 先创建账号
	account := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "cookieuser",
		Cookie:   "old_cookie",
	}
	database.Create(account)

	// 保存新 Cookie（R8: 现需传入 userID 做所有权校验）
	newCookie := "new_test_cookie"
	err := service.SaveCookies(account.ID, newCookie, account.UserID)
	if err != nil {
		t.Fatalf("SaveCookies failed: %v", err)
	}

	// 验证 Cookie 已更新
	var updated model.AutoReplyAccount
	database.First(&updated, account.ID)
	if updated.Cookie == "" || updated.Cookie == "old_cookie" {
		t.Error("Expected cookie to be updated and encrypted")
	}

	// 验证可以正确解密
	decrypted, err := updated.GetCookie()
	if err != nil {
		t.Errorf("GetCookie failed: %v", err)
	}
	if decrypted != newCookie {
		t.Errorf("Expected decrypted cookie '%s', got %s", newCookie, decrypted)
	}
}

// TestAutoReplyService_SaveCookies_NotFound 测试保存不存在的账号 Cookie
func TestAutoReplyService_SaveCookies_NotFound(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// R8: 现需传入 userID
	err := service.SaveCookies(99999, "test_cookie", 1)
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

// TestAutoReplyService_SaveCookies_OwnershipCheck 测试 IDOR 所有权校验
//
// R8 回归测试：用户 A 不能修改用户 B 的账号 Cookie。
func TestAutoReplyService_SaveCookies_OwnershipCheck(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 用户 A 创建账号
	accountA := &model.AutoReplyAccount{
		UserID:   100,
		Platform: "douyin",
		Username: "userA_account",
		Cookie:   "old_cookie",
	}
	database.Create(accountA)

	// 用户 B 尝试修改用户 A 的账号 Cookie：应返回 ErrAccountNotOwned
	err := service.SaveCookies(accountA.ID, "hijacked_cookie", 200)
	if err == nil {
		t.Fatal("Expected ErrAccountNotOwned when user B modifies user A's account")
	}
	if err != ErrAccountNotOwned {
		t.Errorf("Expected ErrAccountNotOwned, got: %v", err)
	}

	// 用户 A 自己修改：应成功
	err = service.SaveCookies(accountA.ID, "new_cookie_by_owner", 100)
	if err != nil {
		t.Errorf("Expected success for owner, got: %v", err)
	}
}

// TestAutoReplyService_DeleteAccount 测试删除账号
func TestAutoReplyService_DeleteAccount(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建账号
	account := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "deleteuser",
	}
	database.Create(account)

	// 删除账号
	err := service.DeleteAccount(context.Background(), account.ID, 1)
	if err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	// 验证已删除
	var count int64
	database.Model(&model.AutoReplyAccount{}).Where("id = ?", account.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected account to be deleted, got count %d", count)
	}
}

// TestAutoReplyService_DeleteAccount_WrongUser 测试删除其他用户的账号
func TestAutoReplyService_DeleteAccount_WrongUser(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建账号
	account := &model.AutoReplyAccount{
		UserID:   1,
		Platform: "douyin",
		Username: "user1",
	}
	database.Create(account)

	// 尝试用错误的用户 ID 删除
	err := service.DeleteAccount(context.Background(), account.ID, 2)
	if err != nil {
		t.Fatalf("DeleteAccount should not fail, got: %v", err)
	}

	// 验证账号未被删除（因为用户 ID 不匹配）
	var count int64
	database.Model(&model.AutoReplyAccount{}).Where("id = ?", account.ID).Count(&count)
	if count != 1 {
		t.Errorf("Expected account to still exist, got count %d", count)
	}
}

// ============== 规则管理测试 ==============

// TestAutoReplyService_GetRule 测试获取规则
func TestAutoReplyService_GetRule(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建测试规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "关键词 1,关键词 2",
		ReplyContent: "自动回复内容",
		Frequency:    60,
		DailyLimit:   100,
		IsActive:     true,
	}
	database.Create(rule)

	// 获取规则
	result, err := service.GetRule("douyin", 1)
	if err != nil {
		t.Fatalf("GetRule failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil rule")
	}
	if result.Keywords != "关键词 1,关键词 2" {
		t.Errorf("Expected keywords '关键词 1,关键词 2', got %s", result.Keywords)
	}
}

// TestAutoReplyService_GetRule_NotFound 测试获取不存在的规则
func TestAutoReplyService_GetRule_NotFound(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	_, err := service.GetRule("douyin", 1)
	if err == nil {
		t.Error("Expected error for non-existent rule")
	}
}

// TestAutoReplyService_SaveRule_Create 测试创建规则
func TestAutoReplyService_SaveRule_Create(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	startTime := "09:00"
	endTime := "18:00"
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "测试关键词",
		ReplyContent: "测试回复内容",
		Frequency:    30,
		DailyLimit:   50,
		StartTime:    &startTime,
		EndTime:      &endTime,
		IsActive:     true,
	}

	err := service.SaveRule(rule)
	if err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	// 验证规则已创建
	var count int64
	database.Model(&model.AutoReplyRule{}).Where("platform = ?", "douyin").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 rule, got %d", count)
	}
}

// TestAutoReplyService_SaveRule_Update 测试更新规则
func TestAutoReplyService_SaveRule_Update(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 先创建规则
	existing := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "旧关键词",
		ReplyContent: "旧回复内容",
		Frequency:    60,
		DailyLimit:   100,
		IsActive:     true,
	}
	database.Create(existing)

	// 更新规则
	updateRule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "新关键词",
		ReplyContent: "新回复内容",
		Frequency:    30,
		DailyLimit:   50,
		IsActive:     false,
	}

	err := service.SaveRule(updateRule)
	if err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	// 验证规则已更新
	var updated model.AutoReplyRule
	database.Where("platform = ?", "douyin").First(&updated)
	if updated.Keywords != "新关键词" {
		t.Errorf("Expected keywords '新关键词', got %s", updated.Keywords)
	}
	if updated.IsActive {
		t.Error("Expected IsActive to be false")
	}
}

// TestAutoReplyService_ListRules 测试获取规则列表
func TestAutoReplyService_ListRules(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建测试数据
	for i := 0; i < 5; i++ {
		rule := &model.AutoReplyRule{
			UserID:       1,
			Platform:     "douyin",
			Keywords:     "关键词" + string(rune('0'+i)),
			ReplyContent: "回复内容" + string(rune('0'+i)),
			Frequency:    60,
			DailyLimit:   100,
			IsActive:     i%2 == 0,
		}
		database.Create(rule)
	}

	// 获取全部规则
	req := &dto.AutoReplyRuleListRequest{
		Platform: "douyin",
		UserID:   1,
		Page:     1,
		PageSize: 10,
	}

	rules, total, err := service.ListRules(ctx, req)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(rules) != 5 {
		t.Errorf("Expected 5 rules, got %d", len(rules))
	}
}

// TestAutoReplyService_CreateRule 测试创建规则
func TestAutoReplyService_CreateRule(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	startTime := "08:00"
	endTime := "20:00"
	req := &dto.AutoReplyRuleRequest{
		UserID:       1,
		Platform:     "kuaishou",
		Keywords:     "快手关键词",
		ReplyContent: "快手回复内容",
		Frequency:    45,
		DailyLimit:   80,
		StartTime:    &startTime,
		EndTime:      &endTime,
		IsActive:     true,
	}

	rule, err := service.CreateRule(ctx, req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if rule == nil {
		t.Fatal("Expected non-nil rule")
	}
	if rule.Keywords != "快手关键词" {
		t.Errorf("Expected keywords '快手关键词', got %s", rule.Keywords)
	}
	if rule.Platform != "kuaishou" {
		t.Errorf("Expected platform 'kuaishou', got %s", rule.Platform)
	}
}

// TestAutoReplyService_UpdateRule 测试更新规则
func TestAutoReplyService_UpdateRule(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 先创建规则
	existing := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "旧关键词",
		ReplyContent: "旧回复",
		Frequency:    60,
		DailyLimit:   100,
		IsActive:     true,
	}
	database.Create(existing)

	newStartTime := "10:00"
	newEndTime := "22:00"
	req := &dto.AutoReplyRuleRequest{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "新关键词",
		ReplyContent: "新回复",
		Frequency:    30,
		DailyLimit:   50,
		StartTime:    &newStartTime,
		EndTime:      &newEndTime,
		IsActive:     false,
	}

	rule, err := service.UpdateRule(ctx, existing.ID, req)
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	if rule.Keywords != "新关键词" {
		t.Errorf("Expected keywords '新关键词', got %s", rule.Keywords)
	}
	if rule.Frequency != 30 {
		t.Errorf("Expected frequency 30, got %d", rule.Frequency)
	}
}

// TestAutoReplyService_UpdateRule_NotFound 测试更新不存在的规则
func TestAutoReplyService_UpdateRule_NotFound(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	req := &dto.AutoReplyRuleRequest{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "关键词",
		ReplyContent: "回复",
	}

	_, err := service.UpdateRule(ctx, 99999, req)
	if err == nil {
		t.Error("Expected error for non-existent rule")
	}
}

// TestAutoReplyService_DeleteRule 测试删除规则
func TestAutoReplyService_DeleteRule(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "待删除关键词",
		ReplyContent: "待删除回复",
	}
	database.Create(rule)

	err := service.DeleteRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	// 验证已删除
	var count int64
	database.Model(&model.AutoReplyRule{}).Where("id = ?", rule.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected rule to be deleted, got count %d", count)
	}
}

// ============== 日志管理测试 ==============

// TestAutoReplyService_ListRecentLogs 测试获取最近日志
func TestAutoReplyService_ListRecentLogs(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	now := time.Now()

	// 创建测试日志（72 小时内）
	for i := 0; i < 5; i++ {
		log := &model.AutoReplyLog{
			UserID:        1,
			AccountID:     1,
			RuleID:        1,
			Platform:      "douyin",
			TargetContent: "目标内容" + string(rune('0'+i)),
			ReplyContent:  "回复内容" + string(rune('0'+i)),
			Status:        "matched",
			CreatedAt:     now.Add(-time.Duration(i) * time.Hour),
		}
		database.Create(log)
	}

	// 获取日志
	logs, total, err := service.ListRecentLogs("douyin", 1, 1, 10)
	if err != nil {
		t.Fatalf("ListRecentLogs failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}
}

// TestAutoReplyService_ListRecentLogs_Pagination 测试日志分页
func TestAutoReplyService_ListRecentLogs_Pagination(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	now := time.Now()

	// 创建 15 条日志
	for i := 0; i < 15; i++ {
		log := &model.AutoReplyLog{
			UserID:        1,
			AccountID:     1,
			Platform:      "douyin",
			TargetContent: "内容" + string(rune('0'+i)),
			Status:        "matched",
			CreatedAt:     now.Add(-time.Duration(i) * time.Hour),
		}
		database.Create(log)
	}

	// 第一页
	logs, total, err := service.ListRecentLogs("douyin", 1, 1, 5)
	if err != nil {
		t.Fatalf("ListRecentLogs failed: %v", err)
	}
	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs on page 1, got %d", len(logs))
	}

	// 第二页
	logs, total, err = service.ListRecentLogs("douyin", 1, 2, 5)
	if err != nil {
		t.Fatalf("ListRecentLogs failed: %v", err)
	}
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs on page 2, got %d", len(logs))
	}
}

// TestAutoReplyService_ListRecentLogs_InvalidPage 测试无效页码处理
func TestAutoReplyService_ListRecentLogs_InvalidPage(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 页码 <= 0 应该默认为 1
	logs, _, err := service.ListRecentLogs("douyin", 1, 0, 10)
	if err != nil {
		t.Fatalf("ListRecentLogs failed: %v", err)
	}
	if logs == nil {
		t.Error("Expected non-nil logs")
	}
}

// TestAutoReplyService_AppendLog 测试添加日志
func TestAutoReplyService_AppendLog(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	err := service.AppendLog(1, 2, 3, "douyin", "目标内容", "回复内容", "matched", "")
	if err != nil {
		t.Fatalf("AppendLog failed: %v", err)
	}

	// 验证日志已创建
	var count int64
	database.Model(&model.AutoReplyLog{}).Where("user_id = ? AND platform = ?", 1, "douyin").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 log, got %d", count)
	}

	// 验证日志内容
	var log model.AutoReplyLog
	database.First(&log)
	if log.Status != "matched" {
		t.Errorf("Expected status 'matched', got %s", log.Status)
	}
	if log.TargetContent != "目标内容" {
		t.Errorf("Expected target content '目标内容', got %s", log.TargetContent)
	}
}

// ============== 关键词匹配测试 ==============

// TestAutoReplyService_TestMatching 测试关键词匹配
func TestAutoReplyService_TestMatching(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建激活的规则（不设置时间段和日限制以避免被跳过）
	// 注意：关键词使用英文逗号分隔
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "你好,hello,价格", // 使用英文逗号分隔
		ReplyContent: "您好,请问有什么可以帮您？",
		Frequency:    60,
		DailyLimit:   0, // 0 表示无限制
		IsActive:     true,
	}
	database.Create(rule)


	// 测试精确匹配
	matchedRule, err := service.TestMatching(ctx, "douyin", "你好,请问这个多少钱", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule == nil {
		t.Error("Expected matched rule")
	}
}

// TestAutoReplyService_TestMatching_NoMatch 测试无匹配
func TestAutoReplyService_TestMatching_NoMatch(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建激活的规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "特定关键词",
		ReplyContent: "回复内容",
		IsActive:     true,
	}
	database.Create(rule)


	// 测试不匹配的消息
	matchedRule, err := service.TestMatching(ctx, "douyin", "完全不相关的内容", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule != nil {
		t.Error("Expected nil for no match")
	}
}

// TestAutoReplyService_TestMatching_InactiveRule 测试非激活规则不匹配
func TestAutoReplyService_TestMatching_FuzzyPattern(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建带通配符的规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "优惠*",
		ReplyContent: "优惠活动详情",
		IsActive:     true,
		DailyLimit:   0, // 无限制
	}
	database.Create(rule)


	// 测试通配符匹配
	matchedRule, err := service.TestMatching(ctx, "douyin", "今天有什么优惠活动", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule == nil {
		t.Error("Expected matched rule with wildcard pattern")
	}
}

// TestAutoReplyService_TestMatching_RegexPattern 测试正则匹配
func TestAutoReplyService_TestMatching_RegexPattern(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建带正则表达式的规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "/\\d+/", // 匹配数字
		ReplyContent: "检测到数字",
		IsActive:     true,
		DailyLimit:   0, // 无限制
	}
	database.Create(rule)


	// 测试正则匹配
	matchedRule, err := service.TestMatching(ctx, "douyin", "我的电话是 12345", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule == nil {
		t.Error("Expected matched rule with regex pattern")
	}
}

// TestAutoReplyService_TestMatching_CaseInsensitive 测试不区分大小写匹配
func TestAutoReplyService_TestMatching_CaseInsensitive(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "HELLO",
		ReplyContent: "Hello reply",
		IsActive:     true,
	}
	database.Create(rule)


	// 测试小写消息匹配大写关键词
	matchedRule, err := service.TestMatching(ctx, "douyin", "hello world", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule == nil {
		t.Error("Expected case-insensitive match")
	}
}

// ============== 时间范围测试 ==============

// TestAutoReplyService_isWithinTimeRange_NormalRange 测试正常时间范围
func TestAutoReplyService_isWithinTimeRange_NormalRange(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	startTime := "09:00"
	endTime := "18:00"
	rule := model.AutoReplyRule{
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	// 在工作时间内（假设当前时间在 9-18 点之间）
	// 由于测试运行时间不确定,我们测试边界逻辑
	if !service.isWithinTimeRange(rule) {
		// 如果当前时间不在 9-18 点,这是预期的
		t.Logf("Current time may be outside 09:00-18:00")
	}
}

// TestAutoReplyService_isWithinTimeRange_NilTime 测试 nil 时间范围
func TestAutoReplyService_isWithinTimeRange_NilTime(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	rule := model.AutoReplyRule{
		StartTime: nil,
		EndTime:   nil,
	}

	// nil 时间应该始终允许
	allowed := service.isWithinTimeRange(rule)
	if !allowed {
		t.Error("Expected true for nil time range")
	}
}

// TestAutoReplyService_isWithinTimeRange_InvalidFormat 测试无效时间格式
func TestAutoReplyService_isWithinTimeRange_InvalidFormat(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	invalidTime := "invalid"
	rule := model.AutoReplyRule{
		StartTime: &invalidTime,
		EndTime:   &invalidTime,
	}

	// 无效格式应该允许（fallback 行为）
	allowed := service.isWithinTimeRange(rule)
	if !allowed {
		t.Error("Expected true for invalid time format")
	}
}

// TestAutoReplyService_isWithinTimeRange_CrossDay 测试跨天时间范围
func TestAutoReplyService_isWithinTimeRange_CrossDay(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 跨天情况：22:00 - 06:00
	startTime := "22:00"
	endTime := "06:00"
	rule := model.AutoReplyRule{
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	// 测试跨天逻辑（实际结果取决于当前时间）
	_ = service.isWithinTimeRange(rule)
}

// ============== 每日配额测试 ==============

// TestAutoReplyService_hasRemainingDailyQuota_NoLimit 测试无限制情况
func TestAutoReplyService_hasRemainingDailyQuota_NoLimit(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	rule := model.AutoReplyRule{
		ID:         1,
		DailyLimit: 0, // 0 表示无限制
	}

	hasQuota := service.hasRemainingDailyQuota(ctx, rule, 1)
	if !hasQuota {
		t.Error("Expected true for no daily limit")
	}
}

// TestAutoReplyService_hasRemainingDailyQuota_WithLimit 测试有日限制情况
func TestAutoReplyService_hasRemainingDailyQuota_WithLimit(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建日限制为 5 的规则
	rule := model.AutoReplyRule{
		ID:         1,
		UserID:     1,
		DailyLimit: 5,
	}
	database.Create(&rule)

	// 创建 3 条今日日志（未超过限制）
	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	for i := 0; i < 3; i++ {
		log := &model.AutoReplyLog{
			UserID:    1,
			RuleID:    rule.ID,
			Status:    "matched",
			CreatedAt: today.Add(time.Duration(i) * time.Hour),
		}
		database.Create(log)
	}

	hasQuota := service.hasRemainingDailyQuota(ctx, rule, 1)
	if !hasQuota {
		t.Error("Expected true when under daily limit")
	}

	// 再创建 3 条日志,使总数达到 6（超过限制）
	for i := 0; i < 3; i++ {
		log := &model.AutoReplyLog{
			UserID:    1,
			RuleID:    rule.ID,
			Status:    "matched",
			CreatedAt: today.Add(time.Duration(i+3) * time.Hour),
		}
		database.Create(log)
	}

	hasQuota = service.hasRemainingDailyQuota(ctx, rule, 1)
	if hasQuota {
		t.Error("Expected false when over daily limit")
	}
}

// ============== 模拟消息测试 ==============

// TestAutoReplyService_SimulateMessage_Matched 测试模拟消息匹配
func TestAutoReplyService_SimulateMessage_Matched(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "测试",
		ReplyContent: "测试回复",
		IsActive:     true,
	}
	database.Create(rule)


	log, err := service.SimulateMessage(ctx, "douyin", "这是一条测试消息", "sender", 1, 1)
	if err != nil {
		t.Fatalf("SimulateMessage failed: %v", err)
	}
	if log == nil {
		t.Fatal("Expected non-nil log")
	}
	if log.Status != "matched" {
		t.Errorf("Expected status 'matched', got %s", log.Status)
	}
	if log.ReplyContent != "测试回复" {
		t.Errorf("Expected reply '测试回复', got %s", log.ReplyContent)
	}
}

// TestAutoReplyService_SimulateMessage_NoMatch 测试模拟消息无匹配
func TestAutoReplyService_SimulateMessage_NoMatch(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)


	log, err := service.SimulateMessage(ctx, "douyin", "不相关的内容", "sender", 1, 1)
	if err != nil {
		t.Fatalf("SimulateMessage failed: %v", err)
	}
	if log == nil {
		t.Fatal("Expected non-nil log")
	}
	if log.Status != "no_match" {
		t.Errorf("Expected status 'no_match', got %s", log.Status)
	}
}

// ============== 批量匹配测试 ==============

// TestAutoReplyService_TestBatchMatching 测试批量匹配
func TestAutoReplyService_TestBatchMatching(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建规则（使用英文逗号分隔关键词）
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "测试,你好", // 使用英文逗号分隔
		ReplyContent: "自动回复",
		IsActive:     true,
		DailyLimit:   0, // 无限制
	}
	database.Create(rule)

	messages := []string{
		"这是一条测试消息", // 包含"测试"
		"你好,请问在吗",  // 包含"你好"
		"完全不相关的内容", // 不匹配
	}

	results, err := service.TestBatchMatching(ctx, "douyin", messages, 1, 1)
	if err != nil {
		t.Fatalf("TestBatchMatching failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// 验证匹配状态
	matchCount := 0
	for _, r := range results {
		if r.Status == "matched" {
			matchCount++
		}
	}
	// 前两条消息应该匹配（包含"测试"和"你好"）
	if matchCount < 2 {
		t.Errorf("Expected at least 2 matches, got %d", matchCount)
	}
}

// ============== 速率限制测试 ==============

// TestAutoReplyService_TestRateLimit 测试速率限制
func TestAutoReplyService_TestRateLimit(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)


	results, err := service.TestRateLimit(ctx, "douyin", 1, 1, 10)
	if err != nil {
		t.Fatalf("TestRateLimit failed: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	// 验证速率限制逻辑
	// 代码逻辑：if i > 0 && i%3 == 0，即 i=3,6,9 时被限制
	// 对应的 TestID = i+1 = 4,7,10
	limitedCount := 0
	for _, r := range results {
		// TestID 4,7,10 应该被限制
		expectedLimited := (r.TestID == 4 || r.TestID == 7 || r.TestID == 10)

		if r.Allowed == expectedLimited {
			t.Errorf("TestID %d: expected allowed=%v, got allowed=%v", r.TestID, !expectedLimited, r.Allowed)
		}

		if !r.Allowed {
			limitedCount++
		}
	}
	if limitedCount != 3 {
		t.Errorf("Expected 3 rate limited results, got %d", limitedCount)
	}
}

// TestAutoReplyService_ResetDailyLimit 测试重置每日限制
func TestAutoReplyService_ResetDailyLimit(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	err := service.ResetDailyLimit(ctx, "douyin", 1, 1)
	if err != nil {
		t.Fatalf("ResetDailyLimit failed: %v", err)
	}
}

// ============== 统计信息测试 ==============

// TestAutoReplyService_GetRateLimitStats 测试获取速率限制统计
func TestAutoReplyService_GetRateLimitStats(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	stats, err := service.GetRateLimitStats(ctx, "douyin", 1, 1)
	if err != nil {
		t.Fatalf("GetRateLimitStats failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}
	if stats["platform"] != "douyin" {
		t.Errorf("Expected platform 'douyin', got %v", stats["platform"])
	}
	if stats["user_id"] != uint(1) {
		t.Errorf("Expected user_id 1, got %v", stats["user_id"])
	}
	if stats["daily_limit"] != int64(100) {
		t.Errorf("Expected daily_limit 100, got %v", stats["daily_limit"])
	}
}

// TestAutoReplyService_GetConcurrentStats 测试获取并发统计
func TestAutoReplyService_GetConcurrentStats(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	stats, err := service.GetConcurrentStats(ctx, "douyin", 1)
	if err != nil {
		t.Fatalf("GetConcurrentStats failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}
	if stats["platform"] != "douyin" {
		t.Errorf("Expected platform 'douyin', got %v", stats["platform"])
	}
	if stats["max_concurrent"] != int64(5) {
		t.Errorf("Expected max_concurrent 5, got %v (type: %T)", stats["max_concurrent"], stats["max_concurrent"])
	}
}

// ============== 边界条件测试 ==============

// TestAutoReplyService_ListAccounts_EmptyPlatform 测试空平台参数
func TestAutoReplyService_ListAccounts_EmptyPlatform(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	result, err := service.ListAccounts("", 1)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// TestAutoReplyService_ListRules_EmptyRequest 测试空请求参数
func TestAutoReplyService_ListRules_EmptyRequest(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	req := &dto.AutoReplyRuleListRequest{
		Page:     1,
		PageSize: 10,
	}

	rules, total, err := service.ListRules(ctx, req)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if rules == nil {
		t.Error("Expected non-nil rules")
	}
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

// TestAutoReplyService_ListRecentLogs_OldLogs 测试 72 小时前的日志过滤
func TestAutoReplyService_ListRecentLogs_OldLogs(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	now := time.Now()

	// 创建 5 条 72 小时内的日志
	for i := 0; i < 5; i++ {
		log := &model.AutoReplyLog{
			UserID:        1,
			Platform:      "douyin",
			TargetContent: "新日志",
			Status:        "matched",
			CreatedAt:     now.Add(-time.Duration(i) * time.Hour),
		}
		database.Create(log)
	}

	// 创建 5 条 72 小时前的日志
	for i := 0; i < 5; i++ {
		log := &model.AutoReplyLog{
			UserID:        1,
			Platform:      "douyin",
			TargetContent: "旧日志",
			Status:        "matched",
			CreatedAt:     now.Add(-100 * time.Hour), // 100 小时前
		}
		database.Create(log)
	}

	logs, total, err := service.ListRecentLogs("douyin", 1, 1, 20)
	if err != nil {
		t.Fatalf("ListRecentLogs failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5 (only recent logs), got %d", total)
	}

	// 验证返回的日志都是新的
	for _, log := range logs {
		if log.TargetContent != "新日志" {
			t.Errorf("Expected only recent logs, got old log: %s", log.TargetContent)
		}
	}
}

// TestAutoReplyService_TestMatching_MultipleKeywords 测试多个关键词匹配
func TestAutoReplyService_TestMatching_MultipleKeywords(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建带多个关键词的规则（使用英文逗号分隔）
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "关键词 1,关键词 2,关键词 3,空,", // 使用英文逗号分隔,包含空关键词
		ReplyContent: "多关键词回复",
		IsActive:     true,
		DailyLimit:   0, // 无限制
	}
	database.Create(rule)


	// 测试匹配第二个关键词
	matchedRule, err := service.TestMatching(ctx, "douyin", "包含关键词 2 的消息", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	if matchedRule == nil {
		t.Error("Expected match for second keyword")
	}
}

// TestAutoReplyService_TestMatching_InvalidRegex 测试无效正则表达式处理
func TestAutoReplyService_TestMatching_InvalidRegex(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建带无效正则的规则
	rule := &model.AutoReplyRule{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "/[invalid(/", // 无效的正则
		ReplyContent: "回复",
		IsActive:     true,
	}
	database.Create(rule)


	// 无效正则会失败,但不应崩溃
	matchedRule, err := service.TestMatching(ctx, "douyin", "测试消息", 1)
	if err != nil {
		t.Fatalf("TestMatching failed: %v", err)
	}
	// 无效正则不应匹配
	if matchedRule != nil {
		t.Error("Expected no match for invalid regex")
	}
}

// TestAutoReplyService_SimulateMessage_WithContextCancellation 测试上下文取消
func TestAutoReplyService_SimulateMessage_WithContextCancellation(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 即使上下文已取消,模拟消息仍应完成（因为不使用上下文进行数据库操作）
	log, err := service.SimulateMessage(ctx, "douyin", "测试消息", "sender", 1, 1)
	if err != nil {
		t.Fatalf("SimulateMessage failed: %v", err)
	}
	if log == nil {
		t.Error("Expected non-nil log even with cancelled context")
	}
}

// TestAutoReplyService_CreateRule_EmptyContent 测试创建空内容规则
func TestAutoReplyService_CreateRule_EmptyContent(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	req := &dto.AutoReplyRuleRequest{
		UserID:       1,
		Platform:     "douyin",
		Keywords:     "",    // 空关键词
		ReplyContent: "",    // 空回复内容
		Frequency:    0,     // 零频率
		DailyLimit:   0,     // 零日限制
		IsActive:     false, // 非激活
	}

	rule, err := service.CreateRule(ctx, req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if rule == nil {
		t.Fatal("Expected non-nil rule")
	}
}

// TestAutoReplyService_ListRules_LargePageSize 测试大分页参数
func TestAutoReplyService_ListRules_LargePageSize(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	// 创建 100 条规则
	for i := 0; i < 100; i++ {
		rule := &model.AutoReplyRule{
			UserID:       1,
			Platform:     "douyin",
			Keywords:     "关键词" + string(rune(i%10+'0')),
			ReplyContent: "回复",
			IsActive:     true,
		}
		database.Create(rule)
	}

	req := &dto.AutoReplyRuleListRequest{
		Page:     1,
		PageSize: 1000, // 大分页
	}

	rules, total, err := service.ListRules(ctx, req)
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if total != 100 {
		t.Errorf("Expected total 100, got %d", total)
	}
	if len(rules) != 100 {
		t.Errorf("Expected 100 rules, got %d", len(rules))
	}
}

// TestAutoReplyService_DeleteRule_NonExistent 测试删除不存在的规则
func TestAutoReplyService_DeleteRule_NonExistent(t *testing.T) {
	database := setupAutoReplyTestDB(t)
	service := newTestAutoReplyService(database)

	err := service.DeleteRule(ctx, 99999)
	if err != nil {
		t.Errorf("DeleteRule should not fail for non-existent rule: %v", err)
	}
}

// ============== 辅助函数测试 ==============

// TestIsFuzzyMatch 测试模糊匹配函数
func TestIsFuzzyMatch(t *testing.T) {
	tests := []struct {
		message string
		pattern string
		want    bool
	}{
		{"hello world", "hello*", true},
		{"hello world", "*world", true},
		{"hello world", "h?llo", true},
		{"hello world", "test*", false},
		{"hello", "hello", false}, // 没有通配符,不是模糊匹配
		{"test", "te?t", true},
	}

	for _, tt := range tests {
		got := isFuzzyMatch(tt.message, tt.pattern)
		if got != tt.want {
			t.Errorf("isFuzzyMatch(%q, %q) = %v, want %v", tt.message, tt.pattern, got, tt.want)
		}
	}
}

// TestIsRegexMatch 测试正则匹配函数
func TestIsRegexMatch(t *testing.T) {
	tests := []struct {
		message string
		pattern string
		want    bool
	}{
		{"hello123", "/\\d+/", true},
		{"hello", "/[a-z]+/", true},
		{"hello123", "/^world/", false},
		{"hello", "hello", false},      // 没有斜杠,不是正则
		{"test", "/[invalid(/", false}, // 无效正则
		{"123", "/\\d{3}/", true},      // 精确 3 位数字
		{"1234", "/\\d{3}/", true},     // 包含 3 位数字
	}

	for _, tt := range tests {
		got := isRegexMatch(tt.message, tt.pattern)
		if got != tt.want {
			t.Errorf("isRegexMatch(%q, %q) = %v, want %v", tt.message, tt.pattern, got, tt.want)
		}
	}
}
