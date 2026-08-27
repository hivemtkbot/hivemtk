package controller

import (
	"io"
	"net/http"

	"hivemtk-user/internal/aiagent/mcp"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)

// MCPController 处理 MCP（Model Context Protocol 2025-06-18）JSON-RPC over HTTP 请求。
//
// 原 router.go 中 bridgeWS.POST("/mcp", ...) 的内联 handler 抽出至此 controller。
//
// 设计要点：
//   - 每次请求新建 mcp.Server：v3 审计发现 Server 不能是单例（initialize 状态
//     会被并发请求污染——会话握手、capabilities 协商是 per-request 状态）。
//   - 当前 mcp.NewServer(nil) 暂未接入 tooluse registry（initialize/ping 已工作）；
//     后续挂上 tooluse registry 时只需替换参数。
//   - 直接写 c.Writer + c.Header：MCP 响应体是 JSON-RPC 2.0 原始字节，
//     不应经过 response 包装层。
type MCPController struct{}

// NewMCPController 构造 MCP controller。
func NewMCPController() *MCPController {
	return &MCPController{}
}

// Handle POST /api/mcp。
//
// 流程：
//  1. 读 request body（限制最大 1MB，超出按 400 处理）
//  2. 新建 mcp.Server（per-request）
//  3. 调用 HandleRequest 拿 JSON-RPC 响应字节
//  4. 写 Content-Type: application/json + 200 OK
//
// 错误处理：
//   - body 读取失败 → 400 + 错误响应
//   - mcp 内部错误：服务端已在 HandleRequest 内部返回 JSON-RPC error 响应，
//     此处不重复报错（HTTP 层仅保证 200 OK + JSON-RPC 语义错误码）。
func (c *MCPController) Handle(ctx *gin.Context) {
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxBody+1))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}
	if int64(len(body)) > maxBody {
		ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}

	mcpSrv := mcp.NewServer(nil)
	resp, err := mcpSrv.HandleRequest(ctx.Request.Context(), body)
	if err != nil {

		logger.Ctx(ctx.Request.Context()).Warn().Err(err).Msg("[MCP] handle request failed")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Header("Content-Type", "application/json")
	ctx.Status(http.StatusOK)
	if _, werr := ctx.Writer.Write(resp); werr != nil {
		logger.Ctx(ctx.Request.Context()).Warn().Err(werr).Msg("[MCP] write response")
	}
}
