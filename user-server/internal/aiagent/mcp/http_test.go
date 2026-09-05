package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

func TestNegotiateProtocolVersion_Matrix(t *testing.T) {
	cases := []struct {
		requested string
		want      string
	}{
		{"2025-06-18", "2025-06-18"},
		{"2025-11-25", "2025-11-25"},
		{"2025-03-26", "2025-11-25"},
		{"2030-01-01", "2025-11-25"},
		{"", "2025-11-25"},
		{"garbage", "2025-11-25"},
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

	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("missing session id: status = %d, want 400", rec.Code)
	}

	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, "bogus-session-id")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("wrong session id: status = %d, want 400", rec.Code)
	}

	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(pingBody))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("valid session id: status = %d, want 200", rec.Code)
	}
}

func TestHTTPHandler_AcceptValidation(t *testing.T) {
	h := NewHTTPHandler(tooluse.NewToolRegistry())
	body := `{"jsonrpc":"2.0","id":"1","method":"ping"}`

	cases := []struct {
		accept string
		want   int
	}{
		{"", 406},
		{"text/html", 406},
		{"application/json", 400},
		{"text/event-stream", 400},
		{"text/html, application/json;q=0.9", 400},
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

	initBody := `{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-11-25"}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(initBody))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	sid := rec.Header().Get(HeaderMcpSessionID)

	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderMcpSessionID, sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Errorf("notification status = %d, want 202", rec.Code)
	}
}

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
