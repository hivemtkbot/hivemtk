package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

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
	Simulated bool       `json:"simulated"` // true = 模拟结果(仅历史数据兼容; R49后探针链不再产出模拟结果)
	Error     string     `json:"error,omitempty"`
	BrandHit  bool       `json:"brand_hit"`
	Sentiment string     `json:"sentiment"`
}

// SearchProbe 探针接口（保持对外签名不变，适配多引擎）
type SearchProbe interface {
	Name() string
	Probe(ctx context.Context, query string) (*ProbeResult, error)
}

// ---- HTTP 基础客户端（复用，避免各引擎重复 new Client） ----

var probeHTTPClient = &http.Client{Timeout: 60 * time.Second}

// doJSON 通用 JSON POST 辅助
func doJSON(ctx context.Context, endpoint, authHeader string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(rb))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rb, out)
}

// ---- 1. openaiProbe ----

type openaiProbe struct{ apiKey string }

func (p *openaiProbe) Name() string { return "openai" }

func (p *openaiProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Context struct {
					Annotations []struct {
						WebSearchPreview struct {
							Citations []struct {
								URL   string `json:"url"`
								Title string `json:"title"`
							} `json:"citations"`
						} `json:"web_search_preview"`
					} `json:"annotations"`
				} `json:"context"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	err := doJSON(ctx,
		"https://api.openai.com/v1/chat/completions",
		"Bearer "+p.apiKey,
		map[string]any{
			"model": "gpt-4o-mini-search-preview",
			"messages": []map[string]any{
				{"role": "user", "content": query},
			},
			"web_search_options": map[string]any{"search_context_size": "medium"},
		},
		&out,
	)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	cites := []Citation{}
	if len(out.Choices) > 0 {
		for _, ann := range out.Choices[0].Message.Context.Annotations {
			for _, c := range ann.WebSearchPreview.Citations {
				cites = append(cites, Citation{URL: c.URL, Title: c.Title})
			}
		}
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	return &ProbeResult{
		Engine: p.Name(), Query: query, Response: content,
		Citations: cites, LatencyMs: latency,
	}, nil
}

// ---- 2. perplexityProbe ----

type perplexityProbe struct{ apiKey string }

func (p *perplexityProbe) Name() string { return "perplexity" }

func (p *perplexityProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			}
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	err := doJSON(ctx,
		"https://api.perplexity.ai/chat/completions",
		"Bearer "+p.apiKey,
		map[string]any{
			"model": "sonar-pro",
			"messages": []map[string]any{
				{"role": "user", "content": query},
			},
		},
		&out,
	)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("perplexity: %w", err)
	}
	cites := []Citation{}
	for _, u := range out.Citations {
		cites = append(cites, Citation{URL: u})
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	return &ProbeResult{
		Engine: p.Name(), Query: query, Response: content,
		Citations: cites, LatencyMs: latency,
	}, nil
}

// ---- 3. deepseekProbe ----

type deepseekProbe struct{ apiKey string }

func (p *deepseekProbe) Name() string { return "deepseek" }

func (p *deepseekProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			}
		} `json:"choices"`
	}
	err := doJSON(ctx,
		"https://api.deepseek.com/v1/chat/completions",
		"Bearer "+p.apiKey,
		map[string]any{
			"model": "deepseek-chat",
			"messages": []map[string]any{
				{"role": "user", "content": query},
			},
			"enable_search": true,
		},
		&out,
	)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("deepseek: %w", err)
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	return &ProbeResult{
		Engine: p.Name(), Query: query, Response: content,
		LatencyMs: latency,
	}, nil
}

// ---- 4. doubaoProbe（豆包/火山引擎） ----

type doubaoProbe struct{ apiKey string }

func (p *doubaoProbe) Name() string { return "doubao" }

func (p *doubaoProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			}
		} `json:"choices"`
	}
	err := doJSON(ctx,
		"https://ark.cn-beijing.volces.com/api/v3/chat/completions",
		"Bearer "+p.apiKey,
		map[string]any{
			"model": "doubao-pro-32k",
			"messages": []map[string]any{
				{"role": "user", "content": query},
			},
		},
		&out,
	)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("doubao: %w", err)
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	return &ProbeResult{
		Engine: p.Name(), Query: query, Response: content,
		LatencyMs: latency,
	}, nil
}

// ---- 5. qwenProbe（阿里千问） ----

type qwenProbe struct{ apiKey string }

func (p *qwenProbe) Name() string { return "qwen" }

func (p *qwenProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	start := time.Now()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			}
		} `json:"choices"`
	}
	err := doJSON(ctx,
		"https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"Bearer "+p.apiKey,
		map[string]any{
			"model": "qwen-plus",
			"messages": []map[string]any{
				{"role": "user", "content": query},
			},
			"enable_search": true,
		},
		&out,
	)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("qwen: %w", err)
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	return &ProbeResult{
		Engine: p.Name(), Query: query, Response: content,
		LatencyMs: latency,
	}, nil
}

// ---- 工厂 ----

// NewEngineProbes 按环境变量自动装配所有可用真实引擎探针。
//
// R49 裁决：移除 mockProbe 兜底。57c1255 加入的"MockProbe 恒可用"与
// R21 红队 F1（禁止 LLM 模拟冒充真实引擎）及 R29 GEO 数据诚实性铁律冲突：
// LLM 模拟的信源/引用会流入思维链，污染 SOV/负面监控等全部下游指标。
// 无任何真实引擎配置时返回空列表，MultiEngineProbe.Probe 对空列表
// 显式报错（fail-closed），与未配置探针的既有契约一致。
func NewEngineProbes() []SearchProbe {
	probes := []SearchProbe{}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		probes = append(probes, &openaiProbe{apiKey: k})
	}
	if k := os.Getenv("PERPLEXITY_API_KEY"); k != "" {
		probes = append(probes, &perplexityProbe{apiKey: k})
	}
	if k := os.Getenv("DEEPSEEK_API_KEY"); k != "" {
		probes = append(probes, &deepseekProbe{apiKey: k})
	}
	if k := os.Getenv("DOUBAO_API_KEY"); k != "" {
		probes = append(probes, &doubaoProbe{apiKey: k})
	}
	if k := os.Getenv("QWEN_API_KEY"); k != "" {
		probes = append(probes, &qwenProbe{apiKey: k})
	}
	return probes
}

// MultiEngineProbe 将多个 SearchProbe 包装成一个 SearchProbe：顺序尝试，首个成功即返回
type MultiEngineProbe struct{ probes []SearchProbe }

func (m *MultiEngineProbe) Name() string { return "multi-engine" }

func (m *MultiEngineProbe) Probe(ctx context.Context, query string) (*ProbeResult, error) {
	probes := m.probes
	if len(probes) == 0 {
		probes = NewEngineProbes()
	}
	// R49: 空探针列表 fail-closed（红队 F1：无真实引擎配置必须显式报错，
	// 不允许 LLM 模拟冒充，防思维链/SOV 数据被模拟结果污染）
	if len(probes) == 0 {
		return nil, fmt.Errorf("no search probe engine configured: set one of OPENAI_API_KEY/PERPLEXITY_API_KEY/DEEPSEEK_API_KEY/DOUBAO_API_KEY/QWEN_API_KEY or GEO_SEARCH_PROBE_URL")
	}
	var lastErr error
	for _, p := range probes {
		r, err := p.Probe(ctx, query)
		if err == nil {
			return r, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all engines failed: %w", lastErr)
}

// NewDefaultSearchProbe 保持对外旧接口不变，内部返回 MultiEngineProbe 包装后的 SearchProbe
func NewDefaultSearchProbe() SearchProbe {
	return &MultiEngineProbe{probes: NewEngineProbes()}
}

// ---- ProbeService ----

// ProbeService 持有 []SearchProbe + ProbeRepository 的聚合服务
type ProbeService struct {
	probes []SearchProbe
	repo   repository.GeoProbeRunRepository
}

// NewProbeService 创建 ProbeService
func NewProbeService(probes []SearchProbe, repo repository.GeoProbeRunRepository) *ProbeService {
	return &ProbeService{probes: probes, repo: repo}
}

// ProbeAllEngines 遍历所有引擎，逐个调用，结果写入 geo_probe_runs
// 返回成功的结果 + 失败的错误（部分引擎失败不阻断）
func (s *ProbeService) ProbeAllEngines(ctx context.Context, query string) ([]*model.GeoProbeRun, []error) {
	probes := s.probes
	if len(probes) == 0 {
		probes = NewEngineProbes()
	}
	runs := make([]*model.GeoProbeRun, 0, len(probes))
	errs := []error{}
	for _, p := range probes {
		pr, err := p.Probe(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Errorf("probe %s: %w", p.Name(), err))
			continue
		}
		// 轻量匹配：品牌提及 + 情感（不需要 LLM，strings.Contains 足够）
		brandName := s.getBrandName(ctx)
		negativeWords := []string{"差评", "投诉", "骗局", "失败", "坑", "垃圾", "烂"}
		isNegative := false
		for _, nw := range negativeWords {
			if strings.Contains(strings.ToLower(pr.Response), strings.ToLower(nw)) {
				isNegative = true
				break
			}
		}
		pr.BrandHit = brandName != "" && strings.Contains(strings.ToLower(pr.Response), strings.ToLower(brandName))
		if isNegative {
			pr.Sentiment = "negative"
		} else {
			pr.Sentiment = "neutral"
		}
		run := &model.GeoProbeRun{
			Engine:         pr.Engine,
			Query:          pr.Query,
			Response:       pr.Response,
			LatencyMs:      pr.LatencyMs,
			Sentiment:      pr.Sentiment,
			BrandMentioned: pr.BrandHit,
		}
		// 持久化引用（JSON 字符串形式存入 datatypes.JSON）
		if len(pr.Citations) > 0 {
			b, _ := json.Marshal(pr.Citations)
			run.Citations = b
		}
		if err := s.repo.Create(ctx, run); err != nil {
			errs = append(errs, fmt.Errorf("persist probe %s: %w", p.Name(), err))
			continue
		}
		runs = append(runs, run)
	}
	return runs, errs
}

// TestSingle 测试单个引擎（给前端调试用）
func (s *ProbeService) TestSingle(ctx context.Context, engineName, query string) (*ProbeResult, error) {
	for _, p := range s.probes {
		if engineName == "" || p.Name() == engineName {
			pr, err := p.Probe(ctx, query)
			if err != nil {
				return nil, err
			}
			// 轻量匹配：品牌提及 + 情感（与 ProbeAllEngines 一致）
			brandName := s.getBrandName(ctx)
			negativeWords := []string{"差评", "投诉", "骗局", "失败", "坑", "垃圾", "烂"}
			isNegative := false
			for _, nw := range negativeWords {
				if strings.Contains(strings.ToLower(pr.Response), strings.ToLower(nw)) {
					isNegative = true
					break
				}
			}
			pr.BrandHit = brandName != "" && strings.Contains(strings.ToLower(pr.Response), strings.ToLower(brandName))
			if isNegative {
				pr.Sentiment = "negative"
			} else {
				pr.Sentiment = "neutral"
			}
			return pr, nil
		}
	}
	return nil, fmt.Errorf("engine %s not found (available: %s)", engineName, availableEngineNames(s.probes))
}

// ListRuns 返回最近 N 条探针运行记录
func (s *ProbeService) ListRuns(ctx context.Context, limit int) ([]*model.GeoProbeRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListRecent(ctx, limit)
}

func availableEngineNames(probes []SearchProbe) string {
	names := make([]string, 0, len(probes))
	for _, p := range probes {
		names = append(names, p.Name())
	}
	return strings.Join(names, ",")
}

// getBrandName 从 GeoConfig 读取品牌名（轻量、不打 LLM）
func (s *ProbeService) getBrandName(ctx context.Context) string {
	cfgRepo := repository.NewGeoConfigRepository()
	cfg, err := cfgRepo.Get()
	if err != nil || cfg.BrandName == "" {
		return ""
	}
	return cfg.BrandName
}
