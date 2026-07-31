package controller

// chat_ws_origin_test.go B-018 WebSocket CheckOrigin 白名单测试
//
// 验证 buildCheckOrigin 在不同白名单 / Origin 组合下的行为:
//   - 严格匹配 (allowed list 包含 origin -> 放行)
//   - 严格不匹配 (allowed list 不包含 -> 拒绝)
//   - 通配符 "*" -> 放行所有
//   - 空 Origin (无 Origin 头) -> 默认放行 (兼容非浏览器客户端)

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// makeOriginReq 构造带指定 Origin 头的测试请求
func makeOriginReq(origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws/chat", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// TestBuildCheckOrigin_StrictMatch 验证严格匹配
func TestBuildCheckOrigin_StrictMatch(t *testing.T) {
	check := buildCheckOrigin([]string{"https://app.example.com", "https://admin.example.com"})

	// 白名单内: 放行
	if !check(makeOriginReq("https://app.example.com")) {
		t.Error("expected allow for whitelisted origin")
	}
	if !check(makeOriginReq("https://admin.example.com")) {
		t.Error("expected allow for whitelisted origin")
	}

	// 不在白名单: 拒绝
	if check(makeOriginReq("https://evil.com")) {
		t.Error("expected reject for non-whitelisted origin")
	}
	if check(makeOriginReq("https://app.example.com.evil.com")) {
		t.Error("expected reject for subdomain spoofing")
	}
	if check(makeOriginReq("http://app.example.com")) {
		t.Error("expected reject for http vs https mismatch")
	}
}

// TestBuildCheckOrigin_EmptyOrigin 验证无 Origin 头默认放行
//
// 场景: 移动端 SDK、服务端、curl 测试通常不带 Origin 头
// 严格放行这些可避免误伤合法客户端
func TestBuildCheckOrigin_EmptyOrigin(t *testing.T) {
	check := buildCheckOrigin([]string{"https://app.example.com"})
	if !check(makeOriginReq("")) {
		t.Error("expected allow for empty Origin (non-browser client)")
	}
}

// TestBuildCheckOrigin_Wildcard 验证 "*" 通配符
func TestBuildCheckOrigin_Wildcard(t *testing.T) {
	check := buildCheckOrigin([]string{"*"})
	if !check(makeOriginReq("https://any.com")) {
		t.Error("wildcard should allow any origin")
	}
	if !check(makeOriginReq("http://random.evil.com")) {
		t.Error("wildcard should allow any origin (debug only)")
	}
}

// TestBuildCheckOrigin_EmptyList 验证空白名单行为
//
// 严格模式下, 空白名单应拒绝所有带 Origin 头的请求。
// 但无 Origin 头的请求仍放行 (兼容非浏览器)。
func TestBuildCheckOrigin_EmptyList(t *testing.T) {
	check := buildCheckOrigin([]string{})
	if !check(makeOriginReq("")) {
		t.Error("empty Origin should still be allowed (non-browser)")
	}
	if check(makeOriginReq("https://any.com")) {
		t.Error("with empty whitelist, non-empty origin should be rejected")
	}
}

// TestBuildCheckOrigin_NoExternalMutation 验证 buildCheckOrigin 不修改输入切片
func TestBuildCheckOrigin_NoExternalMutation(t *testing.T) {
	origins := []string{"https://a.com", "https://b.com"}
	_ = buildCheckOrigin(origins)
	if len(origins) != 2 || origins[0] != "https://a.com" {
		t.Errorf("input slice should not be mutated, got %v", origins)
	}
}
