package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"marketing/internal/aiagent/agent/portcontract"
)

// ============================================================================
// 售后回写电商平台客户端（HTTP，配置驱动）
//
// 客服系统不是电商，售后单（退款/退货）必须由电商执行落地。
// 客服侧 only 负责发起 + 本地落库 + 状态跟踪；发起后通过本客户端把请求推给电商，
// 用返回的电商侧售后单号(external_id) + 状态更新本地记录。
//
// 凭证/基地址来自数据库 system_config_kv[agent.tool_integrations].after_sale（见
// tool_integration_config.go），由 NewAfterSaleExternalClientFromConfig 按配置构造。
// 未配置（baseURL 为空）时 Configured()=false，AfterSaleService.Create 走 best-effort
// 本地落库（状态等待电商 Webhook 回推或定时拉取），与旧行为一致。
//
// 期望响应体：{ "external_id": "...", "status": "..." }
// ============================================================================

// AfterSaleExternalResult 电商侧回写结果
type AfterSaleExternalResult struct {
	ExternalID string // 电商侧售后单号
	Status     string // 电商侧状态
}

// AfterSaleExternalClient 售后回写电商平台客户端接口
type AfterSaleExternalClient interface {
	// Create 把售后请求推给电商执行落地，返回电商侧售后单号 + 状态。
	// 未配置 / 不可达时返回 (nil, nil)，由调用方走 best-effort 本地落库。
	Create(ctx context.Context, req *portcontract.AfterSaleRequest) (*AfterSaleExternalResult, error)
	// Configured 回写接口是否已配置（有基地址即视为已配置）。
	Configured() bool
}

// HTTPAfterSaleClient 标准售后回写 HTTP 客户端，实现 AfterSaleExternalClient
type HTTPAfterSaleClient struct {
	baseURL string
	key     string
	secret  string
	http    *http.Client
}

// NewAfterSaleExternalClientFromConfig 按数据库中的售后集成配置构造。baseURL 为空表示未配置回写接口。
func NewAfterSaleExternalClientFromConfig(cfg AfterSaleIntegration) *HTTPAfterSaleClient {
	return &HTTPAfterSaleClient{
		baseURL: cfg.BaseURL,
		key:     cfg.Key,
		secret:  cfg.Secret,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Configured 回写接口是否已配置
func (c *HTTPAfterSaleClient) Configured() bool { return c.baseURL != "" }

// Create 把售后请求推给电商执行落地。未配置时返回 (nil, nil)。
func (c *HTTPAfterSaleClient) Create(ctx context.Context, req *portcontract.AfterSaleRequest) (*AfterSaleExternalResult, error) {
	if !c.Configured() {
		return nil, nil
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := c.baseURL + "/api/refund"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.key)
	}
	if c.secret != "" {
		httpReq.Header.Set("X-Api-Secret", c.secret)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aftersale api status %d", resp.StatusCode)
	}
	var out struct {
		ExternalID string `json:"external_id"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &AfterSaleExternalResult{ExternalID: out.ExternalID, Status: out.Status}, nil
}
