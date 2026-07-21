package service

// row_level_security_test.go A 域 P1-4 数据行级权限服务测试
//
// 测试目标：
//  1. TeamDataScope 枚举有效性
//  2. IsValidTeamDataScope / TeamDataScopeName 工具函数
//  3. ReadTeamDataScopeContext 从 gin.Context 正确解析 user_id/role/data_scope/department_id/team_id
//  4. ApplyDataScopeForTeamByScope 注入正确的 WHERE 条件（admin/self/department/custom/降级）
//  5. BuildScopeDescription 描述生成
//
// 测试策略：仅在内存中构造 *gorm.DB（testutil.NewTestDB + AutoMigrate），
// 不需要真实连接：ApplyDataScopeForTeamByScope 实际上不执行查询，只构造 chain。

import (
	"context"
	"strings"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// containsIgnore 不区分大小写判断 s 是否包含 substr
func containsIgnore(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// setupRLSTestDB 准备 RLS 测试库
func setupRLSTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.TeamUser{},
		&model.OperationLog{},
		&model.Notification{},
		&model.SecurityAlert{},
		&model.LoginEvent{},
	)
	db.SetTestDB(database)
	return database
}

// TestIsValidTeamDataScope 测试 data_scope 枚举校验
func TestIsValidTeamDataScope(t *testing.T) {
	cases := []struct {
		scope int
		want  bool
	}{
		{int(TeamDataScopeAll), true},
		{int(TeamDataScopeDepartment), true},
		{int(TeamDataScopeSelf), true},
		{int(TeamDataScopeCustom), true},
		{0, false},
		{5, false},
		{-1, false},
		{99, false},
	}
	for _, c := range cases {
		if got := IsValidTeamDataScope(c.scope); got != c.want {
			t.Errorf("IsValidTeamDataScope(%d) = %v, want %v", c.scope, got, c.want)
		}
	}
}

// TestTeamDataScopeName 测试 data_scope 名称映射
func TestTeamDataScopeName(t *testing.T) {
	cases := []struct {
		scope int
		want  string
	}{
		{int(TeamDataScopeAll), "全部"},
		{int(TeamDataScopeDepartment), "本部门"},
		{int(TeamDataScopeSelf), "本人"},
		{int(TeamDataScopeCustom), "自定义"},
		{0, "未知"},
		{99, "未知"},
	}
	for _, c := range cases {
		if got := TeamDataScopeName(c.scope); got != c.want {
			t.Errorf("TeamDataScopeName(%d) = %q, want %q", c.scope, got, c.want)
		}
	}
}

// TestReadTeamDataScopeContext_NilCtx 测试 nil gin.Context
func TestReadTeamDataScopeContext_NilCtx(t *testing.T) {
	_, err := ReadTeamDataScopeContext(nil)
	if err == nil {
		t.Fatal("ReadTeamDataScopeContext(nil) should return error")
	}
}

// TestReadTeamDataScopeContext_AllFields 测试完整 ctx 字段解析
func TestReadTeamDataScopeContext_AllFields(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set("user_id", uint(42))
	c.Set("role", "manager")
	c.Set("data_scope", int(TeamDataScopeDepartment))
	c.Set("department_id", uint(7))
	c.Set("team_id", uint(3))

	scope, err := ReadTeamDataScopeContext(c)
	if err != nil {
		t.Fatalf("ReadTeamDataScopeContext 失败: %v", err)
	}
	if scope.UserID != 42 {
		t.Errorf("UserID = %d, want 42", scope.UserID)
	}
	if scope.Role != "manager" {
		t.Errorf("Role = %q, want manager", scope.Role)
	}
	if scope.DataScope != int(TeamDataScopeDepartment) {
		t.Errorf("DataScope = %d, want %d", scope.DataScope, TeamDataScopeDepartment)
	}
	if scope.DepartmentID != 7 {
		t.Errorf("DepartmentID = %d, want 7", scope.DepartmentID)
	}
	if scope.TeamID != 3 {
		t.Errorf("TeamID = %d, want 3", scope.TeamID)
	}
	if scope.IsAdmin {
		t.Error("IsAdmin should be false for manager")
	}
}

