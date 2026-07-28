package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
)

// EmbeddingConfig Embedding 配置
//
// 私域部署基线（2026-07-18，2026-07-24 修订）：Embedding 必须在本地推理服务内完成，
// 跑真实 BGE 模型（bge-m3，1024 维），不允许静默走公网 LLM 厂商 API，
// 也不允许静默降级到哈希伪向量。
//
//   - 正常路径：POST {BaseURL}/v1/embeddings（bge-m3，dim=1024）
//   - 强约束：BaseURL 默认 http://mtk-embedding:8208/v1（docker 网络）或 http://127.0.0.1:8208/v1（宿主机）
//   - 离线/单测：需显式设置 EMBEDDING_ALLOW_FALLBACK=true 才允许 hash 降级
type EmbeddingConfig struct {
	APIKey         string
	BaseURL        string
	APIType        string // openai（TEI 兼容 OpenAI /v1/embeddings）
	Model          string // 默认 bge-m3
	Dimension      int    // 期望向量维度（用于校验，默认 1024）
	RequestTimeout int    // 秒
	MaxRetries     int
	AllowFallback  bool // 是否允许 hash 伪向量降级（仅单测，默认 false）
}

// EmbeddingServiceInterface Embedding 服务接口
type EmbeddingServiceInterface interface {
	Embed(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error)
	EmbedOne(ctx context.Context, cfg *EmbeddingConfig, text string) ([]float32, error)
	DefaultConfig() *EmbeddingConfig
}

// EmbeddingService 真实 Embedding 服务
//
// 默认对接本地推理服务（http://mtk-embedding:8208/v1 docker / http://127.0.0.1:8208/v1 宿主机，私域部署强制）；
// 调用 /v1/embeddings（OpenAI 兼容协议，底层跑真实 bge-m3 模型）。
//
// 强约束（2026-07-18）：
//  1. 不再 fallback 到任何云端 LLM 厂商（OpenAI/Azure/通义/智谱/Moonshot）
//     私域数据禁止出域；LLM 走 API ≠ Embedding 走 API。
//  2. 当 EMBEDDING_ALLOW_FALLBACK=false（默认）时，
//     若本地 embedding 不可达直接返回错误，禁止静默使用哈希伪向量。
//  3. 当 EMBEDDING_ALLOW_FALLBACK=true 时（仅单测），
//     允许降级到 hash 实现并打 ERROR 日志。
type EmbeddingService struct {
	httpClient    *http.Client
	mu            sync.RWMutex
	fallback      EmbeddingServiceInterface // 仅在 EMBEDDING_ALLOW_FALLBACK=true 时启用
	defaultConfig *EmbeddingConfig          // 可选：per 知识库显式覆盖全局默认
}

