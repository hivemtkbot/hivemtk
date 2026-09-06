package router

import (
	"strings"
	"testing"

	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
)

// TestSetup_WeComRoutes 验证企业微信路由已注册
func TestSetup_WeComRoutes(t *testing.T) {
	database := testutil.NewTestDB(t)
	dbutil.SetTestDB(database)
	defer dbutil.SetTestDB(nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	Setup(r, database)

	routes := r.Routes()

	expectedRoutes := map[string]bool{
		"/api/wecom/accounts":                    false,
		"/api/wecom/accounts/:id":                false,
		"/api/wecom/accounts/:id/sync-customers": false,
		"/api/wecom/customers":                   false,
		"/api/wecom/accounts/:id/sync-groups":    false,
		"/api/wecom/groups":                      false,
		"/api/wecom/accounts/:id/send-message":   false,
		"/api/wecom/messages":                    false,
		"/api/wecom/tags":                        false,
		"/api/wecom/accounts/:id/sync-tags":      false,
	}

	wecomCount := 0
	for _, route := range routes {
		if strings.HasPrefix(route.Path, "/api/wecom/") {
			wecomCount++
			if _, ok := expectedRoutes[route.Path]; ok {
				expectedRoutes[route.Path] = true
			}
		}
	}

	if wecomCount < 13 {
		t.Errorf("Expected at least 13 WeCom routes, got %d", wecomCount)
	}

	for path, found := range expectedRoutes {
		if !found {
			t.Errorf("Expected WeCom route %s to be registered", path)
		}
	}
}