// TestReadTeamDataScopeContext_AdminAutoDetect 测试 admin 角色自动识别
func TestReadTeamDataScopeContext_AdminAutoDetect(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set("user_id", uint(1))
	c.Set("role", "admin")

	scope, err := ReadTeamDataScopeContext(c)
	if err != nil {
		t.Fatalf("ReadTeamDataScopeContext 失败: %v", err)
	}
	if !scope.IsAdmin {
		t.Error("IsAdmin should be true for admin role")
	}
}

// TestReadTeamDataScopeContext_StringScope 测试 string 类型的 data_scope 兼容
func TestReadTeamDataScopeContext_StringScope(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"all", int(TeamDataScopeAll)},
		{"department", int(TeamDataScopeDepartment)},
		{"self", int(TeamDataScopeSelf)},
		{"custom", int(TeamDataScopeCustom)},
		{"unknown_xxx", DefaultTeamDataScope}, // 未知值回落到默认
	}
	for _, c := range cases {
		c2, _ := gin.CreateTestContext(nil)
		c2.Set("user_id", uint(1))
		c2.Set("role", "manager")
		c2.Set("data_scope", c.input)
		scope, err := ReadTeamDataScopeContext(c2)
		if err != nil {
			t.Fatalf("ReadTeamDataScopeContext(%q) 失败: %v", c.input, err)
		}
		if scope.DataScope != c.want {
			t.Errorf("data_scope=%q parsed as %d, want %d", c.input, scope.DataScope, c.want)
		}
	}
}

