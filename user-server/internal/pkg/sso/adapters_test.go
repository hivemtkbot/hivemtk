// SSO 适配器测试（2026-08-15 M3-P1-E3）
//
// 覆盖各 IdP 适配器的 claim 归一化逻辑：
//   - 飞书：name 兜底用户名 / avatar_url / union_id
//   - 钉钉：nick 优先用户名 / avatar
//   - 企微：userid 兜底 subject+用户名 / avatar
//   - 通用 OIDC：标准 claims 直通
//   - NewAdapter 空名称报错 / 未知名称回退通用
package sso

import (
	"testing"
)

func mkClaims(sub, name, email, username string, extra map[string]interface{}) *IDTokenClaims {
	c := &IDTokenClaims{
		Subject:           sub,
		Name:              name,
		Email:             email,
		PreferredUsername: username,
		Extra:             map[string]interface{}{},
	}
	for k, v := range extra {
		c.Extra[k] = v
	}
	return c
}

func newTestAdapter(t *testing.T, name string) Adapter {
	t.Helper()
	a, err := NewAdapter(name, OIDCConfig{
		Issuer:      "https://test-idp.example.com",
		ClientID:    "test-client",
		RedirectURL: "https://hivemtk.example.com/api/sso/callback/" + name,
		DefaultRole: "user",
	})
	if err != nil {
		t.Fatalf("NewAdapter(%q): %v", name, err)
	}
	return a
}

func TestGenericAdapter_MapClaims(t *testing.T) {
	a := newTestAdapter(t, "generic")
	nu := a.MapClaims(mkClaims("sub-1", "张三", "zhang@example.com", "zhangsan", nil))
	if nu.Provider != "generic" {
		t.Errorf("provider: got %q", nu.Provider)
	}
	if nu.Subject != "sub-1" {
		t.Errorf("subject: got %q", nu.Subject)
	}
	if nu.Username != "zhangsan" {
		t.Errorf("username: got %q", nu.Username)
	}
	if nu.Email != "zhang@example.com" {
		t.Errorf("email: got %q", nu.Email)
	}
	if nu.RealName != "张三" {
		t.Errorf("real_name: got %q", nu.RealName)
	}
	if nu.Role != "user" {
		t.Errorf("role: got %q (default fallback)", nu.Role)
	}
}

func TestFeishuAdapter_MapClaims(t *testing.T) {
	a := newTestAdapter(t, "feishu")

	nu := a.MapClaims(mkClaims("on_123", "李四", "li@example.com", "", map[string]interface{}{
		"avatar_url": "https://cdn.feishu.cn/a.png",
		"union_id":   "on_union_123",
	}))
	if nu.Username != "李四" {
		t.Errorf("username should fall back to name, got %q", nu.Username)
	}
	if nu.Avatar != "https://cdn.feishu.cn/a.png" {
		t.Errorf("avatar: got %q", nu.Avatar)
	}
	if nu.Subject != "on_123" {
		t.Errorf("subject should keep sub (union_id only when sub empty), got %q", nu.Subject)
	}

	nu2 := a.MapClaims(mkClaims("", "王五", "", "", map[string]interface{}{
		"union_id": "on_union_456",
	}))
	if nu2.Subject != "on_union_456" {
		t.Errorf("subject fallback to union_id: got %q", nu2.Subject)
	}
}

func TestDingTalkAdapter_MapClaims(t *testing.T) {
	a := newTestAdapter(t, "dingtalk")

	nu := a.MapClaims(mkClaims("userid-1", "赵六", "", "", map[string]interface{}{
		"nick":   "赵六昵称",
		"avatar": "https://avatar.dingtalk.com/1.png",
	}))
	if nu.Username != "赵六昵称" {
		t.Errorf("username should prefer nick, got %q", nu.Username)
	}
	if nu.Avatar != "https://avatar.dingtalk.com/1.png" {
		t.Errorf("avatar: got %q", nu.Avatar)
	}
}

func TestWeComAdapter_MapClaims(t *testing.T) {
	a := newTestAdapter(t, "wecom")

	nu := a.MapClaims(mkClaims("", "孙七", "", "", map[string]interface{}{
		"userid": "zhangsan@corp",
	}))
	if nu.Subject != "zhangsan@corp" {
		t.Errorf("subject fallback to userid: got %q", nu.Subject)
	}
	if nu.Username != "zhangsan@corp" {
		t.Errorf("username fallback to userid: got %q", nu.Username)
	}
}

func TestNewAdapter_EmptyName(t *testing.T) {
	if _, err := NewAdapter("", OIDCConfig{}); err == nil {
		t.Fatal("expected error for empty provider name")
	}
}

func TestNewAdapter_UnknownNameFallsBackToGeneric(t *testing.T) {
	a := newTestAdapter(t, "okta")
	if _, ok := a.(*GenericAdapter); !ok {
		t.Fatalf("expected GenericAdapter for unknown name, got %T", a)
	}
	if a.DisplayName() != "okta" {
		t.Errorf("display name: got %q", a.DisplayName())
	}
}

func TestNewAdapter_BuiltinDisplayNames(t *testing.T) {
	cases := map[string]string{
		"feishu":   "飞书",
		"dingtalk": "钉钉",
		"wecom":    "企业微信",
	}
	for name, want := range cases {
		a := newTestAdapter(t, name)
		if got := a.DisplayName(); got != want {
			t.Errorf("%s display name: got %q want %q", name, got, want)
		}
	}
}

func TestAdapters_MapRoleFromRolesClaim(t *testing.T) {
	a := newTestAdapter(t, "generic")
	c := mkClaims("sub-1", "", "", "", nil)
	c.Roles = []string{"admin"}
	nu := a.MapClaims(c)
	if nu.Role != "admin" {
		t.Errorf("role should come from roles claim, got %q", nu.Role)
	}
}
