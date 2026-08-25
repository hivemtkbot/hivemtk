package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// SearchProbe AI 搜索探针（v3 GEO 决策链化 Phase2 前置闸门）
//
// 红队论证 F1：现有 LLMAdapter 为纯 Chat Completion，无联网/引用返回能力。
// 信源反推必须有"真实引擎回答+被引 URL"数据源。本接口将探针渠道抽象为
// 可插拔实现；通用 HTTP 探针约定：
//
//	POST $GEO_SEARCH_PROBE_URL
//	请求: {"query": "..."}
//	响应: {"engine":"xxx","response":"...","citations":["https://..",".."]}
//
// 未配置 GEO_SEARCH_PROBE_URL 时使用 noopProbe（显式报错而非静默模拟，
// 杜绝把模型想象当真实引擎数据的红队 F1 缺陷复发）。

// Citation 单条被引信源
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// ProbeResult 单次探针结果
type ProbeResult struct {
	Engine    string     `json:"engine"`
	Query     string     `json:"query"`
	Response  string     `json:"response"`
	Citations []Citation `json:"citations"`
	LatencyMs int64      `json:"latency_ms"`
}

// SearchProbe 探针接口
type SearchProbe interface {
	Name() string
	Probe(ctx context.Context, query string) (*ProbeResult, error)
}

// httpSearchProbe 通用 HTTP 探针（对接任意外部"AI 搜索+引用"服务）
type httpSearchProbe struct {
	endpoint string
	client   *http.Client
}

func (p *httpSearchProbe) Name() string { return "http" }

func (p *httpSearchProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	body, _ := json.Marshal(map[string]string{"query": query})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("probe endpoint status %d", resp.StatusCode)
	}
	var out struct {
		Engine    string   `json:"engine"`
		Response  string   `json:"response"`
		Citations []string `json:"citations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	cites := make([]Citation, 0, len(out.Citations))
	for _, u := range out.Citations {
		cites = append(cites, Citation{URL: u})
	}
	return &ProbeResult{
		Engine: out.Engine, Query: query, Response: out.Response,
		Citations: cites, LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// noopProbe 未配置时的显式失败实现
type noopProbe struct{}

func (noopProbe) Name() string { return "noop" }
func (noopProbe) Probe(_ context.Context, _ string) (*ProbeResult, error) {
	return nil, fmt.Errorf("搜索探针未配置：请设置 GEO_SEARCH_PROBE_URL（红队论证 F1：禁止用纯 LLM 模拟冒充真实引擎）")
}

// NewDefaultSearchProbe 按环境选择探针实现
func NewDefaultSearchProbe() SearchProbe {
	if ep := strings.TrimSpace(os.Getenv("GEO_SEARCH_PROBE_URL")); ep != "" {
		return &httpSearchProbe{endpoint: ep, client: &http.Client{Timeout: 60 * time.Second}}
	}
	return noopProbe{}
}