// sharedEmbeddingTransport 进程级共享 Transport，避免每次请求 new http.Client 导致连接不复用、
// 连接数随并发线性膨胀。
var sharedEmbeddingTransport = &http.Transport{
	MaxIdleConns:        200,
	MaxIdleConnsPerHost: 50,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// embeddingSem 进程级并发闸门：控制同一时刻在飞的 embedding 请求数。
//
// 历史：TEI 容器以 --max-concurrent-requests=1 运行，容量硬编码 1。
// 现在（2026-07-24）：切换到 llama.cpp，其 embedding 模式默认 --parallel 1，
// 但可通过 EMBEDDING_PARALLEL 环境变量调高（llama-server 端 --parallel + Go 端闸门同步）。
//
// 容量来源（init 时读取）：
//   - 环境变量 EMBEDDING_CONCURRENCY（优先；与 start-embedding.sh --parallel 对应）
//   - 默认 1（保守，避免内存带宽竞争；CPU bound 场景并发收益有限）
//
// 注意：调高此值时，必须同步调高 llama-server 的 --parallel（见 env.sh EMBEDDING_PARALLEL），
// 否则多余的请求会在 llama-server 队列中排队，不会带来实际并发收益。
var embeddingSem = newEmbeddingSem()

func newEmbeddingSem() chan struct{} {
	n := 1
	if v := strings.TrimSpace(os.Getenv("EMBEDDING_CONCURRENCY")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 1 && parsed <= 16 {
			n = parsed
		}
	}
	return make(chan struct{}, n)
}

// NewEmbeddingService 构造真实 Embedding 服务
func NewEmbeddingService() *EmbeddingService {
	return &EmbeddingService{
		httpClient: &http.Client{Transport: sharedEmbeddingTransport, Timeout: 60 * time.Second},
		fallback:   &HashEmbeddingService{defaultDim: 1024},
	}
}

// NewEmbeddingServiceWithConfig 构造带 per 知识库配置的 Embedding 服务
// （DefaultConfig 直接返回 cfg，使检索/入库走指定云端 text-embedding 端点）
func NewEmbeddingServiceWithConfig(cfg *EmbeddingConfig) *EmbeddingService {
	return &EmbeddingService{
		httpClient:    &http.Client{Transport: sharedEmbeddingTransport, Timeout: 60 * time.Second},
		fallback:      &HashEmbeddingService{defaultDim: 1024},
		defaultConfig: cfg,
	}
}

// DefaultConfig 读取配置得到默认配置（配置文件为准，其次环境变量，最后内置 docker 默认）
//
// 优先级（私域部署基线 2026-07-24 修订）：
//  1. config.yaml / config-docker.yaml 的 embedding 段（按环境提供正确的本地服务地址，为准）：
//     - 宿主机：http://127.0.0.1:8208/v1（宿主机 llama.cpp，端口 8208）
//     - docker：http://mtk-embedding:8208/v1（容器内服务名，端口 8208）
//  2. 环境变量（EMBEDDING_*）仅在配置文件未指定时回退读取，便于部署层/调试显式覆盖
//  3. 内置默认：http://mtk-embedding:8208/v1（docker 网络，仅当配置与环境都缺失时生效）
//
// 注意：必须以配置文件为准，否则宿主机会被 docker 专用的 mtk-embedding 服务名带偏
// （宿主机无法解析该名，导致 embedding 不可达并静默降级哈希伪向量）。
func (s *EmbeddingService) DefaultConfig() *EmbeddingConfig {
	// per 知识库覆盖：实例被显式赋予配置时优先返回（不读全局 config.yaml）
	if s.defaultConfig != nil {
		return s.defaultConfig
	}
	// 1) 配置文件为准（按环境提供正确的本地服务地址）
	fileCfg := config.GetAppConfig().Inference.Embedding
	baseURL := fileCfg.BaseURL
	model := fileCfg.Model
	dim := fileCfg.Dimension
	allowFallback := fileCfg.AllowFallback
	apiKey := fileCfg.APIKey

	// 2) 环境变量仅在配置文件未指定时回退读取（便于部署层/调试显式覆盖）
	if baseURL == "" {
		baseURL = os.Getenv("EMBEDDING_BASE_URL")
	}
	if model == "" {
		model = os.Getenv("EMBEDDING_MODEL")
	}
	if dim <= 0 {
		if v := os.Getenv("EMBEDDING_DIM"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				dim = n
			}
		}
	}
	// 环境变量显式控制哈希降级开关：true/1/yes 开启；false/0/no 关闭（覆盖配置文件）
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDING_ALLOW_FALLBACK"))); v != "" {
		allowFallback = v == "true" || v == "1" || v == "yes"
	}

	// 3) 内置默认（docker 网络内本地 embedding 服务名，端口 8208；仅当配置与环境都缺失时生效）
	if baseURL == "" {
		// 单一源：config.DefaultEmbeddingBaseURLDocker（user-server/internal/pkg/utils/config/ports.go）
		// 文档源：DEVELOPMENT.md §2.4 + docker-compose-host.yml mtk-embedding 服务
		// 行为：仅在容器内可解析 mtk-embedding；host 部署会失败并回退到 hash 实现（需 EMBEDDING_ALLOW_FALLBACK=true）
		baseURL = config.DefaultEmbeddingBaseURLDocker
	}
	if model == "" {
		// 单一源：config.DefaultEmbeddingModel()（user-server/internal/pkg/utils/config/server.go）
		// 文档源：DEVELOPMENT.md §2.4 + config.yaml inference.embedding.model
		// 行为：dev 档默认为 bge-m3（1024 维，与 pgvector vector(1024) 对齐）
		model = config.DefaultEmbeddingModel()
	}
	// 按模型名推断默认维度（仅当维度未显式指定时生效）
	if dim <= 0 {
		switch model {
		case "text-embedding-3-large":
			dim = 3072
		case "text-embedding-ada-002", "text-embedding-3-small":
			dim = 1536
		default:
			dim = 1024
		}
	}
	// 硬约束：文本向量维度必须与 pgvector vector(1024) 一致；非 1024 一律回落 1024 并告警
	if dim != 1024 {
		logger.Warnf("[embedding] WARN: configured dimension=%d != 1024, force reset to 1024 (pgvector vector(1024) compatible)", dim)
		dim = 1024
	}

	if apiKey == "" {
		apiKey = os.Getenv("EMBEDDING_API_KEY")
	}
	return &EmbeddingConfig{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		APIType:        "openai", // TEI 兼容
		Model:          model,
		Dimension:      dim,
		RequestTimeout: 60,
		MaxRetries:     5,
		AllowFallback:  allowFallback,
	}
}

