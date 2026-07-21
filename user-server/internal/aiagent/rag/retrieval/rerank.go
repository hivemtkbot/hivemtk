package ragretrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
)

// RerankDoc 待重排文档
type RerankDoc struct {
	ID      string
	Content string
}

// RerankResult 重排结果（按相关性降序）
type RerankResult struct {
	ID    string
	Score float64
}

// RerankerInterface 重排接口
type RerankerInterface interface {
	// Rerank 对候选文档按与 query 的相关性重排，返回按分数降序的结果
	Rerank(ctx context.Context, query string, docs []RerankDoc) ([]RerankResult, error)
}

// RerankConfig 重排配置（来自环境变量）
type RerankConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	Enabled    bool
	Timeout    int
	MaxRetries int
}

// LocalReranker 基于本地推理服务的重排实现（OpenAI 兼容 /rerank）
//
// 私域部署基线（2026-07-18）：rerank 与 embedding 同走本地推理容器，
// 使用 bge-reranker-v2-m3（跨编码器），数据不出域。
//
// 端点约定（与 embedding 类似，直接在 BaseURL 后追加 /rerank）：
//   - TEI:         BaseURL=http://host:port        → POST {BaseURL}/rerank
//   - llama.cpp:   BaseURL=http://host:port/v1     → POST {BaseURL}/rerank（即 /v1/rerank）
//
// 两者响应字段一致：results[].index / results[].relevance_score，业务代码零改动。
type LocalReranker struct {
	httpClient *http.Client
	cfg        *RerankConfig // 可选：per 知识库显式覆盖全局默认
}

// sharedRerankTransport 进程级共享 Transport，避免每次请求 new http.Client 导致连接不复用、
// 连接数随并发线性膨胀（评审 V5 修复）。
var sharedRerankTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 20,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// NewLocalReranker 构造本地重排器
func NewLocalReranker() *LocalReranker {
	return &LocalReranker{httpClient: &http.Client{Transport: sharedRerankTransport, Timeout: 30 * time.Second}}
}

// NewLocalRerankerWithConfig 构造带 per 知识库配置的重排器
// （Rerank 直接使用 cfg，使重排走指定云端 rerank 端点）
func NewLocalRerankerWithConfig(cfg *RerankConfig) *LocalReranker {
	return &LocalReranker{
		httpClient: &http.Client{Transport: sharedRerankTransport, Timeout: 30 * time.Second},
		cfg:        cfg,
	}
}

// 私域部署 rerank 模型支持表（2026-07-21）：
//   - bge-reranker-base       轻量基线，CPU 可跑，单文档 <50ms
//   - bge-reranker-large      高精度，GPU 推荐，单文档 <200ms
//   - bge-reranker-v2-m3      多语种，默认推荐
//   - bge-reranker-v2-gemma   多语种高精（自托管，>2B 参数）
//
// 通过 RERANK_MODEL 环境变量或 config.yaml 切换；BaseURL 不变。
const (
	RerankModelBgeBase  = "bge-reranker-base"
	RerankModelBgeLarge = "bge-reranker-large"
	RerankModelBgeV2M3  = "bge-reranker-v2-m3"
)

// DefaultRerankConfig 读取重排配置（配置文件为准，其次环境变量，最后内置 docker 默认）
//
// 优先级（与 Embedding 一致的私域基线）：
//  1. config.yaml / config-docker.yaml 的 rerank 段（为准）
//     - 宿主机：http://127.0.0.1:9998/v1（OrbStack 暴露的本地 TEI rerank）
//     - docker：http://tei-rerank:9998/v1（容器内服务名）
//  2. 环境变量（RERANK_*）仅在配置文件未指定时回退读取
//  3. 内置默认：http://tei-rerank:9998/v1（docker 网络，仅当配置与环境都缺失时生效）
//
// 必须以配置文件为准，否则宿主机被 docker 专用的 tei-rerank 服务名带偏（宿主机无法解析）。
func DefaultRerankConfig() *RerankConfig {
	// 1) 配置文件为准
	fileCfg := config.GetAppConfig().Inference.Rerank
	baseURL := fileCfg.BaseURL
	model := fileCfg.Model
	apiKey := fileCfg.APIKey
	// 默认启用；仅当配置文件显式指定 enabled: false 或环境变量关闭时才禁用
	enabled := true
	if baseURL != "" && !fileCfg.Enabled {
		enabled = false
	}

	// 2) 环境变量仅在配置文件未指定时回退读取（便于部署层/调试显式覆盖）
	if baseURL == "" {
		baseURL = os.Getenv("RERANK_BASE_URL")
	}
	if model == "" {
		model = os.Getenv("RERANK_MODEL")
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("RERANK_ENABLED"))); v == "false" || v == "0" || v == "no" {
		enabled = false
	}

	// 3) 内置默认（docker 网络内本地 TEI rerank 服务名）
	if baseURL == "" {
		// fallback 必须指向 rerank 服务（teirank），不能 fallback 到 embedding（9997）
		baseURL = "http://mtk-rerank:9998/v1"
	}
	if model == "" {
		model = RerankModelBgeV2M3
	}
	timeout := 30
	if v := os.Getenv("RERANK_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("RERANK_API_KEY")
	}
	return &RerankConfig{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Model:      model,
		Enabled:    enabled,
		Timeout:    timeout,
		MaxRetries: 2,
	}
}

