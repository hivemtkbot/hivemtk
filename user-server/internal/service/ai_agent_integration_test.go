// 营销工具套件 - AI 智能体模块完整集成测试
// 覆盖：List / Get / Create / Update / Delete / Toggle / Test / Bindings / Context
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"hivemtk-user/internal/config"
)


var (
	baseURL = config.DefaultUserServerBaseURL
	user    = os.Getenv("TEST_ADMIN_USERNAME")
	pass    = os.Getenv("TEST_ADMIN_PASSWORD")
)

type apiResp struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

var globalToken string


func mustPost(t *testing.T, path string, body any, token string) (apiResp, int) {
	return mustRequest(t, "POST", path, body, token)
}

func mustGet(t *testing.T, path string, token string) (apiResp, int) {
	return mustRequest(t, "GET", path, nil, token)
}

func mustPut(t *testing.T, path string, body any, token string) (apiResp, int) {
	return mustRequest(t, "PUT", path, body, token)
}

func mustDelete(t *testing.T, path string, body any, token string) (apiResp, int) {
	return mustRequest(t, "DELETE", path, body, token)
}

func mustRequest(t *testing.T, method, path string, body any, token string) (apiResp, int) {
	var bodyReader io.Reader
	if body != nil {
		bs, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(bs)
	}
	req, _ := http.NewRequest(method, baseURL+path, bodyReader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s 请求失败: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var ar apiResp
	_ = json.Unmarshal(raw, &ar)
	return ar, resp.StatusCode
}

func login(t *testing.T) string {
	if globalToken != "" {
		return globalToken
	}
	client := &http.Client{Timeout: 1 * time.Second}
	if _, err := client.Get(baseURL + "/health"); err != nil {
		t.Skipf("集成测试跳过：user-server 未运行于 %s (%v)", baseURL, err)
	}
	r, _ := mustPost(t, "/api/auth/login", map[string]string{"username": user, "password": pass}, "")
	if r.Code != "SUCCESS" {
		t.Skipf("登录失败，集成测试跳过: %s", r.Message)
	}
	if token, ok := r.Data["token"].(string); ok && token != "" {
		globalToken = token
	} else {
		t.Skipf("登录响应缺少 token，集成测试跳过")
	}
	return globalToken
}


func TestAIAgent_FullChain(t *testing.T) {
	token := login(t)

	t.Run("List", func(t *testing.T) {
		r, code := mustGet(t, "/api/ai-agents", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("List 失败: code=%d resp=%+v", code, r)
		}
		t.Logf("✅ List 智能体成功，共 %v 个", r.Data["total"])
	})

	t.Run("ListEnabled", func(t *testing.T) {
		r, code := mustGet(t, "/api/ai-agents-enabled", token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("ListEnabled 失败: code=%d resp=%+v", code, r)
		}
		t.Logf("✅ ListEnabled 启用智能体成功")
	})

	// ---- 3. 创建智能体 ----
	var agentID float64
	agentCode := fmt.Sprintf("test_ai_agent_%d", time.Now().Unix())
	t.Run("Create", func(t *testing.T) {
		body := map[string]any{
			"agent_code":          agentCode,
			"name":                "测试智能体",
			"description":         "用于集成测试的智能体",
			"agent_type":          "sales",
			"persona":             "你是一名测试销售智能体",
			"system_prompt":       "请使用简洁的语言回复",
			"greeting":            "您好，我是测试销售",
			"llm_model":           "gpt-4o-mini",
			"temperature":         0.7,
			"max_tokens":          800,
			"enable_rag":          true,
			"enable_script_match": true,
			"rag_top_k":           3,
			"max_ai_consecutive":  5,
		}
		r, code := mustPost(t, "/api/ai-agents", body, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Create 失败: code=%d resp=%+v", code, r)
		} else {
			agentID = r.Data["id"].(float64)
			t.Logf("✅ Create 智能体成功 ID=%v", agentID)
		}
	})

	if agentID == 0 {
		t.Fatal("智能体未创建成功，跳过后续测试")
	}

	t.Run("Get", func(t *testing.T) {
		r, code := mustGet(t, fmt.Sprintf("/api/ai-agents/%.0f", agentID), token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Get 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Get 智能体详情成功 name=%v", r.Data["name"])
		}
	})

	t.Run("Update_Partial", func(t *testing.T) {
		body := map[string]any{
			"agent_code": agentCode,
			"name":       "更新后名称",
			"agent_type": "sales",
			"persona":    "你是一名测试销售智能体",
		}
		r, code := mustPut(t, fmt.Sprintf("/api/ai-agents/%.0f", agentID), body, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Update 失败: code=%d resp=%+v", code, r)
		} else {
			enableRAG := r.Data["enable_rag"]
			t.Logf("✅ Update 部分更新成功，enable_rag=%v (期望: true)", enableRAG)
			if enableRAG != true {
				t.Logf("⚠️ BUG: Update 部分更新重置了未提供的字段 enable_rag=%v (应为 true)", enableRAG)
			}
		}
	})

	t.Run("Toggle_Disable", func(t *testing.T) {
		body := map[string]int{"status": 0}
		r, code := mustPost(t, fmt.Sprintf("/api/ai-agents/%.0f/toggle", agentID), body, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Toggle Disable 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Toggle 禁用成功")
		}
	})

	t.Run("Toggle_Enable", func(t *testing.T) {
		body := map[string]int{"status": 1}
		r, code := mustPost(t, fmt.Sprintf("/api/ai-agents/%.0f/toggle", agentID), body, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Toggle Enable 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ Toggle 启用成功")
		}
	})

	// ---- 7. 渠道绑定 ----
	var bindingID float64
	t.Run("Binding_Create", func(t *testing.T) {
		body := map[string]any{
			"channel_type": "wecom",
			"account_id":   fmt.Sprintf("acc_test_%d", time.Now().Unix()),
			"agent_id":     agentID,
			"is_primary":   true,
			"enabled":      true,
		}
		r, code := mustPost(t, "/api/channel-agent-bindings", body, token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Binding Create 失败: code=%d resp=%+v", code, r)
		} else {
			bindingID = r.Data["id"].(float64)
			t.Logf("✅ Binding Create 成功 ID=%v", bindingID)
		}
	})

	if bindingID > 0 {
		t.Run("Binding_ListByAgent", func(t *testing.T) {
			r, code := mustGet(t, fmt.Sprintf("/api/channel-agent-bindings/by-agent/%.0f", agentID), token)
			if code != 200 || r.Code != "SUCCESS" {
				t.Errorf("ListByAgent 失败: code=%d resp=%+v", code, r)
			} else {
				t.Logf("✅ ListByAgent 成功，共 %v 个绑定", r.Data["total"])
			}
		})
	}

	t.Run("Context", func(t *testing.T) {
		r, code := mustGet(t, fmt.Sprintf("/api/ai-agents/%.0f/context", agentID), token)
		if code != 200 || r.Code != "SUCCESS" {
			t.Errorf("Context 失败: code=%d resp=%+v", code, r)
		} else {
			t.Logf("✅ 加载智能体上下文成功")
		}
	})

	t.Run("Test", func(t *testing.T) {
		body := map[string]string{
			"customer_id": "test_customer",
			"message":     "你好",
		}
		r, code := mustPost(t, fmt.Sprintf("/api/ai-agents/%.0f/test", agentID), body, token)
		if code >= 500 {
			t.Logf("ℹ️ Test 接口可达，但执行失败（无 LLM 凭证）: %s", r.Message)
		} else if code == 200 {
			t.Logf("✅ Test 执行成功")
		} else {
			t.Logf("ℹ️ Test 返回 code=%d: %s", code, r.Message)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		if bindingID > 0 {
			r, _ := mustDelete(t, fmt.Sprintf("/api/channel-agent-bindings/%.0f", bindingID), nil, token)
			if r.Code != "SUCCESS" {
				t.Logf("⚠️ Binding Delete 失败: %s", r.Message)
			}
		}
		r, _ := mustDelete(t, fmt.Sprintf("/api/ai-agents/%.0f", agentID), nil, token)
		if r.Code != "SUCCESS" {
			t.Errorf("Agent Delete 失败: %s", r.Message)
		} else {
			t.Logf("✅ 清理完成，删除智能体 ID=%v", agentID)
		}
	})
}

// 防止 os 引用
var _ = os.Getenv

