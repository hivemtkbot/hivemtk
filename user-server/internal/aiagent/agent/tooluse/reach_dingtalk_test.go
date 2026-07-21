package tooluse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marketing/internal/service"
)

// TestIntegrationReachAdapter_SendDingTalk_Real 验证 DingTalk 渠道从 NoOp 变为真实可达：
// 注入 DingTalkService 后，SendDingTalk 经钉钉群机器人 webhook 真实出站。
func TestIntegrationReachAdapter_SendDingTalk_Real(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "errmsg": "ok"})
	}))
	defer srv.Close()

	a := &IntegrationReachAdapter{dingtalk: service.NewDingTalkService()}
	msgID, err := a.SendDingTalk(context.Background(), srv.URL, "text", "hello dingtalk")
	if err != nil {
		t.Fatalf("SendDingTalk 真实出站失败: %v", err)
	}
	if !strings.HasPrefix(msgID, "dingtalk-") {
		t.Fatalf("msgID 格式异常: %q", msgID)
	}
}

// TestIntegrationReachAdapter_SendDingTalk_NilService 验证未注入服务时返回 sentinel error。
func TestIntegrationReachAdapter_SendDingTalk_NilService(t *testing.T) {
	a := &IntegrationReachAdapter{}
	_, err := a.SendDingTalk(context.Background(), "https://oapi.dingtalk.com/robot/send?access_token=x", "text", "hi")
	if err == nil {
		t.Fatal("dingtalk 未注入应返回 ErrIntegrationServiceNotConfigured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("应返回 ErrIntegrationServiceNotConfigured，实际: %v", err)
	}
}
