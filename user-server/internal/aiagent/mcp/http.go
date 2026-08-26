package mcp

// TL-2（MASTER_COMPETITIVE_DECISIONS M13）：MCP Streamable HTTP 传输层最小子集。
//
// 覆盖项：
//   - Accept 头校验：必须含 application/json 或 text/event-stream 至少其一，否则 406
//   - initialize 成功 → 生成 Mcp-Session-Id（可见 ASCII）响应头
//   - 后续请求校验 Mcp-Session-Id：缺失/不匹配 → 400
//
// 无推送需求：不实现 GET/SSE 流（spec 允许服务器不提供 server→client 流，
// 仅以 POST 响应式返回），属合法子集。

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// HeaderMcpSessionID MCP 会话标识响应头/请求头名
const HeaderMcpSessionID = "Mcp-Session-Id"

// sessionIdleTTL 会话空闲过期时间（超时后惰性回收）
const sessionIdleTTL = 30 * time.Minute

// HTTPHandler MCP Streamable HTTP 最小子集处理器。
//
// 每个会话持有一个独立 Server 实例（避免 initialize 状态跨客户端污染），
// 由 SessionManager 按会话 ID 存取。
type HTTPHandler struct {
	registry *tooluse.ToolRegistry

	mu       sync.Mutex
	sessions map[string]*mcpHTTPSession
}

type mcpHTTPSession struct {
	server   *Server
	lastSeen time.Time
}

// NewHTTPHandler 构造 MCP HTTP handler
func NewHTTPHandler(registry *tooluse.ToolRegistry) *HTTPHandler {
	return &HTTPHandler{
		registry: registry,
		sessions: make(map[string]*mcpHTTPSession),
	}
}

// ServeHTTP 实现 http.Handler（仅 POST）
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed: MCP streamable HTTP requires POST", http.StatusMethodNotAllowed)
		return
	}

	// TL-2.3：Accept 头校验（缺失/不含合法类型 → 406）
	if !acceptHeaderOK(r.Header.Get("Accept")) {
		http.Error(w, "Not Acceptable: client must accept application/json or text/event-stream", http.StatusNotAcceptable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req JSONRPCRequest
	_ = json.Unmarshal(body, &req) // 解析失败由 HandleRequest 返回 parse error

	if req.Method == "initialize" {
		h.handleInitializeRequest(w, r, body)
		return
	}

	// TL-2.2：非 initialize 请求必须携带有效 Mcp-Session-Id
	srv := h.lookupSession(r.Header.Get(HeaderMcpSessionID))
	if srv == nil {
		http.Error(w, "Bad Request: missing or invalid "+HeaderMcpSessionID+" header", http.StatusBadRequest)
		return
	}
	resp, _ := srv.HandleRequest(r.Context(), body)
	h.writeJSONRPCResponse(w, resp)
}

// handleInitializeRequest 处理 initialize：成功则建立会话并下发 Mcp-Session-Id
func (h *HTTPHandler) handleInitializeRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	srv := NewServer(h.registry)
	resp, _ := srv.HandleRequest(r.Context(), body)

	// 仅在 initialize 成功时建立会话（失败响应不签发会话 ID）
	var rpcResp JSONRPCResponse
	if json.Unmarshal(resp, &rpcResp) == nil && rpcResp.Error == nil && srv.SessionID() != "" {
		h.storeSession(srv.SessionID(), srv)
		w.Header().Set(HeaderMcpSessionID, srv.SessionID())
	}
	h.writeJSONRPCResponse(w, resp)
}

// writeJSONRPCResponse 写出 JSON-RPC 响应体；通知类请求无响应体时返回 202
func (h *HTTPHandler) writeJSONRPCResponse(w http.ResponseWriter, resp []byte) {
	if len(resp) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

// acceptHeaderOK 校验 Accept 头是否含 application/json 或 text/event-stream 至少其一
func acceptHeaderOK(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		mt := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mt == "application/json" || mt == "text/event-stream" {
			return true
		}
	}
	return false
}

// storeSession 登记会话并顺带清理空闲过期会话
func (h *HTTPHandler) storeSession(id string, srv *Server) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for sid, s := range h.sessions {
		if now.Sub(s.lastSeen) > sessionIdleTTL {
			delete(h.sessions, sid)
		}
	}
	h.sessions[id] = &mcpHTTPSession{server: srv, lastSeen: now}
}

// lookupSession 按 ID 取会话；缺失/不匹配返回 nil 并刷新命中会话的活跃时间
func (h *HTTPHandler) lookupSession(id string) *Server {
	if id == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return nil
	}
	s.lastSeen = time.Now()
	return s.server
}
