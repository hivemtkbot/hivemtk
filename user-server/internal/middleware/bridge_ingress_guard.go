package middleware

import (
	"context"
	"crypto/subtle"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/repository"
)

// bridgeTokenCandidates 返回当前有效的桥接 token 集合（当前值 + 轮换灰度旧值）。
// 双值灰度：BRIDGE_INGEST_TOKEN_PREV 在一个发布周期内与主 token 等效，
// 扩展端滚动更新零中断；窗口结束后移除 PREV 即完成轮换。
// bridgeTokenCandidates 运行时凭证候选集：
// 优先 DB system_config_kv（管理端轮换写入），回退环境变量；PREV 双值灰度。
func bridgeTokenCandidates() []string {
	out := []string{}
	kvRepo := repository.NewSystemConfigKVRepository()
	ctx := context.Background()
	if v, err := kvRepo.Get(ctx, "bridge_ingest_token"); err == nil && strings.TrimSpace(v) != "" {
		out = append(out, strings.TrimSpace(v))
	}
	if v, err := kvRepo.Get(ctx, "bridge_ingest_token_prev"); err == nil && strings.TrimSpace(v) != "" {
		out = append(out, strings.TrimSpace(v))
	}
	if len(out) > 0 {
		return out
	}
	if v := strings.TrimSpace(os.Getenv("BRIDGE_INGEST_TOKEN")); v != "" {
		out = append(out, v)
	}
	if v := strings.TrimSpace(os.Getenv("BRIDGE_INGEST_TOKEN_PREV")); v != "" {
		out = append(out, v)
	}
	return out
}

// extractBridgeToken 提取请求凭证：优先 X-Bridge-Token header；
// EventSource 无法携带自定义 header，SSE 场景允许 ?bridge_token= 一次性透传。
func extractBridgeToken(c *gin.Context) string {
	if v := c.GetHeader("X-Bridge-Token"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := c.Query("bridge_token"); v != "" {
		if dec, err := url.QueryUnescape(v); err == nil {
			return strings.TrimSpace(dec)
		}
		return strings.TrimSpace(v)
	}
	return ""
}

// BridgeIngressGuard 浏览器扩展桥接通道凭证守卫（v3 审计 P0-1）。
//
// 策略：
//   - 配置 BRIDGE_INGEST_TOKEN 后：请求必须携带有效凭证（header 或 SSE query），
//     常量时间比对通过；支持 PREV 双值灰度
//   - 未配置：默认拒绝；开发/联调可显式设置 BRIDGE_INGEST_AUTH=off 放行
func BridgeIngressGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		candidates := bridgeTokenCandidates()
		if len(candidates) == 0 {
			if strings.EqualFold(os.Getenv("BRIDGE_INGEST_AUTH"), "off") {
				c.Next()
				return
			}
			response.Error(c, 503, "桥接通道未配置 BRIDGE_INGEST_TOKEN；联调环境请显式设置 BRIDGE_INGEST_AUTH=off")
			c.Abort()
			return
		}

		got := extractBridgeToken(c)
		if got == "" {
			response.Error(c, 401, "缺少 X-Bridge-Token")
			c.Abort()
			return
		}

		ok := false
		for _, want := range candidates {
			if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
				ok = true
				break
			}
		}
		if !ok {
			response.Error(c, 401, "bridge token 无效")
			c.Abort()
			return
		}

		// query 透传场景：凭证已消费，剥离 URL 参数防止进入代理/访问日志与浏览器历史
		if c.Query("bridge_token") != "" {
			q := c.Request.URL.Query()
			q.Del("bridge_token")
			c.Request.URL.RawQuery = q.Encode()
		}
		c.Next()
	}
}
