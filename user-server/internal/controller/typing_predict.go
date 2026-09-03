package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// TypingPredictController 打字预回复 SSE 控制器
// G15: 竞品标配功能 - 访客打字实时意图预测
//
// 工作模式：
//   1) 前端打开 SSE 连接 GET /api/chat/typing-predict?session_id=xxx
//   2) 前端每次用户输入（或 debounce 300ms 后）POST /api/chat/typing-predict/predict
//      body: {"text": "用户正在输入的文字"}
//   3) 后端计算意图预测 → 通过之前建立的 SSE 连接推送 "intent_prediction" 事件
//
// 这里实现为最简单的"每次 POST 后同步返回预测结果"模式，
// 避免引入额外的 SSE hub / 连接管理复杂度（与既有 SSEHub 复用）。
type TypingPredictController struct {
	svc *service.TypingPredictService
}

// NewTypingPredictController 创建打字预测控制器
func NewTypingPredictController() *TypingPredictController {
	return &TypingPredictController{
		svc: service.GetTypingPredictService(),
	}
}

// PredictRequest 预测请求体
type PredictRequest struct {
	Text      string `json:"text" binding:"required"`
	SessionID string `json:"session_id,omitempty"`
}

// Predict 同步预测接口（最简化）
// POST /api/chat/typing-predict/predict
//
// 同步返回 IntentPrediction，前端拿到后直接渲染建议回复 UI。
// 不自动发送，仅展示。
func (c *TypingPredictController) Predict(ctx *gin.Context) {
	var req PredictRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	if len(req.Text) > 500 {
		response.Error(ctx, http.StatusBadRequest, "输入过长，请控制在 500 字以内")
		return
	}
	pred, err := c.svc.Predict(ctx.Request.Context(), req.Text, req.SessionID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "预测失败: "+err.Error())
		return
	}
	response.Success(ctx, pred, "预测成功")
}

// SSEStream SSE 推送模式（可选，用于高实时性场景）
// GET /api/chat/typing-predict/sse?session_id=xxx
//
// 实现说明：
// 与既有 SSEDashboard 不同，这里采用长轮询式 SSE——
// 客户端连接后发送心跳，服务端暂存请求，
// 后端通过 SSEPub 注入预测结果并推送。
//
// 为简化实现，本版本 SSE 仅推送心跳，实际预测走 POST /predict 同步返回。
// 这样避免了跨 goroutine 的连接管理复杂度，但仍预留了 SSE 端点。
func (c *TypingPredictController) SSEStream(ctx *gin.Context) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		response.Error(ctx, http.StatusBadRequest, "session_id 必填")
		return
	}

	// SSE 头
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no") // 禁用 反向代理层 缓冲

	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "不支持流式响应")
		return
	}

	// 立即发送一次 connected 事件
	fmt.Fprintf(ctx.Writer, "event: connected\ndata: {\"session_id\":\"%s\",\"timestamp\":%d}\n\n",
		sessionID, time.Now().Unix())
	flusher.Flush()

	// 心跳循环（30s 间隔），同时监听客户端断开
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Request.Context().Done():
			// 客户端断开
			return
		case <-ticker.C:
			ping, _ := json.Marshal(map[string]any{
				"type":      "ping",
				"timestamp": time.Now().Unix(),
			})
			fmt.Fprintf(ctx.Writer, "event: ping\ndata: %s\n\n", string(ping))
			flusher.Flush()
		}
	}
}