type rerankRequest struct {
	Model string   `json:"model"`
	Query string   `json:"query"`
	Texts []string `json:"texts"`
}

type rerankResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Rerank 调用本地推理服务（TEI 或 llama.cpp）的 /rerank 对候选文档重排
func (r *LocalReranker) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]RerankResult, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	cfg := r.cfg
	if cfg == nil {
		cfg = DefaultRerankConfig()
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("rerank 未启用（RERANK_ENABLED=false）")
	}

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	// 端点约定：直接在 BaseURL 后追加 /rerank。
	//   - TEI:       BaseURL=http://host:port      → /rerank（根路径）
	//   - llama.cpp: BaseURL=http://host:port/v1   → /v1/rerank
	// 两种部署响应字段一致，商户仅需切换 BaseURL 即可。
	endpoint := baseURL + "/rerank"
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		res, err := r.callOnce(ctx, endpoint, timeout, cfg, query, texts)
		if err == nil {
			return mapRerankResults(docs, res), nil
		}
		lastErr = err
		logger.Errorf("[Rerank] 本地重排服务调用失败 (attempt %d/%d): %v", attempt+1, maxRetries, err)
	}
	return nil, fmt.Errorf("本地重排服务不可达（%s），已重试 %d 次: %w", endpoint, maxRetries, lastErr)
}

func (r *LocalReranker) callOnce(ctx context.Context, endpoint string, timeout time.Duration, cfg *RerankConfig, query string, texts []string) (*rerankResponse, error) {
	body, err := json.Marshal(rerankRequest{Model: cfg.Model, Query: query, Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Transport: sharedRerankTransport, Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank API error status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var rr rerankResponse
	if err := json.Unmarshal(respBody, &rr); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w (body=%s)", err, string(respBody))
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("rerank API error: %s", rr.Error.Message)
	}
	return &rr, nil
}

// mapRerankResults 将 TEI 返回（index + score）映射回原始文档 ID，并按分数降序
func mapRerankResults(docs []RerankDoc, rr *rerankResponse) []RerankResult {
	type scored struct {
		id    string
		score float64
	}
	scoredMap := make(map[int]float64, len(rr.Results))
	for _, item := range rr.Results {
		scoredMap[item.Index] = item.RelevanceScore
	}

	results := make([]scored, 0, len(docs))
	for i, d := range docs {
		score := scoredMap[i]
		results = append(results, scored{id: d.ID, score: score})
	}
	sort.SliceStable(results, func(a, b int) bool {
		return results[a].score > results[b].score
	})

	out := make([]RerankResult, 0, len(results))
	for _, s := range results {
		out = append(out, RerankResult{ID: s.id, Score: s.score})
	}
	return out
}

// SetReranker 为检索服务装配重排器（可选；nil 时跳过重排）
func (r *RagRetrievalServiceImpl) SetReranker(reranker RerankerInterface) {
	r.reranker = reranker
}

// toRerankDocs 将检索分片转为重排文档
func toRerankDocs(chunks []Chunk) []RerankDoc {
	docs := make([]RerankDoc, 0, len(chunks))
	for _, c := range chunks {
		docs = append(docs, RerankDoc{ID: c.ID, Content: c.Content})
	}
	return docs
}

// applyRerank 依据重排结果对分片重新排序（保留未出现在结果中的分片于末尾）
func applyRerank(chunks []Chunk, results []RerankResult) []Chunk {
	byID := make(map[string]Chunk, len(chunks))
	for _, c := range chunks {
		byID[c.ID] = c
	}
	ordered := make([]Chunk, 0, len(chunks))
	seen := make(map[string]bool, len(results))
	for _, res := range results {
		if c, ok := byID[res.ID]; ok {
			ordered = append(ordered, c)
			seen[res.ID] = true
		}
	}
	// 追加重排结果中未覆盖的分片（保底）
	for _, c := range chunks {
		if !seen[c.ID] {
			ordered = append(ordered, c)
		}
	}
	return ordered
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