// TestApplyDataScopeForTeamByScope_Admin 测试 admin 角色不附加 WHERE
func TestApplyDataScopeForTeamByScope_Admin(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	scope := &TeamDataScopeContext{
		UserID:  1,
		Role:    "admin",
		IsAdmin: true,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "user_id", "department_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	// admin → 不应该附加 user_id = 条件（用 GORM DryRun，参数化占位符 $1 / ?）
	hasUserIDFilter := containsStr(sql, "user_id = $") || containsStr(sql, "user_id = ?")
	if hasUserIDFilter {
		t.Errorf("admin should not add user_id filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeamByScope_Self 测试 self 范围附加 owner = user_id
func TestApplyDataScopeForTeamByScope_Self(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	scope := &TeamDataScopeContext{
		UserID:    42,
		Role:      "viewer",
		DataScope: int(TeamDataScopeSelf),
		IsAdmin:   false,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "owner_id", "")

	// 通过 ToSQL 拿生成的 SQL
	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "owner_id =") {
		t.Errorf("self scope should add owner_id filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeamByScope_Department 测试 department 范围
func TestApplyDataScopeForTeamByScope_Department(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	scope := &TeamDataScopeContext{
		UserID:       42,
		Role:         "manager",
		DataScope:    int(TeamDataScopeDepartment),
		DepartmentID: 7,
		IsAdmin:      false,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "user_id", "department_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "department_id =") {
		t.Errorf("department scope should add department_id filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeamByScope_DepartmentFallback 测试 department 降级为 self
func TestApplyDataScopeForTeamByScope_DepartmentFallback(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	// 没有 department_field
	scope := &TeamDataScopeContext{
		UserID:       42,
		DataScope:    int(TeamDataScopeDepartment),
		DepartmentID: 7,
		IsAdmin:      false,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "user_id", "")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "user_id =") {
		t.Errorf("department fallback should degrade to user_id filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeamByScope_Custom 测试 custom 范围（白名单）
func TestApplyDataScopeForTeamByScope_Custom(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	scope := &TeamDataScopeContext{
		UserID:        42,
		DataScope:     int(TeamDataScopeCustom),
		CustomDeptIDs: []uint{1, 2, 3},
		IsAdmin:       false,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "user_id", "department_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "department_id IN") {
		t.Errorf("custom scope should add department_id IN filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeamByScope_CustomFallback 测试 custom 无白名单时降级为 self
func TestApplyDataScopeForTeamByScope_CustomFallback(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	database := db.GetDB()
	scope := &TeamDataScopeContext{
		UserID:    42,
		DataScope: int(TeamDataScopeCustom),
		// CustomDeptIDs 为空
		IsAdmin: false,
	}
	q := svc.ApplyDataScopeForTeamByScope(database, scope, "user_id", "department_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "user_id =") {
		t.Errorf("custom fallback should degrade to user_id filter, got: %s", sql)
	}
}

// TestBuildScopeDescription 测试描述生成
func TestBuildScopeDescription(t *testing.T) {
	svc := NewRowLevelSecurityService()
	cases := []struct {
		scope int
		want  string
	}{
		{int(TeamDataScopeAll), "全部"},
		{int(TeamDataScopeSelf), "本人"},
		{int(TeamDataScopeDepartment), "本部门"},
		{int(TeamDataScopeCustom), "自定义"},
	}
	for _, c := range cases {
		if got := svc.BuildScopeDescription(c.scope); got != c.want {
			t.Errorf("BuildScopeDescription(%d) = %q, want %q", c.scope, got, c.want)
		}
	}
}

// TestApplyDataScopeForTeam_Context 集成测试：从 gin.Context 直接走 ApplyDataScopeForTeam
func TestApplyDataScopeForTeam_Context(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	c, _ := gin.CreateTestContext(nil)
	c.Set("user_id", uint(99))
	c.Set("role", "viewer")
	c.Set("data_scope", int(TeamDataScopeSelf))

	database := db.GetDB()
	q := svc.ApplyDataScopeForTeam(database, c, "user_id", "department_id", "team_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if !containsStr(sql, "user_id =") {
		t.Errorf("context-based self should add user_id filter, got: %s", sql)
	}
}

// TestApplyDataScopeForTeam_AdminContext 测试 admin 角色从 ctx 走也无过滤
func TestApplyDataScopeForTeam_AdminContext(t *testing.T) {
	setupRLSTestDB(t)
	svc := NewRowLevelSecurityService()

	c, _ := gin.CreateTestContext(nil)
	c.Set("user_id", uint(1))
	c.Set("role", "admin")
	c.Set("data_scope", int(TeamDataScopeAll))

	database := db.GetDB()
	q := svc.ApplyDataScopeForTeam(database, c, "user_id", "department_id", "team_id")

	sql := generateSQL(t, q.Model(&model.TeamUser{}))
	if containsStr(sql, "user_id =") {
		t.Errorf("admin context should not add user_id filter, got: %s", sql)
	}
}

// TestDefaultTeamDataScope 测试默认值
func TestDefaultTeamDataScope(t *testing.T) {
	if DefaultTeamDataScope != int(TeamDataScopeSelf) {
		t.Errorf("DefaultTeamDataScope = %d, want %d (self)", DefaultTeamDataScope, TeamDataScopeSelf)
	}
}

// TestMapStringDataScopeToInt 测试字符串到 int 的转换
func TestMapStringDataScopeToInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"all", int(TeamDataScopeAll)},
		{"ALL", int(TeamDataScopeAll)},
		{"department", int(TeamDataScopeDepartment)},
		{"DEPARTMENT", int(TeamDataScopeDepartment)},
		{"self", int(TeamDataScopeSelf)},
		{"custom", int(TeamDataScopeCustom)},
		{"team", int(TeamDataScopeSelf)}, // team → self（team_user 中映射）
		{"", DefaultTeamDataScope},
		{"garbage", DefaultTeamDataScope},
	}
	for _, c := range cases {
		if got := mapStringDataScopeToInt(c.in); got != c.want {
			t.Errorf("mapStringDataScopeToInt(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// generateSQL 通过 GORM 构造链生成 SQL（不执行）
func generateSQL(t *testing.T, tx *gorm.DB) string {
	t.Helper()
	// 用 ToSQL 拿到 SQL 与绑定参数
	stmt := tx.Session(&gorm.Session{DryRun: true}).Find(&[]model.TeamUser{}).Statement
	return stmt.SQL.String()
}

// Ensure unused context import stays valid for future expansion
var _ = context.Background
