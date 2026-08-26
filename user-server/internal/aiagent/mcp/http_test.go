package mcp

// TL-2 单测：版本协商矩阵 / Session-Id 缺失与错误 / Accept 校验

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// ---------- 版本协商矩阵 ----------

func TestNegotiateProtocolVersion_Matrix(t *testing.T) {
	cases := []struct {
		requested string
		want      string
	}{
		{"2025-06-18", "2025-06-18"},   // 支持的旧版 → 原样
		{"2025-11-25", "2025-11-25"},   // 支持的最新版 → 原样
		{"2025-03-26", "2025-11-25"},   // 已废弃版本 → 最新支持版
		{"2030-01-01", "2025-11-25"},   // 未来版本 → 最新支持版
		{"", "2025-11-25"},             // 缺失 → 最新支持版
		{"garbage", "2025-11-25"},      // 非法值 → 最新支持版
	}
	for _, c := range cases {
		if got := NegotiateProtocolVersion(c.requested); got != c.want {
			t.Errorf("NegotiateProtocolVersion(%q) = %q, want %q", c.requested, got, c.want)
		}
	}
}

func TestInitialize_NegotiationMatrix(t *testing.T) {
	cases := []struct {
		requested string
		want      string
	}{
		{"2025-06-18", "2025-06-18"},
		{"2025-11-25", "2025-11-25"},
		{"2024-11-05", ProtocolVersionLatest},
	}
	for _, c := range cases {
		s := NewServer(nil)
		resp := send(t, s, "initialize", InitializeParams{
			ProtocolVersion: c.requested,
			ClientInfo:      Implementation{Name: "t", Version: "0"},
		})
		if resp.Error != nil {
			t.Fatalf("requested=%s: initialize failed: %v", c.requested, resp.Error)
		}
		var result InitializeResult
		data, _ := json.Marshal(resp.Result)
		json.Unmarshal(data, &result)
		if result.ProtocolVersion != c.want {
			t.Errorf("requested=%s: negotiated=%s, want %s", c.requested, result.ProtocolVersion, c.want)
		}
	}
}

// ---------- Session-Id ----------

func TestInitialize_SessionIDVisibleASCII(t *testing.T) {
	s := NewServer(nil)
	initializeReq(t, s)
	sid := s.SessionID()
	if sid == "" {
		t.Fatal("session id should be generated after initialize")
	}
	for _, r := range sid {
		if r < 0x21 || r > 0x7E {
			t.Fatalf("session id contains non-visible-ASCII char %q in %q", r, sid)
		}
	}
}

func TestHTTPHandler_SessionIDFlow(t *testing.T) {
	h := NewHTTPHandler(tooluse.NewToolRegistry())

	// 1. initialize → 200 + Mcp-Session-Id 头
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"t","version":"0"}}}`))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("initialize status = %d, body=%s", rec.Code, rec.Body.String())
	}
	sid := rec.Header().Get(HeaderMcpSessionID)
	if sid == "" {
		t.Fatal("initialize response must carry Mcp-Session-Id header")
	}

	pingBody := `{"jsonrpc":"2.0","id":"2","method":"ping"}`

	// 2a. 后续请求缺失 Session-Id → 400
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("missing session id: status = %d, want 400", rec.Code)
	}

	// 2b. 错误 Session-Id → 400
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, "bogus-session-id")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("wrong session id: status = %d, want 400", rec.Code)
	}

	// 2c. 正确 Session-Id → 200
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("valid session id: status = %d, want 200", rec.Code)
	}
}

// ---------- Accept 校验 ----------

func TestHTTPHandler_AcceptValidation(t *testing.T) {
	h := NewHTTPHandler(tooluse.NewToolRegistry())
	body := `{"jsonrpc":"2.0","id":"1","method":"ping"}`

	cases := []struct {
		accept string
		want   int
	}{
		{"", 406},                        // 缺失 → 406
		{"text/html", 406},               // 不含合法类型 → 406
		{"application/json", 400},        // 合法；无会话 → 下一层校验 400
		{"text/event-stream", 400},       // 合法（SSE 类型）
		{"text/html, application/json;q=0.9", 400}, // 带参数/多值仍命中
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		if c.accept != "" {
			req.Header.Set("Accept", c.accept)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("Accept=%q: status = %d, want %d", c.accept, rec.Code, c.want)
		}
	}
}

func TestHTTPHandler_NotificationAccepted(t *testing.T) {
	h := NewHTTPHandler(tooluse.NewToolRegistry())
	// 先 initialize 拿会话
	initBody := `{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-11-25"}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(initBody))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	sid := rec.Header().Get(HeaderMcpSessionID)

	// notifications/initialized → 无响应体，返回 202
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Errorf("notification status = %d, want 202", rec.Code)
	}
}

// ---------- 回归：HandleRequest 层协议版本协商不影响既有语义 ----------

func TestInitialize_UnknownClientStillInitializes(t *testing.T) {
	s := NewServer(tooluse.NewToolRegistry())
	resp := send(t, s, "initialize", InitializeParams{
		ProtocolVersion: "1999-01-01",
		ClientInfo:      Implementation{Name: "legacy", Version: "0"},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if !s.IsInitialized() || s.SessionID() == "" {
		t.Error("server should be initialized with a session id")
	}
}
