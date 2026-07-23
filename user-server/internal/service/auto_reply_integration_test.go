// 营销工具套件 - 自动回复系统完整集成测试
// 覆盖：通用 / 闲鱼 / 小红书 / TikTok / RAG 知识库 / 知识库工作台 / 商户视角
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// 测试入口：所有自动回复系统全链路
// ============================================================================

func TestAutoReply_FullChain(t *testing.T) {
	token := login(t)

	// ---- 1. 通用自动回复 ----
	t.Run("Common_AutoReply", func(t *testing.T) {
		// 1.1 账号列表
		r, code := mustGet(t, "/api/auto-reply/accounts", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("GetAccounts 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.GetAccounts 成功: %v", r.Data)
		}

		// 1.2 获取规则
		r, code = mustGet(t, "/api/auto-reply/rule", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("GetRule 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.GetRule 成功: %v", r.Data)
		}

		// 1.3 日志
		r, code = mustGet(t, "/api/auto-reply/logs?page=1&page_size=10", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListLogs 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.ListLogs 成功: %v", r.Data)
		}

		// 1.4 登录状态
		r, code = mustGet(t, "/api/auto-reply/login-status", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("LoginStatus 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.LoginStatus 成功: %v", r.Data)
		}

		// 1.5 无头模式
		r, code = mustGet(t, "/api/auto-reply/headless", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("GetHeadlessMode 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.GetHeadlessMode 成功")
		}

		// 1.6 调试状态
		r, code = mustGet(t, "/api/auto-reply/debug/status", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("GetDebugStatus 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Common.GetDebugStatus 成功")
		}
	})

	// ---- 2. 自动回复管理器 ----
	var ruleID float64
	t.Run("Manager", func(t *testing.T) {
		// 2.1 规则列表
		r, code := mustGet(t, "/api/auto-reply/rules?page=1&page_size=10", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListRules 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Manager.ListRules 成功: %v", r.Data)
		}

		// 2.2 统计
		r, code = mustGet(t, "/api/auto-reply/statistics", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Statistics 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Manager.Statistics 成功: %v", r.Data)
		}

		// 2.3 限流统计
		r, code = mustGet(t, "/api/auto-reply/rate-limit-stats", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("RateLimitStats 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Manager.RateLimitStats 成功")
		}

		// 2.4 并发统计
		r, code = mustGet(t, "/api/auto-reply/concurrent-stats", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ConcurrentStats 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Manager.ConcurrentStats 成功")
		}

		// 2.5 创建规则
		keyword := fmt.Sprintf("test_kw_%d", time.Now().Unix())
		createBody := map[string]any{
			"keyword":       keyword,
			"reply_type":    "text",
			"reply_content": "您好，欢迎咨询！",
			"platform":      "douyin",
			"priority":      10,
			"is_active":     true,
			"frequency":     1,
			"daily_limit":   100,
		}
		r, code = mustPost(t, "/api/auto-reply/rules", createBody, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("CreateRule 失败: code=%d resp=%+v", code, r)
		} else {
			if id, ok := r.Data["id"]; ok {
				ruleID, _ = id.(float64)
			} else if id, ok := r.Data["data"].(map[string]any)["id"]; ok {
				ruleID, _ = id.(float64)
			}
			t.Logf("✅ Manager.CreateRule 成功 ID=%v", ruleID)
		}

		// 2.6 测试匹配
		r, code = mustPost(t, "/api/auto-reply/test-matching", map[string]any{
			"message": keyword,
		}, token)
		t.Logf("  test-matching 返回: code=%d resp=%v", code, r.Data)

		// 2.7 模拟消息
		r, code = mustPost(t, "/api/auto-reply/simulate-message", map[string]any{
			"message":  keyword,
			"platform": "douyin",
		}, token)
		t.Logf("  simulate-message 返回: code=%d resp=%v", code, r.Data)
	})

	// ---- 3. 闲鱼自动回复（两种 URL 格式） ----
	t.Run("Xianyu", func(t *testing.T) {
		endpoints := map[string]string{
			"GET /api/xianyu-auto-reply/accounts":     "/api/xianyu-auto-reply/accounts",
			"GET /api/xianyu-auto-reply/rule":         "/api/xianyu-auto-reply/rule",
			"GET /api/xianyu-auto-reply/logs":         "/api/xianyu-auto-reply/logs",
			"GET /api/xianyu-auto-reply/login-status": "/api/xianyu-auto-reply/login-status",
			"GET /api/xianyu/auto-reply/accounts":     "/api/xianyu/auto-reply/accounts",
			"GET /api/xianyu/auto-reply/rules":        "/api/xianyu/auto-reply/rules",
			"GET /api/xianyu/auto-reply/logs":         "/api/xianyu/auto-reply/logs",
			"GET /api/xianyu/auto-reply/login/status": "/api/xianyu/auto-reply/login/status",
			"GET /api/xianyu/auto-reply/health":       "/api/xianyu/auto-reply/health",
		}
		for label, path := range endpoints {
			r, code := mustGet(t, path, token)
			if code != 200 || r.Code != "SUCCESS" {
				t.Errorf("❌ %s 失败: code=%d resp=%+v", label, code, r)
			} else {
				t.Logf("✅ %s 成功", label)
			}
		}
	})

	// ---- 4. 小红书自动回复 ----
	t.Run("Xiaohongshu", func(t *testing.T) {
		endpoints := []string{
			"/api/xiaohongshu/auto-reply/accounts",
			"/api/xiaohongshu/auto-reply/rules",
			"/api/xiaohongshu/auto-reply/logs",
			"/api/xiaohongshu/auto-reply/login-status",
			"/api/xiaohongshu/auto-reply/health",
		}
		for _, path := range endpoints {
			r, code := mustGet(t, path, token)
			if code != 200 || r.Code != "SUCCESS" {
				t.Errorf("❌ %s 失败: code=%d resp=%+v", path, code, r)
			} else {
				t.Logf("✅ %s 成功", path)
			}
		}
	})

	// ---- 5. TikTok 自动回复 ----
	t.Run("TikTok", func(t *testing.T) {
		endpoints := []string{
			"/api/tiktok/auto-reply/accounts",
			"/api/tiktok/auto-reply/rule",
			"/api/tiktok/auto-reply/logs",
		}
		for _, path := range endpoints {
			r, code := mustGet(t, path, token)
			if code != 200 || r.Code != "SUCCESS" {
				t.Errorf("❌ %s 失败: code=%d resp=%+v", path, code, r)
			} else {
				t.Logf("✅ %s 成功", path)
			}
		}
	})

	// ---- 6. RAG 知识库配置 ----
	// 注意：RAG 控制器返回的 code 是数字 200 (与标准 SUCCESS 字符串不同)
	t.Run("RAGConfig", func(t *testing.T) {
		// 6.1 产品列表
		r, code := mustGet(t, "/api/rag-config/products", token)
		if code != 200 {
			t.Errorf("ListRagProducts 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ RAGConfig.ListRagProducts 成功: items=%v", r.Data["items"])
		}

		// 6.2 创建产品
		productName := fmt.Sprintf("集成测试产品-%d", time.Now().Unix())
		r, code = mustPost(t, "/api/rag-config/products", map[string]any{
			"name":         productName,
			"description":  "集成测试自动创建",
			"category":     "test",
			"vector_table": fmt.Sprintf("test_vec_%d", time.Now().Unix()),
		}, token)
		if code != 200 {
			t.Logf("⚠️ CreateRagProduct 返回: code=%d resp=%v", code, r.Data)
		} else {
			t.Logf("✅ RAGConfig.CreateRagProduct 成功")
		}

		// 6.3 查询（缺少必填 product_id 应 400）
		r, code = mustPost(t, "/api/rag-config/query", map[string]any{
			"query": "测试查询",
			"top_k": 3,
		}, token)
		t.Logf("  QueryRAG (无 product_id): code=%d, msg=%s", code, r.Message)
	})

	// ---- 7. 知识库管理 ----
	t.Run("KnowledgeBase", func(t *testing.T) {
		// 2026-07-18：路由统一收敛到 /api/rag/*（MASTER §4.3 禁止 /api/knowledge-base/* 历史前缀）
		r, code := mustGet(t, "/api/rag/documents?page=1&page_size=20", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListDocuments 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ KnowledgeBase.ListDocuments 成功: %v", r.Data)
		}
	})

	// ---- 8. 知识库工作台 ----
	t.Run("KnowledgeWorkspace", func(t *testing.T) {
		r, code := mustGet(t, "/api/knowledge/documents?page=1&page_size=20", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListDocuments 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ KnowledgeWorkspace.ListDocuments 成功")
		}
	})

	// ---- 9. 商户视角知识库 ----
	t.Run("KnowledgeMerchant", func(t *testing.T) {
		r, code := mustGet(t, "/api/knowledge-merchant/feedbacks?page=1&page_size=20", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListFeedbacks 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ KnowledgeMerchant.ListFeedbacks 成功")
		}
	})

	// ---- 10. 清理 ----
	t.Run("Cleanup", func(t *testing.T) {
		if ruleID > 0 {
			r, code := mustDelete(t, fmt.Sprintf("/api/auto-reply/rules/%v", ruleID), nil, token)
			if code != 200 {
				t.Logf("⚠️ 清理规则失败: code=%d resp=%v", code, r.Data)
			} else {
				t.Logf("✅ 已清理规则 ID=%v", ruleID)
			}
		}
	})
	_ = ctx
	_ = http.MethodGet
	_ = strings.TrimSpace
	_ = json.Number("")
}