// embeddingMaxBatch 单次 embedding 请求最大文本数。
// 超过此值时自动分片串行请求，避免单次请求体过大导致 llama-server OOM 或超时。
// llama-server 默认 --batch-size=512（token 级），64 条短文本约 6400 token 会分批处理，
// 设 64 是兼顾吞吐与内存安全的经验值。
const embeddingMaxBatch = 64

// Embed 批量向量化
func (s *EmbeddingService) Embed(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if cfg == nil {
		cfg = s.DefaultConfig()
	}

	// 私域部署基线（2026-07-16）：禁止静默降级
	// 除非显式开启 EMBEDDING_ALLOW_FALLBACK=true（仅供单元测试）
	if !cfg.AllowFallback {
		// 大批量分片：超过 embeddingMaxBatch 时串行分批，避免 OOM/超时
		if len(texts) <= embeddingMaxBatch {
			return s.callProviderWithRetry(ctx, cfg, texts)
		}
		return s.embedInBatches(ctx, cfg, texts)
	}

	// 显式允许降级（单测场景）：打 ERROR 日志后回退
	logger.Errorf("[Embedding] ERROR: EMBEDDING_ALLOW_FALLBACK=true,降级为本地哈希向量(仅供离线/单测,严禁生产环境使用)")
	return s.fallback.Embed(ctx, cfg, texts)
}

// embedInBatches 将大批量文本分片串行请求，合并结果保持原始顺序
func (s *EmbeddingService) embedInBatches(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error) {
	total := len(texts)
	result := make([][]float32, total)
	for i := 0; i < total; i += embeddingMaxBatch {
		end := i + embeddingMaxBatch
		if end > total {
			end = total
		}
		batch := texts[i:end]
		vectors, err := s.callProviderWithRetry(ctx, cfg, batch)
		if err != nil {
			return nil, fmt.Errorf("embedding 分片 %d-%d 失败: %w", i, end-1, err)
		}
		copy(result[i:end], vectors)
	}
	logger.Infof("[Embedding] 大批量分片完成: %d 条文本, 分 %d 批", total, (total+embeddingMaxBatch-1)/embeddingMaxBatch)
	return result, nil
}

