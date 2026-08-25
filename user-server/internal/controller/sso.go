// 企业 SSO 登录控制器（2026-08-15 M3-P1-E3）
//
// 提供 3 个公开端点（无需登录态，供浏览器 OAuth 授权码流程使用）：
//   - GET /api/sso/providers      列出已启用登录方式（登录页按钮）
//   - GET /api/sso/login/:provider 发起登录：生成 state/nonce/PKCE 并 302 到 IdP
//   - GET /api/sso/callback/:provider 处理 IdP 回调：换 token → 关联/创建用户 → 签发本地 JWT
//
// 安全设计：
//   - state 存 HttpOnly+Secure cookie，回调时严格比对（防 CSRF）
//   - code_verifier 存 cookie（PKCE，防授权码截获重放）
//   - 回调成功默认 302 到前端（PUBLIC_BASE_URL）携带 token；?format=json 返回 JSON 便于测试/嵌入
package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/sso"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// SSOCookieTTL state/nonce/verifier cookie 有效期（5 分钟，与 sso 包默认一致）
const SSOCookieTTL = 5 * time.Minute

// SSOController 企业 SSO 登录控制器
type SSOController struct {
	ssoService *service.SSOService
	frontend   string
}

// NewSSOController 创建 SSO 控制器（生产入口，注入已构建的 SSO 服务）
func NewSSOController(ssoService *service.SSOService) *SSOController {
	return &SSOController{
		ssoService: ssoService,
		frontend:   config.GetPublicBaseURL(),
	}
}

// ListProviders 列出已启用的 SSO 登录方式
func (c *SSOController) ListProviders(ctx *gin.Context) {
	if !c.ssoService.Enabled() {
		response.Success(ctx, gin.H{"enabled": false, "providers": []service.ProviderInfo{}}, "未启用企业登录")
		return
	}
	response.Success(ctx, gin.H{
		"enabled":   true,
		"providers": c.ssoService.ListProviders(),
	}, "获取登录方式成功")
}

// Login 发起 SSO 登录：生成 state + nonce + PKCE verifier，重定向到 IdP
func (c *SSOController) Login(ctx *gin.Context) {
	if !c.ssoService.Enabled() {
		response.Error(ctx, http.StatusForbidden, service.ErrSSONotEnabled.Error())
		return
	}
	provider := ctx.Param("provider")
	adapter, ok := c.ssoService.Adapter(provider)
	if !ok {
		response.Error(ctx, http.StatusNotFound, "未找到该登录方式")
		return
	}

	state := sso.RandString(32)
	nonce := sso.RandString(32)
	verifier := sso.RandString(64)

	setSSOCookie(ctx, "sso_state", state)
	setSSOCookie(ctx, "sso_nonce", nonce)
	setSSOCookie(ctx, "sso_verifier", verifier)

	oidc := adapter.OIDC()
	if err := oidc.EnsureFresh(ctx.Request.Context()); err != nil {
		clearSSOCookies(ctx)
		response.Error(ctx, http.StatusInternalServerError, "获取授权配置失败: "+err.Error())
		return
	}
	authURL, err := oidc.BuildAuthURL(state, nonce, verifier)
	if err != nil {
		clearSSOCookies(ctx)
		response.Error(ctx, http.StatusInternalServerError, "构建授权地址失败: "+err.Error())
		return
	}
	ctx.Redirect(http.StatusFound, authURL)
}

// Callback 处理 IdP 回调
func (c *SSOController) Callback(ctx *gin.Context) {
	provider := ctx.Param("provider")

	state := ctx.Query("state")
	expectedState, errState := ctx.Cookie("sso_state")
	if errState != nil || state == "" || state != expectedState {
		response.Error(ctx, http.StatusBadRequest, service.ErrSSOInvalidState.Error())
		return
	}

	verifier, _ := ctx.Cookie("sso_verifier")

	result, err := c.ssoService.HandleCallback(ctx.Request.Context(), provider, ctx.Query("code"), verifier)
	clearSSOCookies(ctx)
	if err != nil {
		c.callbackError(ctx, err)
		return
	}

	if ctx.Query("format") != "json" {
		target := c.resolveRedirectTarget(ctx)
		if target != "" {
			sep := "?"
			if strings.Contains(target, "?") {
				sep = "&"
			}
			ctx.Redirect(http.StatusFound, fmt.Sprintf("%s%stoken=%s", target, sep, url.QueryEscape(result.Token)))
			return
		}
	}

	response.Success(ctx, gin.H{
		"token":       result.Token,
		"user":        result.User,
		"expires":     result.Expires,
		"provider":    result.Provider,
		"is_new_user": result.IsNewUser,
	}, "登录成功")
}

// resolveRedirectTarget 解析回调成功后的前端跳转地址
//
// 优先级：
//  1. 请求参数 redirect（仅允许相对路径，防止开放重定向）
//  2. 配置的 PUBLIC_BASE_URL
//  3. 空（返回 JSON）
func (c *SSOController) resolveRedirectTarget(ctx *gin.Context) string {
	if r := strings.TrimSpace(ctx.Query("redirect")); r != "" {
		if strings.HasPrefix(r, "/") && !strings.HasPrefix(r, "//") {
			return r
		}
		return ""
	}
	return strings.TrimRight(c.frontend, "/")
}

// callbackError 回调失败统一响应（语义化状态码）
func (c *SSOController) callbackError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSSONotEnabled):
		response.Error(ctx, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrSSOProviderNotFound):
		response.Error(ctx, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrSSOMissingCode):
		response.Error(ctx, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrSSOUserNotBound):
		response.Error(ctx, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrSSOUserDisabled):
		response.Error(ctx, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrSSOInvalidState):
		response.Error(ctx, http.StatusBadRequest, err.Error())
	default:
		response.Error(ctx, http.StatusUnauthorized, "企业登录失败: "+err.Error())
	}
}

// setSSOCookie 设置 SSO 流程 cookie（HttpOnly + Secure，5 分钟）
func setSSOCookie(ctx *gin.Context, name, value string) {
	ctx.SetCookie(name, value, int(SSOCookieTTL.Seconds()), "/", "", true, true)
}

// clearSSOCookies 清理 SSO 流程 cookie
func clearSSOCookies(ctx *gin.Context) {
	ctx.SetCookie("sso_state", "", -1, "/", "", true, true)
	ctx.SetCookie("sso_nonce", "", -1, "/", "", true, true)
	ctx.SetCookie("sso_verifier", "", -1, "/", "", true, true)
}