// callProviderWithRetry 带重试的本地 embedding 调用
//
// 候选 BaseURL：docker 内 EMBEDDING_BASE_URL 默认指向服务名 mtk-embedding；
// 宿主机（macOS 等）无法解析该服务名时，自动回退到 localhost（同一实例），
// 不影响 docker 内解析（docker 中 mtk-embedding 首轮即成功，不会触发回退）。
func (s *EmbeddingService) callProviderWithRetry(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error) {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	candidates := s.baseURLCandidates(cfg.BaseURL)
	var lastErr error
	for ci, baseURL := range candidates {
		attemptCfg := *cfg
		attemptCfg.BaseURL = baseURL
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				// 限流/过载（429、no permits available、Model is overloaded）时采用更长退避，
				// 因为本地 Qwen3-Embedding-0.6B 单次推理约 10s，短退避（1/2/4s）仍会撞上在飞的请求。
				// 其余错误用指数退避。两者均以 30s 封顶。
				if isRateLimited(lastErr) {
					backoff := min(time.Duration(2<<uint(attempt))*time.Second, 30*time.Second)
					logger.Warnf("[Embedding] 限流/过载，退避 %.0fs 后重试 (baseURL=%s attempt %d/%d): %v", backoff.Seconds(), baseURL, attempt+1, maxRetries, lastErr)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(backoff):
					}
				} else {
					backoff := min(time.Duration(1<<uint(attempt))*time.Second, 30*time.Second)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(backoff):
					}
				}
			}
			// 串行占用 embedding 闸门（容量=1），确保同一时刻仅 1 个请求打到 TEI，
			// 彻底避免自竞争 429。
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case embeddingSem <- struct{}{}:
			}
			vectors, err := s.callProvider(ctx, &attemptCfg, texts)
			<-embeddingSem
			if err == nil {
				return vectors, nil
			}
			lastErr = err
			// 连接类错误（服务名无法解析/拒绝连接）且仍有候选时，立即切换候选，避免无谓退避
			if ci < len(candidates)-1 && isConnError(err) {
				logger.Warnf("[Embedding] %s 不可达，回退候选 BaseURL: %v", baseURL, err)
				break
			}
			logger.Errorf("[Embedding] 本地 embedding 服务调用失败 (baseURL=%s attempt %d/%d): %v", baseURL, attempt+1, maxRetries, err)
		}
	}
	return nil, fmt.Errorf("本地 embedding 服务不可达（%s），已重试 %d 次: %w。请检查 embedding 容器是否启动或设置正确的 EMBEDDING_BASE_URL: %v",
		cfg.BaseURL, maxRetries, lastErr, lastErr)
}

// baseURLCandidates 返回候选 BaseURL 列表。
// 当配置 host 为 mtk-embedding（docker 服务名）时，附带 localhost 候选（保留原端口），
// 用于宿主机部署回退（宿主机无法解析 docker 服务名时自动切到 localhost）。
func (s *EmbeddingService) baseURLCandidates(raw string) []string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return []string{raw}
	}
	if strings.HasPrefix(u.Host, "mtk-embedding") || strings.HasPrefix(u.Host, "tei-embedding") {
		alt := *u
		alt.Host = "localhost" + strings.TrimPrefix(u.Host, "mtk-embedding")
		alt.Host = strings.TrimPrefix(alt.Host, "tei-embedding")
		return []string{raw, alt.String()}
	}
	return []string{raw}
}

// isConnError 判断是否为连接类错误（DNS 不可解析 / 连接被拒 / 超时），用于快速切换候选 BaseURL。
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "i/o timeout")
}

// isRateLimited 判断是否为限流/过载类错误：TEI 在并发超限时返回 429 且 body 含
// "no permits available" / "Model is overloaded"，需以更长退避重试而非立即失败。
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no permits available") ||
		strings.Contains(msg, "Model is overloaded") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "Too Many Requests")
}

// EmbedOne 单条向量化
func (s *EmbeddingService) EmbedOne(ctx context.Context, cfg *EmbeddingConfig, text string) ([]float32, error) {
	vectors, err := s.Embed(ctx, cfg, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding 返回空")
	}
	return vectors[0], nil
}

// embeddingRequest 通用 OpenAI 兼容请求体
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingResponse 通用 OpenAI 兼容响应
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (s *EmbeddingService) callProvider(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("EMBEDDING_BASE_URL 未配置")
	}

	// 兼容 baseURL 已含 /v1 后缀的情况（如 http://mtk-embedding:8208/v1）
	// 私域部署基线：BaseURL 默认就是 /v1 结尾的 OpenAI 兼容根路径
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = baseURL + "/v1"
	}
	endpoint := baseURL + "/embeddings"
	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	// 复用 s.httpClient（共享 Transport 连接池），通过 context.WithTimeout 控制超时
	// 避免每次请求创建新 http.Client 对象增加 GC 压力
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(embeddingRequest{Model: cfg.Model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("embedding 鉴权失败 status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("embedding 服务端错误 status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var er embeddingResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
	}
	if er.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", er.Error.Message)
	}
	if len(er.Data) != len(texts) {
		return nil, fmt.Errorf("embedding 返回数量不匹配: expect %d, got %d", len(texts), len(er.Data))
	}

	vectors := make([][]float32, len(texts))
	for _, d := range er.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding 返回 index 越界: %d", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, nil
}
